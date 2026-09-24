// Package cli implements the ctxpack command-line interface. It is a tiny
// stdlib-only subcommand dispatcher (no cobra/urfave) so the binary stays
// dependency-free: every operation routes through the packer/repomap/gitutil
// packages, and the MCP server in internal/mcp calls the same, so the two
// frontends cannot drift apart.
package cli

import (
	"encoding/csv"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"runtime"
	"sort"
	"strconv"
	"strings"

	"github.com/la2278647-arch/ctxpack/internal/counter"
	"github.com/la2278647-arch/ctxpack/internal/format"
	"github.com/la2278647-arch/ctxpack/internal/gitutil"
	"github.com/la2278647-arch/ctxpack/internal/mcp"
	"github.com/la2278647-arch/ctxpack/internal/packer"
	"github.com/la2278647-arch/ctxpack/internal/repomap"
	"github.com/la2278647-arch/ctxpack/internal/version"
	"github.com/la2278647-arch/ctxpack/internal/walker"
)

// Run dispatches a command and returns a process exit code.
func Run(args []string) int {
	if len(args) == 0 {
		printHelp(os.Stdout)
		return 0
	}
	switch args[0] {
	case "help", "-h", "--help":
		printHelp(os.Stdout)
		return 0
	case "version", "--version", "-v":
		fmt.Println(version.Info())
		return 0
	case "pack":
		return cmdPack(args[1:])
	case "map":
		return cmdMap(args[1:])
	case "diff":
		return cmdDiff(args[1:])
	case "tokens":
		return cmdTokens(args[1:])
	case "models":
		return cmdModels(args[1:])
	case "mcp":
		return runMCP(args[1:])
	case "doctor":
		return cmdDoctor(args[1:])
	default:
		fmt.Fprintf(os.Stderr, "ctxpack: unknown command %q\n\n", args[0])
		printHelp(os.Stderr)
		return 2
	}
}

func printHelp(w io.Writer) {
	fmt.Fprintf(w, `ctxpack %s — pack a repository into LLM-optimized context

USAGE
  ctxpack <command> [path] [flags]

COMMANDS
  pack <path>          Pack all (or selected) files into one bundle (default: XML).
  map  <path>          Print a token-aware tree outline of the repo.
  diff <path>          Pack only files changed vs a git ref (default: working tree).
  tokens <path>        Estimate total tokens and show per-model fit.
  models               List known LLMs and their context windows.
  mcp                  Run as a Model Context Protocol server on stdio.
  doctor               Print environment diagnostics (version, git, models).
  version              Print the build identity.
  help                 Show this help.

FLAGS (pack)
  --format F           xml|markdown|json|text (default xml)
  --include GLOB       Only include paths matching GLOB (repeatable; basename ok)
  --exclude GLOB       Exclude paths matching GLOB (repeatable)
  --max-size BYTES     Read no more than BYTES of a file (larger files stay listed, without content). 0 = unlimited
  --no-gitignore       Ignore .gitignore files (built-in defaults still apply)
  --hidden             Include dotfiles/dotdirs (.git always skipped)
  --budget N           Cap output to ~N tokens (priority-selects files)
  --model NAME         Annotate fit for a model (gpt-4o, claude-3.5-sonnet, ...)
  -o, --output FILE    Write to FILE instead of stdout
  -q, --quiet          Suppress the 'wrote' message when --output is set

FLAGS (diff)
  --ref REF            Base git ref (default: working-tree changes). e.g. HEAD~1, main
  -q, --quiet          Suppress stderr status messages
  (also accepts --format/--include/--exclude/--budget/--model/-o)

FLAGS (map / tokens / models)
  --include/--exclude/--max-size/--no-gitignore/--hidden   (map / tokens only)
  --depth N           Limit traversal to N levels below root (0 = unlimited)
  --sort BY           (map only) Sort children by: name (default), tokens, bytes
  --top N             (map only) Flat list of the N largest files
  --csv               (map only) Flat CSV list of all files
  --model NAME        (tokens only) Show fit for one model instead of all
  --sort BY           (tokens only) Sort fit table by: name (default), pct, window
  --vendor NAME       (models only) Show only models from this vendor
  --json              Emit JSON instead of the text output, for scripting

EXAMPLES
  ctxpack pack ./myrepo --format markdown -o repo.md
  ctxpack pack . --include "*.go" --exclude "*_test.go" --model gpt-4o
  ctxpack diff . --ref main            # what changed since main
  ctxpack pack . --budget 60000 --model gpt-4o
  ctxpack map . --json                 # the tree as JSON, for scripting
  ctxpack tokens . --json              # per-model fit as JSON
  ctxpack models --json                # the model table as JSON
  ctxpack mcp                          # for Claude Desktop / Cursor config

Project: https://github.com/la2278647-arch/ctxpack
`, version.Version)
}

