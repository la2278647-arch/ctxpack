// Package ignore implements .gitignore-style pattern matching.
//
// It is a self-contained (stdlib-only) implementation covering the common
// gitignore semantics: comments, blank lines, negation ("!"), leading slash
// anchoring, trailing slash directory-only matching, and the "*", "?" and
// "**" wildcards. A Matcher is composed of multiple .gitignore files layered
// from the repo root downwards, each scoped to its directory.
//
// It is intentionally pragmatic: it covers the patterns real repositories
// actually use rather than being a byte-exact reimplementation of C git's
// edge cases. Limitations are documented on the relevant methods.
package ignore

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Pattern is a single compiled gitignore pattern.
type Pattern struct {
	re      *regexp.Regexp
	negate  bool
	dirOnly bool
	raw     string
	source  string // path of the .gitignore file that defined it
}

// Matcher holds a set of layered gitignore patterns.
type Matcher struct {
	root     string
	patterns []Pattern
}

// NewMatcher creates an empty matcher rooted at root. Call Load to read the
// root .gitignore (and nested ones via the walker).
func NewMatcher(root string) *Matcher {
	// Abs only fails if os.Getwd fails, which needs the process working
	// directory deleted mid-call. Fall back to the caller's path so a
	// matcher can still be built; the paths it compares against come from the
	// same root, so they stay consistent either way.
	abs, err := filepath.Abs(root)
	if err != nil {
		abs = root
	}
	return &Matcher{root: abs}
}

