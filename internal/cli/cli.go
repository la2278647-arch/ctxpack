// Package cli implements the ctxpack command-line interface. It is a tiny
// stdlib-only subcommand dispatcher (no cobra/urfave) so the binary stays
// dependency-free: every operation routes through the packer/repomap/gitutil
// packages, and the MCP server in internal/mcp calls the same, so the two
// frontends cannot drift apart.
package cli

import (
	"flag"
	"fmt"
	"io"
	"os"
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
		return cmdMCP(args[1:])
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
  version              Print the build identity.
  help                 Show this help.

FLAGS (pack)
  --format F           xml|markdown|json|text (default xml)
  --include GLOB       Only include paths matching GLOB (repeatable; basename ok)
  --exclude GLOB       Exclude paths matching GLOB (repeatable)
  --max-size BYTES     Skip files larger than BYTES (e.g. 100000). 0 = unlimited
  --no-gitignore       Ignore .gitignore files (built-in defaults still apply)
  --hidden             Include dotfiles/dotdirs (.git always skipped)
  --budget N           Cap output to ~N tokens (priority-selects files)
  --model NAME         Annotate fit for a model (gpt-4o, claude-3.5-sonnet, ...)
  -o, --output FILE    Write to FILE instead of stdout

FLAGS (diff)
  --ref REF            Base git ref (default: working-tree changes). e.g. HEAD~1, main
  (also accepts --format/--include/--exclude/--budget/--model/-o)

FLAGS (map / tokens)
  --include/--exclude/--max-size/--no-gitignore/--hidden

EXAMPLES
  ctxpack pack ./myrepo --format markdown -o repo.md
  ctxpack pack . --include "*.go" --exclude "*_test.go" --model gpt-4o
  ctxpack diff . --ref main            # what changed since main
  ctxpack pack . --budget 60000 --model gpt-4o
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
		maxSize  = fs.Int64("max-size", 0, "skip files larger than N bytes")
		noGit    = fs.Bool("no-gitignore", false, "ignore .gitignore")
		hidden   = fs.Bool("hidden", false, "include dotfiles")
		budget   = fs.Int("budget", envInt("CTXPACK_BUDGET"), "cap output to ~N tokens")
		model    = fs.String("model", os.Getenv("CTXPACK_MODEL"), "annotate fit for a model")
		output   = fs.String("output", "", "write to FILE (default stdout)")
	)
	fs.Var(&includes, "include", "include glob (repeatable)")
	fs.Var(&excludes, "exclude", "exclude glob (repeatable)")
	fs.StringVar(output, "o", "", "shorthand for --output")
	if err := fs.Parse(reorderArgs(args)); err != nil {
		return 2
	}
	path := "."
	if fs.NArg() > 0 {
		path = fs.Arg(0)
	}

	bundle, err := packer.Pack(path, packer.Options{
		Walker: walker.Options{
			Include:          []string(includes),
			Exclude:          []string(excludes),
			MaxFileSize:      *maxSize,
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

	out := format.Render(bundle, parseFormat(*fmtFlag))
	header := ""
	if *model != "" {
		header = annotateFit(bundle.TotalTokens, *model) + "\n"
	}
	if err := writeOutput(*output, header+out); err != nil {
		fmt.Fprintln(os.Stderr, "ctxpack:", err)
		return 1
	}
	if *output != "" {
		fmt.Fprintf(os.Stderr, "wrote %s — %d files, ~%d tokens\n", *output, len(bundle.Files), bundle.TotalTokens)
	}
	return 0
}

// --- diff ---

func cmdDiff(args []string) int {
	fs := flag.NewFlagSet("diff", flag.ContinueOnError)
	fs.Usage = func() { printHelp(os.Stderr) }
	var (
		fmtFlag  = fs.String("format", "xml", "output format")
		includes stringList
		excludes stringList
		maxSize  = fs.Int64("max-size", 0, "skip files larger than N bytes")
		noGit    = fs.Bool("no-gitignore", false, "ignore .gitignore")
		hidden   = fs.Bool("hidden", false, "include dotfiles")
		ref      = fs.String("ref", "WORKTREE", "base git ref")
		budget   = fs.Int("budget", 0, "cap output to ~N tokens")
		model    = fs.String("model", "", "annotate fit for a model")
		output   = fs.String("output", "", "write to FILE")
	)
	fs.Var(&includes, "include", "include glob (repeatable)")
	fs.Var(&excludes, "exclude", "exclude glob (repeatable)")
	fs.StringVar(output, "o", "", "shorthand for --output")
	if err := fs.Parse(reorderArgs(args)); err != nil {
		return 2
	}
	path := "."
	if fs.NArg() > 0 {
		path = fs.Arg(0)
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

	bundle, err := packer.Pack(path, packer.Options{
		Walker: walker.Options{
			Include:          []string(includes),
			Exclude:          []string(excludes),
			MaxFileSize:      *maxSize,
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
	out := format.Render(bundle, parseFormat(*fmtFlag))
	header := fmt.Sprintf("<!-- ctxpack diff vs %q: %d files -->\n", *ref, len(changed))
	if *model != "" {
		header += annotateFit(bundle.TotalTokens, *model) + "\n"
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
		maxSize  = fs.Int64("max-size", 0, "skip files larger than N bytes")
		noGit    = fs.Bool("no-gitignore", false, "ignore .gitignore")
		hidden   = fs.Bool("hidden", false, "include dotfiles")
	)
	fs.Var(&includes, "include", "include glob (repeatable)")
	fs.Var(&excludes, "exclude", "exclude glob (repeatable)")
	if err := fs.Parse(reorderArgs(args)); err != nil {
		return 2
	}
	path := "."
	if fs.NArg() > 0 {
		path = fs.Arg(0)
	}
	root, tokens, bytes, err := repomap.Build(path, walker.Options{
		Include:          []string(includes),
		Exclude:          []string(excludes),
		MaxFileSize:      *maxSize,
		RespectGitignore: !*noGit,
		IncludeHidden:    *hidden,
		ReadContent:      true,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "ctxpack:", err)
		return 1
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
		maxSize  = fs.Int64("max-size", 0, "skip files larger than N bytes")
		noGit    = fs.Bool("no-gitignore", false, "ignore .gitignore")
		hidden   = fs.Bool("hidden", false, "include dotfiles")
	)
	fs.Var(&includes, "include", "include glob (repeatable)")
	fs.Var(&excludes, "exclude", "exclude glob (repeatable)")
	if err := fs.Parse(reorderArgs(args)); err != nil {
		return 2
	}
	path := "."
	if fs.NArg() > 0 {
		path = fs.Arg(0)
	}
	root, tokens, bytes, err := repomap.Build(path, walker.Options{
		Include:          []string(includes),
		Exclude:          []string(excludes),
		MaxFileSize:      *maxSize,
		RespectGitignore: !*noGit,
		IncludeHidden:    *hidden,
		ReadContent:      true,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "ctxpack:", err)
		return 1
	}
	fmt.Printf("Path:       %s\n", root.Name)
	fmt.Printf("Tokens:     ~%d\n", tokens)
	fmt.Printf("Bytes:      %s\n", humanBytes(bytes))
	fmt.Println()
	fmt.Println("Per-model fit (est. tokens / context window):")
	for _, m := range counter.Models() {
		fit := counter.FitsModel(counter.Estimate{Tokens: tokens}, m, 4096)
		mark := "fits"
		if !fit.Fits {
			mark = "OVERFLOW"
		}
		fmt.Printf("  [%s] %-22s %s / %s (%.0f%%)\n", mark, m.Name,
			humanTokens(fit.Used), humanTokens(fit.Limit), fit.PctUsed)
	}
	return 0
}

// --- models ---

func cmdModels(args []string) int {
	if len(args) > 0 && (args[0] == "-h" || args[0] == "--help") {
		fmt.Println("usage: ctxpack models")
		return 0
	}
	fmt.Println("Known models (name — context window):")
	for _, m := range counter.Models() {
		fmt.Printf("  %-22s %s (%s)\n", m.Name, humanTokens(m.ContextWindow), m.Vendor)
	}
	return 0
}

// --- mcp ---

func cmdMCP(args []string) int {
	fs := flag.NewFlagSet("mcp", flag.ContinueOnError)
	fs.Usage = func() { printHelp(os.Stderr) }
	if err := fs.Parse(reorderArgs(args)); err != nil {
		return 2
	}
	if err := mcp.Serve(os.Stdin, os.Stdout, version.Version); err != nil {
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
func reorderArgs(args []string) []string {
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
			// "-f value" form: carry the value along unless it is itself a flag.
			if i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
				out = append(out, args[i+1])
				i++
			}
			continue
		}
		positional = append(positional, a)
	}
	return append(out, positional...)
}

func parseFormat(s string) format.Format {
	switch strings.ToLower(s) {
	case "md", "markdown":
		return format.Markdown
	case "json":
		return format.JSON
	case "text", "txt", "plain", "raw":
		return format.Text
	default:
		return format.XML
	}
}

func annotateFit(tokens int, model string) string {
	m, ok := counter.LookupModel(model)
	if !ok {
		return fmt.Sprintf("<!-- unknown model %q; run `ctxpack models` for known models -->", model)
	}
	fit := counter.FitsModel(counter.Estimate{Tokens: tokens}, m, 4096)
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