// --- flag value types ---

type stringList []string

func (s *stringList) String() string     { return strings.Join(*s, ",") }
func (s *stringList) Set(v string) error { *s = append(*s, v); return nil }

// --- pack ---

func cmdPack(args []string) int {
	fs := flag.NewFlagSet("pack", flag.ContinueOnError)
	fs.Usage = func() { printHelp(os.Stderr) }
	var (
		fmtFlag  = fs.String("format", envStrDefault("CTXPACK_FORMAT", "xml"), "output format: xml|markdown|json|text")
		includes stringList
		excludes stringList
		maxSize  = fs.Int64("max-size", 0, "read no more than N bytes of a file (larger files are still listed)")
		noGit    = fs.Bool("no-gitignore", false, "ignore .gitignore")
		hidden   = fs.Bool("hidden", false, "include dotfiles")
		depth    = fs.Int("depth", 0, "limit traversal to N levels below root (0 = unlimited)")
		budget   = fs.Int("budget", envInt("CTXPACK_BUDGET"), "cap output to ~N tokens")
		model    = fs.String("model", os.Getenv("CTXPACK_MODEL"), "annotate fit for a model")
		output   = fs.String("output", "", "write to FILE (default stdout)")
		quiet    = fs.Bool("quiet", false, "suppress the 'wrote' message when --output is set")
	)
	fs.Var(&includes, "include", "include glob (repeatable)")
	fs.Var(&excludes, "exclude", "exclude glob (repeatable)")
	fs.StringVar(output, "o", "", "shorthand for --output")
	fs.BoolVar(quiet, "q", false, "shorthand for --quiet")
	if err := fs.Parse(reorderArgs(fs, args)); err != nil {
		return 2
	}
	path := "."
	if fs.NArg() > 0 {
		path = fs.Arg(0)
	}

	// Validate the format before walking the tree, so a typo fails fast.
	outFmt, err := parseFormat(*fmtFlag)
	if err != nil {
		fmt.Fprintln(os.Stderr, "ctxpack:", err)
		return 2
	}

	bundle, err := packer.Pack(path, packer.Options{
		Walker: walker.Options{
			Include:          []string(includes),
			Exclude:          []string(excludes),
			MaxFileSize:      *maxSize,
			MaxDepth:         *depth,
			RespectGitignore: !*noGit,
			IncludeHidden:    *hidden,
			ReadContent:      true,
		},
		Budget: *budget,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "ctxpack:", err)
		return 1
	}

	out := format.Render(bundle, outFmt)
	header := ""
	if *model != "" {
		note := annotateFit(bundle.TotalTokens, *model)
		if outFmt == format.JSON && !*quiet {
			// An HTML comment in front of a JSON document makes the file fail
			// to parse, so the note goes to stderr instead of the output.
			fmt.Fprintln(os.Stderr, note)
		} else {
			header = note + "\n"
		}
	}
	if err := writeOutput(*output, header+out); err != nil {
		fmt.Fprintln(os.Stderr, "ctxpack:", err)
		return 1
	}
	if *output != "" && !*quiet {
		if len(bundle.Omitted) > 0 {
			fmt.Fprintf(os.Stderr,
				"wrote %s — %d files, ~%d tokens; %d files, ~%d tokens omitted by the budget\n",
				*output, len(bundle.Files), bundle.TotalTokens,
				len(bundle.Omitted), bundle.OmittedTokens)
		} else {
			fmt.Fprintf(os.Stderr, "wrote %s — %d files, ~%d tokens\n",
				*output, len(bundle.Files), bundle.TotalTokens)
		}
	}
	return 0
}

// --- diff ---

// runPack calls packer.Pack. It is a variable so a test can reach cmdDiff's
// pack-error branch without needing a directory that `git rev-parse` accepts
// and `walker.Walk` refuses — a condition no test can produce reliably, since
// Pack silently ignores any path in Options.Files the walk did not reach, and
// a path that failed os.Stat would already have failed ChangedFiles.
var runPack = func(path string, opts packer.Options) (*format.Bundle, error) {
	return packer.Pack(path, opts)
}

