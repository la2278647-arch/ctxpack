package cli

import (
	"bytes"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// flagMatrix lists the flags each command advertises in printHelp. It is the
// executable form of the "FLAGS (...)" sections: one test runs every command
// against the whole union and asserts that the
// advertised flags parse (exit 0) and the non-advertised ones are rejected by
// the parser (exit 2).
//
// This is the regression guard for the drift that shipped v0.1.7: printHelp
// listed "-o, --output" as "(all commands)" while cmdDoctor registered only
// the long form, and it described the walk flags as "(map / tokens only)"
// when four commands accept them. Every entry here was verified by running
// the command.
var flagMatrix = map[string][]string{
	"pack": {
		"--format", "--include", "--exclude", "--max-size", "--no-gitignore",
		"--hidden", "--depth", "--budget", "--model", "--output", "-o",
		"--dry-run", "-q",
	},
	"diff": {
		"--format", "--include", "--exclude", "--max-size", "--no-gitignore",
		"--hidden", "--depth", "--budget", "--model", "--output", "-o",
		"--dry-run", "-q", "--ref", "--list",
	},
	"map": {
		"--format", "--include", "--exclude", "--max-size", "--no-gitignore",
		"--hidden", "--depth", "--json", "--sort", "--top", "--csv",
		"--output", "-o",
	},
	"tokens": {
		"--format", "--include", "--exclude", "--max-size", "--no-gitignore",
		"--hidden", "--depth", "--json", "--sort", "--top", "--model",
		"--output", "-o",
	},
	"models": {
		"--format", "--json", "--sort", "--top", "--vendor", "--output", "-o",
	},
	"doctor": {
		"--format", "--json", "--top", "--output", "-o",
	},
}

// walkFlags are the traversal flags, documented once in the WALK FLAGS group
// rather than repeated four times. Every command in flagMatrix that accepts
// them must be named in that group's header.
var walkFlags = []string{
	"--include", "--exclude", "--max-size", "--no-gitignore", "--hidden", "--depth",
}

var commands = []string{"pack", "diff", "map", "tokens", "models", "doctor"}

func commandFor(name string) func([]string) int {
	switch name {
	case "pack":
		return cmdPack
	case "diff":
		return cmdDiff
	case "map":
		return cmdMap
	case "tokens":
		return cmdTokens
	case "models":
		return cmdModels
	case "doctor":
		return cmdDoctor
	}
	panic("unknown command " + name)
}

// tokensFor renders one flag name into an argument slice that isolates the
// flag name as the variable: value-taking flags get a legal value, so a
// rejection is about the name, not a malformed value.
func tokensFor(flag string, tmp string) []string {
	switch flag {
	case "--format":
		return []string{"--format", "json"}
	case "--include":
		return []string{"--include", "*.md"}
	case "--exclude":
		return []string{"--exclude", "*.md"}
	case "--max-size":
		return []string{"--max-size", "100"}
	case "--depth":
		return []string{"--depth", "1"}
	case "--budget":
		return []string{"--budget", "100"}
	case "--model":
		return []string{"--model", "gpt-4o"}
	case "--sort":
		return []string{"--sort", "name"}
	case "--top":
		return []string{"--top", "3"}
	case "--vendor":
		return []string{"--vendor", "anthropic"}
	case "--ref":
		return []string{"--ref", "HEAD"}
	case "--output", "-o":
		return []string{flag, filepath.Join(tmp, "out.txt")}
	case "--no-gitignore", "--hidden", "--json", "--csv", "--list", "--dry-run", "-q":
		return []string{flag}
	}
	return []string{flag}
}

// unionOfFlags is the union of every flag any command advertises, so the
// matrix test exercises all flags x all commands.
func unionOfFlags() []string {
	seen := map[string]bool{}
	var out []string
	for _, cmd := range commands {
		for _, f := range flagMatrix[cmd] {
			if !seen[f] {
				seen[f] = true
				out = append(out, f)
			}
		}
	}
	return out
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

func TestCommandsAdvertiseTheFlagsTheyAccept(t *testing.T) {
	co := captureStdout(t)
	c := captureStderr(t)
	defer func() {
		_ = co.Content()
		_ = c.Content()
	}()

	tmp := t.TempDir()

	// 'diff' requires a git repository and defaults to the process' current
	// directory, so this test used to pass in a git clone and fail in a source
	// archive, where the current directory is not a checkout. The published
	// source tarball is a real input — Homebrew builds from it — so give
	// 'diff' an explicit repository instead of relying on cwd. A fresh
	// repository is enough: --ref HEAD and the WORKTREE default both take the
	// working-tree path, and every other command ignores the extra argument.
	repo := newGitRepo(t)

	for _, cmd := range commands {
		fn := commandFor(cmd)
		for _, flag := range unionOfFlags() {
			accept := contains(flagMatrix[cmd], flag)
			want := 0
			if !accept {
				want = 2
			}
			toks := tokensFor(flag, tmp)
			if cmd == "diff" {
				toks = append(toks, repo)
			}
			got := fn(toks)
			if got != want {
				verb := "accept"
				if !accept {
					verb = "reject"
				}
				t.Errorf("cmd %s: %s %s (exit %d, want %d)", cmd, flag, verb, got, want)
			}
		}
	}
}

// helpBlock returns the whitespace-separated block of printHelp that starts
// with the given title line, so a flag can be looked up in the section it is
// documented under rather than anywhere in the text.
func helpBlock(help, title string) string {
	for _, block := range strings.Split(help, "\n\n") {
		if strings.HasPrefix(strings.TrimSpace(block), title) {
			return block
		}
	}
	return ""
}

// TestPrintHelpDocumentsEveryRegisteredFlag is the half of the invariant that
// catches "added a flag and forgot to document it": each advertised flag must
// appear in its command's own FLAGS section, or in the shared WALK FLAGS
// group for the traversal flags.
func TestPrintHelpDocumentsEveryRegisteredFlag(t *testing.T) {
	var help bytes.Buffer
	printHelp(&help)
	text := help.String()

	// WALK FLAGS: one block must name the four walking commands and hold all
	// six walk flags.
	walk := helpBlock(text, "WALK FLAGS")
	if walk == "" {
		t.Fatal("help has no WALK FLAGS group")
	}
	for _, cmd := range []string{"pack", "diff", "map", "tokens"} {
		if !strings.Contains(walk, cmd) {
			t.Errorf("WALK FLAGS header does not name %s", cmd)
		}
	}
	for _, f := range walkFlags {
		if !strings.Contains(walk, f) {
			t.Errorf("WALK FLAGS block does not document %s", f)
		}
	}

	for _, cmd := range commands {
		block := helpBlock(text, "FLAGS ("+cmd+")")
		if block == "" {
			t.Errorf("help has no FLAGS (%s) section", cmd)
			continue
		}
		for _, f := range flagMatrix[cmd] {
			if contains(walkFlags, f) {
				continue // documented in the WALK FLAGS group
			}
			if !strings.Contains(block, f) {
				t.Errorf("FLAGS (%s) does not document %s", cmd, f)
			}
		}
	}
}

// TestPrintHelpDoesNotDocumentUnacceptedFlags is the reverse half: no command's
// FLAGS section may name a flag the command never registered, and no walk flag
// may appear under a non-walking command. This is what would have caught the
// "(map / tokens only)" annotation for the four walking commands.
func TestPrintHelpDoesNotDocumentUnacceptedFlags(t *testing.T) {
	var help bytes.Buffer
	printHelp(&help)
	text := help.String()

	for _, cmd := range commands {
		block := helpBlock(text, "FLAGS ("+cmd+")")
		if block == "" {
			continue
		}
		for _, f := range unionOfFlags() {
			if contains(flagMatrix[cmd], f) || contains(walkFlags, f) {
				continue
			}
			if strings.Contains(block, f) {
				t.Errorf("FLAGS (%s) documents %s, which cmd%s does not register",
					cmd, f, cmd)
			}
		}
	}

	// The WALK FLAGS block must not name the two non-walking commands, or the
	// grouping lies about scope.
	walk := helpBlock(text, "WALK FLAGS")
	if walk != "" {
		if !strings.Contains(walk, "models and doctor reject these") {
			t.Error("WALK FLAGS header does not state that models and doctor reject them")
		}
	}
}

// TestWalkFlagScopeIsExact asserts the walk flags are registered on exactly
// the four walking commands, not a superset or subset. This is the structural
// fact the help text claims, pinned against the matrix rather than the prose.
func TestWalkFlagScopeIsExact(t *testing.T) {
	want := []string{"pack", "diff", "map", "tokens"}
	for _, f := range walkFlags {
		var got []string
		for _, cmd := range commands {
			if contains(flagMatrix[cmd], f) {
				got = append(got, cmd)
			}
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("%s registered on %v, want exactly %v", f, got, want)
		}
	}

	// -o and --output are the flags that must be universal.
	for _, f := range []string{"-o", "--output"} {
		var got []string
		for _, cmd := range commands {
			if contains(flagMatrix[cmd], f) {
				got = append(got, cmd)
			}
		}
		if !reflect.DeepEqual(got, commands) {
			t.Errorf("%s registered on %v, want all %d commands", f, got, len(commands))
		}
	}
}