// Load reads a single .gitignore file and appends its patterns, scoped to the
// directory containing that file. Missing files are ignored.
func (m *Matcher) Load(path string) error {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	defer f.Close()

	dir := filepath.Dir(path)
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		// Strip trailing whitespace that isn't escaped.
		line = strings.TrimRightFunc(line, func(r rune) bool {
			return r == ' ' || r == '\t'
		})
		// Git also strips leading whitespace, so "  # comment" is a comment. The
		// only exception is a leading backslash, which escapes the next
		// character and therefore protects the whitespace that follows it.
		if !strings.HasPrefix(line, `\`) {
			line = strings.TrimLeftFunc(line, func(r rune) bool {
				return r == ' ' || r == '\t'
			})
		}
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		negate := false
		if strings.HasPrefix(line, "!") {
			negate = true
			line = line[1:]
		}
		dirOnly := strings.HasSuffix(line, "/")
		if dirOnly {
			line = strings.TrimSuffix(line, "/")
		}
		// Remove a leading backslash before a leading # or ! (escape).
		line = strings.TrimPrefix(line, `\`)

		re, err := compilePattern(line)
		if err != nil {
			// Skip unparseable patterns rather than aborting the whole file.
			// Unreachable today: translateGlob escapes every regex-special
			// character, so regexp.Compile cannot fail on its output. Kept so
			// a future change that lets a pattern through raw cannot silently
			// drop the rest of the file.
			continue
		}
		m.patterns = append(m.patterns, Pattern{
			re:      re,
			negate:  negate,
			dirOnly: dirOnly,
			raw:     line,
			source:  dir,
		})
	}
	return scanner.Err()
}

// Match reports whether relPath (a slash-separated path relative to the
// matcher root) is ignored. isDir indicates whether the path is a directory.
//
// Evaluation follows gitignore precedence: the last matching pattern wins,
// and a negation pattern re-includes a path that an earlier pattern excluded.
// Patterns are only considered when their owning .gitignore directory is an
// ancestor of (or equal to) the path's directory.
func (m *Matcher) Match(relPath string, isDir bool) bool {
	rootRel := filepath.ToSlash(relPath)
	if rootRel == "" {
		// No path, no verdict. Without this, "*" and "**" compile to regexes
		// that match the empty string, so an empty relPath would report as
		// ignored.
		return false
	}

	// Subjects to test: each ancestor directory plus the path itself. This
	// makes an excluded directory exclude its contents (a core gitignore
	// guarantee) even when the matcher is used in isolation.
	type subject struct {
		path  string
		isDir bool
	}
	var subjects []subject
	parts := strings.Split(rootRel, "/")
	acc := ""
	for i := 0; i < len(parts)-1; i++ {
		if acc == "" {
			acc = parts[i]
		} else {
			acc += "/" + parts[i]
		}
		subjects = append(subjects, subject{acc, true})
	}
	subjects = append(subjects, subject{rootRel, isDir})

	matched := false
	for _, p := range m.patterns {
		relDir := m.relSource(p.source)
		for _, s := range subjects {
			// Scope: pattern's source dir must be an ancestor of the subject.
			curRel := s.path
			if relDir != "" && relDir != "." {
				prefix := relDir + "/"
				if !strings.HasPrefix(s.path+"/", prefix) {
					continue
				}
				curRel = strings.TrimPrefix(s.path, prefix)
			}
			if p.dirOnly && !s.isDir {
				continue
			}
			if p.re.MatchString(curRel) {
				// Last matching pattern wins (negation re-includes).
				matched = !p.negate
			}
		}
	}
	return matched
}

// relSource returns sourceDir relative to the matcher root.
func (m *Matcher) relSource(sourceDir string) string {
	rel, err := filepath.Rel(m.root, sourceDir)
	if err != nil {
		return ""
	}
	return filepath.ToSlash(rel)
}

// CompileGlob compiles a single glob pattern (gitignore-style: "*", "?", "**",
// leading "/" anchor, trailing "/" directory-only ignored here) into a regex
// that matches a full slash-relative path. A pattern with no "/" is treated as
// a basename match (applies at any depth); a pattern containing "/" is
// anchored to the root.
func CompileGlob(pattern string) (*regexp.Regexp, error) {
	return compilePattern(pattern)
}

// compilePattern converts a single gitignore glob into a compiled regex that
// matches a slash-relative path string. The conversion handles the common
// cases: leading "/" anchor, internal "/", "**", "*", and "?".
func compilePattern(pattern string) (*regexp.Regexp, error) {
	anchored := strings.HasPrefix(pattern, "/")
	if anchored {
		pattern = pattern[1:]
	}
	var b strings.Builder
	b.WriteString("^")
	if !anchored && !strings.Contains(pattern, "/") {
		// Basename pattern: match at any depth. Allow an optional prefix path.
		b.WriteString(`(?:.*/)?`)
	}
	b.WriteString(translateGlob(pattern))
	b.WriteString("$")
	return regexp.Compile(b.String())
}

// translateGlob converts a gitignore glob fragment into a regex fragment.
// "**" becomes ".*" (matching across "/"), "*" becomes "[^/]*", "?" becomes
// "[^/]". Other characters are regex-escaped.
func translateGlob(g string) string {
	var b strings.Builder
	runes := []rune(g)
	for i := 0; i < len(runes); i++ {
		r := runes[i]
		switch r {
		case '*':
			if i+1 < len(runes) && runes[i+1] == '*' {
				// "**" — match across path separators.
				i++
				if i+1 < len(runes) && runes[i+1] == '/' {
					// "**/" — zero or more intermediate directories. The
					// separator is kept, so "a/**/b" matches "a/b" and "a/x/b"
					// but not "a/xby".
					i++
					b.WriteString(`(?:.*/)?`)
				} else {
					// "**" at the end of a segment, or on its own: anything at
					// any depth. "a/**" becomes "a/.*".
					b.WriteString(".*")
				}
			} else {
				b.WriteString("[^/]*")
			}
		case '?':
			b.WriteString("[^/]")
		case '.', '+', '(', ')', '{', '}', '^', '$', '|', '\\', '[', ']':
			b.WriteByte('\\')
			b.WriteRune(r)
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// String returns a short human description for debugging.
func (p Pattern) String() string {
	perm := "allow"
	if p.negate {
		perm = "unignore"
	}
	return fmt.Sprintf("%s %q (dirOnly=%v)", perm, p.raw, p.dirOnly)
}