func cmdDiff(args []string) int {
	fs := flag.NewFlagSet("diff", flag.ContinueOnError)
	fs.Usage = func() { printHelp(os.Stderr) }
	var (
		fmtFlag  = fs.String("format", "xml", "output format")
		includes stringList
		excludes stringList
		maxSize  = fs.Int64("max-size", 0, "read no more than N bytes of a file (larger files are still listed)")
		noGit    = fs.Bool("no-gitignore", false, "ignore .gitignore")
		hidden   = fs.Bool("hidden", false, "include dotfiles")
		depth    = fs.Int("depth", 0, "limit traversal to N levels below root (0 = unlimited)")
		ref      = fs.String("ref", "WORKTREE", "base git ref")
		budget   = fs.Int("budget", 0, "cap output to ~N tokens")
		model    = fs.String("model", "", "annotate fit for a model")
		output   = fs.String("output", "", "write to FILE")
		quiet    = fs.Bool("quiet", false, "suppress stderr status messages")
	)
	fs.Var(&includes, "include", "include glob (repeatable)")
	fs.Var(&excludes, "exclude", "exclude glob (repeatable)")
	fs.StringVar(output, "o", "", "shorthand for --output")
	fs.BoolVar(quiet, "q", false, "shorthand for --quiet")
	if err := fs.Parse(reorderArgs(fs, args)); err != nil {
		return 2
	}
	path := "."
	if fs.NArg() > 0 {
		path = fs.Arg(0)
	}

	outFmt, err := parseFormat(*fmtFlag)
	if err != nil {
		fmt.Fprintln(os.Stderr, "ctxpack:", err)
		return 2
	}

	changed, err := gitutil.ChangedFiles(path, *ref)
	if err != nil {
		if err == gitutil.ErrNotARepo {
			fmt.Fprintln(os.Stderr, "ctxpack: not a git repository; 'diff' requires git")
			return 1
		}
		fmt.Fprintln(os.Stderr, "ctxpack:", err)
		return 1
	}
	if len(changed) == 0 {
		fmt.Fprintf(os.Stderr, "no changed files vs %q\n", *ref)
		return 0
	}

	bundle, err := runPack(path, packer.Options{
		Walker: walker.Options{
			Include:          []string(includes),
			Exclude:          []string(excludes),
			MaxFileSize:      *maxSize,
			MaxDepth:         *depth,
			RespectGitignore: !*noGit,
			IncludeHidden:    *hidden,
			ReadContent:      true,
		},
		Budget: *budget,
		Files:  changed,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "ctxpack:", err)
		return 1
	}
	out := format.Render(bundle, outFmt)
	// A JSON bundle must parse as JSON, so the diff header (and the fit note)
	// cannot be prepended as an HTML comment. For other formats the comment is
	// the natural carrier.
	header := ""
	if outFmt != format.JSON {
		header = fmt.Sprintf("<!-- ctxpack diff vs %q: %d files -->\n", *ref, len(changed))
		if *model != "" {
			header += annotateFit(bundle.TotalTokens, *model) + "\n"
		}
	} else if !*quiet {
		fmt.Fprintf(os.Stderr, "ctxpack diff vs %q: %d files\n", *ref, len(changed))
		if *model != "" {
			fmt.Fprintln(os.Stderr, annotateFit(bundle.TotalTokens, *model))
		}
	}
	if err := writeOutput(*output, header+out); err != nil {
		fmt.Fprintln(os.Stderr, "ctxpack:", err)
		return 1
	}
	return 0
}

// --- map ---

func cmdMap(args []string) int {
	fs := flag.NewFlagSet("map", flag.ContinueOnError)
	fs.Usage = func() { printHelp(os.Stderr) }
	var (
		includes stringList
		excludes stringList
		maxSize  = fs.Int64("max-size", 0, "read no more than N bytes of a file (larger files are still listed)")
		noGit    = fs.Bool("no-gitignore", false, "ignore .gitignore")
		hidden   = fs.Bool("hidden", false, "include dotfiles")
		depth    = fs.Int("depth", 0, "limit traversal to N levels below root (0 = unlimited)")
		jsonOut  = fs.Bool("json", false, "print the tree as JSON instead of the text outline")
		sortBy   = fs.String("sort", "name", "sort children by: name, tokens, bytes")
		topN     = fs.Int("top", 0, "show only the N largest files (flat list, ignores tree)")
		csvOut   = fs.Bool("csv", false, "output a flat CSV list of all files (path, tokens, bytes)")
	)
	fs.Var(&includes, "include", "include glob (repeatable)")
	fs.Var(&excludes, "exclude", "exclude glob (repeatable)")
	if err := fs.Parse(reorderArgs(fs, args)); err != nil {
		return 2
	}
	path := "."
	if fs.NArg() > 0 {
		path = fs.Arg(0)
	}
	root, tokens, bytes, err := repomap.Build(path, repomap.Options{
		Walker: walker.Options{
			Include:          []string(includes),
			Exclude:          []string(excludes),
			MaxFileSize:      *maxSize,
			MaxDepth:         *depth,
			RespectGitignore: !*noGit,
			IncludeHidden:    *hidden,
			ReadContent:      true,
		},
		SortBy: *sortBy,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "ctxpack:", err)
		return 1
	}
	if *jsonOut {
		return writeEnvelope(os.Stdout, mapEnvelope{
			Root: root.Name, TotalTokens: tokens, TotalBytes: bytes, Tree: toJSONNode(root),
		})
	}
	if *topN > 0 {
		files := collectFiles(root, "")
		sort.Slice(files, func(i, j int) bool {
			switch *sortBy {
			case "bytes":
				return files[i].Bytes > files[j].Bytes
			default:
				return files[i].Tokens > files[j].Tokens
			}
		})
		if *topN < len(files) {
			files = files[:*topN]
		}
		fmt.Printf("Top %d files by %s (of %d total):\n\n", len(files), *sortBy, countFiles(root))
		fmt.Printf("%-45s %10s %10s\n", "PATH", "TOKENS", "BYTES")
		fmt.Printf("%-45s %10s %10s\n", strings.Repeat("-", 45), strings.Repeat("-", 10), strings.Repeat("-", 10))
		for _, f := range files {
			fmt.Printf("%-45s %10d %10d\n", f.RelPath, f.Tokens, f.Bytes)
		}
		return 0
	}
	if *csvOut {
		files := collectFiles(root, "")
		sort.Slice(files, func(i, j int) bool {
			switch *sortBy {
			case "bytes":
				return files[i].Bytes > files[j].Bytes
			default:
				return files[i].Tokens > files[j].Tokens
			}
		})
		w := csv.NewWriter(os.Stdout)
		w.Write([]string{"path", "tokens", "bytes"})
		for _, f := range files {
			w.Write([]string{f.RelPath, strconv.Itoa(f.Tokens), strconv.Itoa(f.Bytes)})
		}
		w.Flush()
		return 0
	}
	fmt.Printf("Repository: %s\nFiles: ~%d tokens, %s\n\n", root.Name, tokens, humanBytes(bytes))
	fmt.Print(repomap.Render(root))
	return 0
}

// --- tokens ---

func cmdTokens(args []string) int {
	fs := flag.NewFlagSet("tokens", flag.ContinueOnError)
	fs.Usage = func() { printHelp(os.Stderr) }
	var (
		includes stringList
		excludes stringList
		maxSize  = fs.Int64("max-size", 0, "read no more than N bytes of a file (larger files are still listed)")
		noGit    = fs.Bool("no-gitignore", false, "ignore .gitignore")
		hidden   = fs.Bool("hidden", false, "include dotfiles")
		depth    = fs.Int("depth", 0, "limit traversal to N levels below root (0 = unlimited)")
		jsonOut  = fs.Bool("json", false, "print the summary and per-model fit as JSON")
		model    = fs.String("model", "", "show fit for one model only")
		sortBy   = fs.String("sort", "name", "sort fit table by: name (default), pct, window")
	)
	fs.Var(&includes, "include", "include glob (repeatable)")
	fs.Var(&excludes, "exclude", "exclude glob (repeatable)")
	if err := fs.Parse(reorderArgs(fs, args)); err != nil {
		return 2
	}
	path := "."
	if fs.NArg() > 0 {
		path = fs.Arg(0)
	}
	root, tokens, bytes, err := repomap.Build(path, repomap.Options{
		Walker: walker.Options{
			Include:          []string(includes),
			Exclude:          []string(excludes),
			MaxFileSize:      *maxSize,
			MaxDepth:         *depth,
			RespectGitignore: !*noGit,
			IncludeHidden:    *hidden,
			ReadContent:      true,
		},
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "ctxpack:", err)
		return 1
	}
	if *jsonOut {
		fits := tokenFits(tokens)
		if *model != "" {
			fits = filterFits(fits, *model)
		}
		sortFits(fits, *sortBy)
		return writeEnvelope(os.Stdout, tokensEnvelope{
			Path:          root.Name,
			TotalTokens:   tokens,
			TotalBytes:    bytes,
			ReserveTokens: fitReserve,
			Fits:          fits,
		})
	}
	if *model != "" {
		if _, ok := counter.LookupModel(*model); !ok {
			fmt.Fprintln(os.Stderr, "ctxpack: unknown model", *model)
			return 2
		}
	}
	fmt.Printf("Path:       %s\n", root.Name)
	fmt.Printf("Tokens:     ~%d\n", tokens)
	fmt.Printf("Bytes:      %s\n", humanBytes(bytes))
	fmt.Println()
	if *model != "" {
		m, _ := counter.LookupModel(*model)
		fit := counter.FitsModel(counter.Estimate{Tokens: tokens}, m, fitReserve)
		mark := "fits"
		if !fit.Fits {
			mark = "OVERFLOW"
		}
		fmt.Printf("  [%s] %-22s %s / %s (%.0f%%)\n", mark, m.Name,
			humanTokens(fit.Used), humanTokens(fit.Limit), fit.PctUsed)
	} else {
		models := counter.Models()
		fits := make([]struct {
			Model counter.Model
			Fit   counter.Fit
		}, len(models))
		for i, m := range models {
			fits[i] = struct {
				Model counter.Model
				Fit   counter.Fit
			}{Model: m, Fit: counter.FitsModel(counter.Estimate{Tokens: tokens}, m, fitReserve)}
		}
		sort.Slice(fits, func(i, j int) bool {
			switch *sortBy {
			case "pct":
				if fits[i].Fit.PctUsed != fits[j].Fit.PctUsed {
					return fits[i].Fit.PctUsed > fits[j].Fit.PctUsed
				}
				return fits[i].Model.Name < fits[j].Model.Name
			case "window":
				if fits[i].Model.ContextWindow != fits[j].Model.ContextWindow {
					return fits[i].Model.ContextWindow > fits[j].Model.ContextWindow
				}
				return fits[i].Model.Name < fits[j].Model.Name
			default:
				return fits[i].Model.Name < fits[j].Model.Name
			}
		})
		fmt.Println("Per-model fit (est. tokens / context window):")
		for _, f := range fits {
			mark := "fits"
			if !f.Fit.Fits {
				mark = "OVERFLOW"
			}
			fmt.Printf("  [%s] %-22s %s / %s (%.0f%%)\n", mark, f.Model.Name,
				humanTokens(f.Fit.Used), humanTokens(f.Fit.Limit), f.Fit.PctUsed)
		}
	}
	return 0
}

// --- models ---

func cmdModels(args []string) int {
	// Handled before the FlagSet so that "-h" keeps returning 0 here, unlike the
	// other commands where an undefined flag is a parse error.
	if len(args) > 0 && (args[0] == "-h" || args[0] == "--help") {
		fmt.Println("usage: ctxpack models")
		return 0
	}
	fs := flag.NewFlagSet("models", flag.ContinueOnError)
	fs.Usage = func() { printHelp(os.Stderr) }
	jsonOut := fs.Bool("json", false, "print the model table as JSON")
	vendor := fs.String("vendor", "", "show only models from this vendor")
	if err := fs.Parse(reorderArgs(fs, args)); err != nil {
		return 2
	}
	// Before this used a manual args[0] check and silently ignored everything
	// else, so `ctxpack models --bogus` and `ctxpack models --json` were both
	// indistinguishable from `ctxpack models`. Rejecting stray arguments is what
	// makes --json mean anything here.
	if fs.NArg() > 0 {
		fmt.Fprintln(os.Stderr, "ctxpack: unexpected argument", fs.Arg(0))
		return 2
	}
	models := counter.Models()
	if *vendor != "" {
		models = filterByVendor(models, *vendor)
		if len(models) == 0 {
			fmt.Fprintln(os.Stderr, "ctxpack: no models for vendor", *vendor)
			return 2
		}
	}
	if *jsonOut {
		return writeEnvelope(os.Stdout, modelsEnvelope{Models: modelJSON(models)})
	}
	fmt.Println("Known models (name — context window):")
	for _, m := range models {
		fmt.Printf("  %-22s %s (%s)\n", m.Name, humanTokens(m.ContextWindow), m.Vendor)
	}
	return 0
}

// filterByVendor returns only models whose vendor matches (case-insensitive).
func filterByVendor(models []counter.Model, vendor string) []counter.Model {
	v := strings.ToLower(vendor)
	out := make([]counter.Model, 0, len(models))
	for _, m := range models {
		if strings.ToLower(m.Vendor) == v {
			out = append(out, m)
		}
	}
	return out
}

// --- mcp ---

// --- doctor ---

func cmdDoctor(args []string) int {
	fs := flag.NewFlagSet("doctor", flag.ContinueOnError)
	fs.Usage = func() { printHelp(os.Stderr) }
	jsonOut := fs.Bool("json", false, "output diagnostics as JSON")
	if err := fs.Parse(reorderArgs(fs, args)); err != nil {
		return 2
	}
	if fs.NArg() > 0 {
		fmt.Fprintln(os.Stderr, "ctxpack: unexpected argument", fs.Arg(0))
		return 2
	}

	info := map[string]any{
		"version":    version.Info(),
		"go_version": runtime.Version(),
		"platform":   runtime.GOOS + "/" + runtime.GOARCH,
	}

	// Check git availability.
	gitPath, err := exec.LookPath("git")
	if err != nil {
		info["git"] = "not found"
		info["git_error"] = err.Error()
	} else {
		info["git"] = gitPath
		out, err := exec.Command("git", "--version").Output()
		if err != nil {
			info["git_version"] = "unknown"
			info["git_version_error"] = err.Error()
		} else {
			info["git_version"] = strings.TrimSpace(string(out))
		}
	}

	models := counter.Models()
	info["model_count"] = len(models)
	info["model_vendors"] = countVendors(models)

	if *jsonOut {
		return writeEnvelope(os.Stdout, info)
	}

	fmt.Println("ctxpack diagnostics:")
	fmt.Printf("  Version:   %s\n", version.Info())
	fmt.Printf("  Go:        %s\n", runtime.Version())
	fmt.Printf("  Platform:  %s\n", runtime.GOOS+"/"+runtime.GOARCH)
	if gitPath != "" {
		gitVer, _ := info["git_version"].(string)
		if gitVer == "" {
			gitVer = "unknown"
		}
		fmt.Printf("  Git:       %s (%s)\n", gitPath, gitVer)
	} else {
		fmt.Println("  Git:       not found (diff command will not work)")
	}
	fmt.Printf("  Models:    %d models, %d vendors\n", len(models), info["model_vendors"])
	return 0
}

func countVendors(models []counter.Model) int {
	seen := make(map[string]bool)
	for _, m := range models {
		seen[m.Vendor] = true
	}
	return len(seen)
}

// runMCP runs the Model Context Protocol server on stdio. It is a variable so
// tests can observe that "ctxpack mcp" dispatches without having to drive a
// live JSON-RPC loop over os.Stdin.
var runMCP = func(args []string) int { return cmdMCP(args, os.Stdin, os.Stdout) }

// cmdMCP parses the mcp subcommand and serves on the given streams.
func cmdMCP(args []string, in io.Reader, out io.Writer) int {
	fs := flag.NewFlagSet("mcp", flag.ContinueOnError)
	fs.Usage = func() { printHelp(os.Stderr) }
	if err := fs.Parse(reorderArgs(fs, args)); err != nil {
		return 2
	}
	if err := mcp.Serve(in, out, version.Version); err != nil {
		fmt.Fprintln(os.Stderr, "ctxpack mcp:", err)
		return 1
	}
	return 0
}

// --- helpers ---

// reorderArgs moves positional arguments to the end of the slice.
//
// The stdlib flag package stops parsing at the first non-flag token, so
// `ctxpack pack ./repo --format markdown` would otherwise silently ignore every
// flag that came after the path. Reordering means the documented usage works
// and users do not have to learn a flag-before-path rule.
func reorderArgs(fs *flag.FlagSet, args []string) []string {
	out := make([]string, 0, len(args))
	var positional []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "-" || (len(a) > 1 && a[0] == '-' && strings.Contains(a, "=")) {
			out = append(out, a)
			continue
		}
		if len(a) > 1 && a[0] == '-' {
			out = append(out, a)
			// "-f value" form: carry the value along - but only when the flag
			// actually takes one. A boolean flag such as --hidden is not
			// followed by a value, and treating the path as its value would
			// swallow it and stop flag parsing for the rest of the command.
			if takesValue(fs, a) && i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
				out = append(out, args[i+1])
				i++
			}
			continue
		}
		positional = append(positional, a)
	}
	return append(out, positional...)
}

// takesValue reports whether the flag token f (such as --format or -o) expects
// a value, as opposed to a boolean flag that stands alone.
func takesValue(fs *flag.FlagSet, f string) bool {
	name := strings.TrimLeft(f, "-")
	if eq := strings.IndexByte(name, '='); eq >= 0 {
		name = name[:eq]
	}
	fl := fs.Lookup(name)
	if fl == nil {
		// Unknown: do not carry, so flag reports "not defined" instead of
		// "needs an argument" for what may have been a positional.
		return false
	}
	// The flag package keeps IsBoolFlag() on an unexported interface, so it
	// cannot be queried from here. Every built-in bool flag stores a bool, so
	// asking its Value for the current value and checking the dynamic type
	// identifies them exactly.
	if g, ok := fl.Value.(interface{ Get() any }); ok {
		if _, isBool := g.Get().(bool); isBool {
			return false
		}
	}
	return true
}

// parseFormat validates the requested output format. An unknown value is an
// error rather than a silent fallback: a typo such as --format jons must not
// quietly produce XML when JSON was asked for.
func parseFormat(s string) (format.Format, error) {
	switch strings.ToLower(s) {
	case "xml":
		return format.XML, nil
	case "md", "markdown":
		return format.Markdown, nil
	case "json":
		return format.JSON, nil
	case "text", "txt", "plain", "raw":
		return format.Text, nil
	}
	return format.XML, fmt.Errorf("unknown format %q (want xml, markdown, json or text)", s)
}

// fitReserve is the token budget held back for a model's reply, so a bundle that
// just fills a window still leaves room to answer. It aliases
// counter.ReplyReserve rather than restating it, so the text table, this
// package's JSON output and the MCP server all read one value.
const fitReserve = counter.ReplyReserve

// mapEnvelope is the JSON form of `ctxpack map`. The text outline is for
// people; this shape is for scripts that want to rank files or pick a budget.
type mapEnvelope struct {
	Root        string    `json:"root"`
	TotalTokens int       `json:"total_tokens"`
	TotalBytes  int       `json:"total_bytes"`
	Tree        *jsonNode `json:"tree"`
}

// tokensEnvelope is the JSON form of `ctxpack tokens`.
type tokensEnvelope struct {
	Path          string     `json:"path"`
	TotalTokens   int        `json:"total_tokens"`
	TotalBytes    int        `json:"total_bytes"`
	ReserveTokens int        `json:"reserve_tokens"`
	Fits          []fitEntry `json:"fits"`
}

// fitEntry is one model's fit against a token total.
type fitEntry struct {
	Model   string  `json:"model"`
	Used    int     `json:"used"`
	Limit   int     `json:"limit"`
	Fits    bool    `json:"fits"`
	PctUsed float64 `json:"pct_used"`
}

// modelsEnvelope is the JSON form of `ctxpack models`.
type modelsEnvelope struct {
	Models []modelEntry `json:"models"`
}

// modelEntry is one registered model. Limit is the context window minus the
// reply reserve, the same value tokens --json reports per fit, so a script can
// compare the two without recomputing the subtraction.
type modelEntry struct {
	Name          string `json:"name"`
	Vendor        string `json:"vendor"`
	ContextWindow int    `json:"context_window"`
	Limit         int    `json:"limit"`
}

// jsonNode mirrors repomap.Node for JSON output. Children is emitted even when
// empty, so a consumer can tell a file ([] ) from an empty directory without
// guessing from a missing field.
type jsonNode struct {
	Name     string      `json:"name"`
	IsDir    bool        `json:"is_dir"`
	Tokens   int         `json:"tokens"`
	Bytes    int         `json:"bytes"`
	Children []*jsonNode `json:"children"`
}

// toJSONNode converts a repomap tree into its JSON form. Children is always a
// slice, never null: a file and an empty directory both get [] and the is_dir
// field is what distinguishes them.
func toJSONNode(n *repomap.Node) *jsonNode {
	out := &jsonNode{
		Name:     n.Name,
		IsDir:    n.IsDir,
		Tokens:   n.Tokens,
		Bytes:    n.Bytes,
		Children: []*jsonNode{},
	}
	if n.Children != nil {
		out.Children = make([]*jsonNode, len(n.Children))
		for i, c := range n.Children {
			out.Children[i] = toJSONNode(c)
		}
	}
	return out
}

// tokenFits reports each known model's fit against a total token count.
func tokenFits(tokens int) []fitEntry {
	models := counter.Models()
	out := make([]fitEntry, 0, len(models))
	for _, m := range models {
		f := counter.FitsModel(counter.Estimate{Tokens: tokens}, m, fitReserve)
		out = append(out, fitEntry{
			Model:   m.Name,
			Used:    f.Used,
			Limit:   f.Limit,
			Fits:    f.Fits,
			PctUsed: round2(f.PctUsed),
		})
	}
	return out
}

// filterFits returns only the entries whose model name matches. An unknown
// name yields an empty slice so the JSON envelope stays well-formed.
func filterFits(fits []fitEntry, name string) []fitEntry {
	out := make([]fitEntry, 0, 1)
	for _, f := range fits {
		if f.Model == name {
			out = append(out, f)
		}
	}
	return out
}

func sortFits(fits []fitEntry, sortBy string) {
	sort.Slice(fits, func(i, j int) bool {
		switch sortBy {
		case "pct":
			if fits[i].PctUsed != fits[j].PctUsed {
				return fits[i].PctUsed > fits[j].PctUsed
			}
			return fits[i].Model < fits[j].Model
		case "window":
			if fits[i].Limit != fits[j].Limit {
				return fits[i].Limit > fits[j].Limit
			}
			return fits[i].Model < fits[j].Model
		default:
			return fits[i].Model < fits[j].Model
		}
	})
}

// modelJSON converts the registered models into their JSON form.
func modelJSON(models []counter.Model) []modelEntry {
	out := make([]modelEntry, len(models))
	for i, m := range models {
		out[i] = modelEntry{
			Name:          m.Name,
			Vendor:        m.Vendor,
			ContextWindow: m.ContextWindow,
			Limit:         m.ContextWindow - fitReserve,
		}
	}
	return out
}

// round2 rounds to two decimals. Two is far more precision than an estimated
// fit percentage warrants, and it keeps the JSON readable instead of echoing
// float64 noise such as 1258.287899747742.
func round2(v float64) float64 {
	return math.Round(v*100) / 100
}

// writeEnvelope marshals v to w as indented JSON and returns the exit code: 0
// on success, 1 if marshalling fails. That failure would be a bug in the shape,
// because every field is a string, an int, a bool or a slice of those.
func writeEnvelope(w io.Writer, v any) int {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		fmt.Fprintln(os.Stderr, "ctxpack:", err)
		return 1
	}
	return 0
}

func annotateFit(tokens int, model string) string {
	m, ok := counter.LookupModel(model)
	if !ok {
		return fmt.Sprintf("<!-- unknown model %q; run `ctxpack models` for known models -->", model)
	}
	fit := counter.FitsModel(counter.Estimate{Tokens: tokens}, m, fitReserve)
	mark := "FITS"
	if !fit.Fits {
		mark = "OVERFLOW"
	}
	return fmt.Sprintf("<!-- fit: %s model=%s used=%s/%s (%.0f%%) %s -->",
		mark, m.Name, humanTokens(fit.Used), humanTokens(fit.Limit), fit.PctUsed, mark)
}

// writeOutput writes data to dest, or stdout when dest is "" or "-".
func writeOutput(dest, data string) error {
	if dest == "" || dest == "-" {
		fmt.Print(data)
		return nil
	}
	return os.WriteFile(dest, []byte(data), 0o644)
}

func humanTokens(n int) string {
	if n < 1000 {
		return strconv.Itoa(n)
	}
	if n < 1_000_000 {
		return fmt.Sprintf("%.1fk", float64(n)/1000)
	}
	return fmt.Sprintf("%.1fM", float64(n)/1_000_000)
}

func humanBytes(n int) string {
	if n < 1024 {
		return fmt.Sprintf("%d B", n)
	}
	if n < 1024*1024 {
		return fmt.Sprintf("%.1f KB", float64(n)/1024)
	}
	return fmt.Sprintf("%.1f MB", float64(n)/1024/1024)
}

func envStrDefault(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}

func envInt(k string) int {
	if v := os.Getenv(k); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return 0
}

type flatFile struct {
	RelPath string
	Tokens  int
	Bytes   int
}

func collectFiles(n *repomap.Node, prefix string) []flatFile {
	if n == nil || n.IsDir {
		var out []flatFile
		prefix += n.Name + "/"
		for _, c := range n.Children {
			out = append(out, collectFiles(c, prefix)...)
		}
		return out
	}
	return []flatFile{{RelPath: prefix + n.Name, Tokens: n.Tokens, Bytes: n.Bytes}}
}

func countFiles(n *repomap.Node) int {
	if n == nil {
		return 0
	}
	if !n.IsDir {
		return 1
	}
	count := 0
	for _, c := range n.Children {
		count += countFiles(c)
	}
	return count
}
