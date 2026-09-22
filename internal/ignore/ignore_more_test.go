package ignore

import (
	"os"
	"path/filepath"
	"testing"
)

func mustWriteIgnore(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func mustLoad(t *testing.T, m *Matcher, dir, rel string) {
	t.Helper()
	if err := m.Load(filepath.Join(dir, rel)); err != nil {
		t.Fatalf("Load(%s) = %v", rel, err)
	}
}

// --- Loader hygiene ---

func TestLoadSkipsBlankAndComments(t *testing.T) {
	dir := t.TempDir()
	gi := filepath.Join(dir, ".gitignore")
	body := "\n" +
		"# a comment\n" +
		"   \t  \n" + // whitespace only
		"a.log  \n" + // trailing spaces are trimmed
		"\t# another comment\n" +
		"b.log\n"
	mustWriteIgnore(t, gi, body)

	m := NewMatcher(dir)
	if err := m.Load(gi); err != nil {
		t.Fatal(err)
	}
	// Exactly two patterns: a.log and b.log. Blank, whitespace-only and
	// commented lines must not become patterns.
	if got := len(m.patterns); got != 2 {
		t.Fatalf("len(patterns) = %d, want 2; got %v", got, describe(m))
	}
	if !m.Match("a.log", false) || !m.Match("b.log", false) {
		t.Error("expected both a.log and b.log to be ignored")
	}
	if m.Match("c.log", false) {
		t.Error("c.log must not be ignored")
	}
}

func TestLoadTrailingWhitespaceOnlyPatternIsDropped(t *testing.T) {
	dir := t.TempDir()
	gi := filepath.Join(dir, ".gitignore")
	mustWriteIgnore(t, gi, "   \t\nbuild\n")
	m := NewMatcher(dir)
	mustLoad(t, m, dir, ".gitignore")
	if got := len(m.patterns); got != 1 {
		t.Fatalf("len(patterns) = %d, want 1", got)
	}
	if !m.Match("build", true) {
		t.Error("build must be ignored")
	}
}

func TestLoadMissingFileIsNotAnError(t *testing.T) {
	dir := t.TempDir()
	m := NewMatcher(dir)
	if err := m.Load(filepath.Join(dir, "nope", ".gitignore")); err != nil {
		t.Fatalf("Load on a missing file = %v, want nil", err)
	}
	if len(m.patterns) != 0 {
		t.Errorf("patterns = %v, want none", describe(m))
	}
}

func TestLoadDirectorySurfacesAnError(t *testing.T) {
	// A directory cannot be scanned line by line; that is a real error, unlike
	// a missing file.
	dir := t.TempDir()
	m := NewMatcher(dir)
	if err := m.Load(dir); err == nil {
		t.Fatal("Load on a directory should return an error")
	}
}

func TestLoadSkipsUnparseablePattern(t *testing.T) {
	// Every glob character is escaped by translateGlob, so regexp.Compile
	// cannot fail here: a pattern that would be invalid regex is still
	// compiled as literal text. The guard exists so a future change that lets
	// a pattern through raw cannot drop the rest of the file.
	dir := t.TempDir()
	gi := filepath.Join(dir, ".gitignore")
	mustWriteIgnore(t, gi, "[\n(unclosed\na.log\n")
	m := NewMatcher(dir)
	mustLoad(t, m, dir, ".gitignore")
	if got := len(m.patterns); got != 3 {
		t.Fatalf("len(patterns) = %d, want 3", got)
	}
	if !m.Match("a.log", false) {
		t.Error("a.log must still be parsed after the odd patterns")
	}
}

func TestLoadEscapedHashAndBang(t *testing.T) {
	dir := t.TempDir()
	gi := filepath.Join(dir, ".gitignore")
	mustWriteIgnore(t, gi, `\#literal
\!literal
# a real comment
*.log
!allowed.log
`)
	m := NewMatcher(dir)
	mustLoad(t, m, dir, ".gitignore")
	// \#literal and \!literal are patterns, not a comment and not a negation.
	if !m.Match("#literal", false) {
		t.Error("#literal must be ignored (backslash escaped the hash)")
	}
	if !m.Match("!literal", false) {
		t.Error("!literal must be ignored (backslash escaped the bang)")
	}
	if !m.Match("drop.log", false) {
		t.Error("drop.log must be ignored")
	}
	// The negation must come after the exclusion it overrides.
	if m.Match("allowed.log", false) {
		t.Error("allowed.log must be re-included by the trailing negation")
	}
	// A negation placed before the exclusion loses: the last match wins.
	mustWriteIgnore(t, gi, `!allowed.log
*.log
`)
	m2 := NewMatcher(dir)
	mustLoad(t, m2, dir, ".gitignore")
	if !m2.Match("allowed.log", false) {
		t.Error("a negation before the exclusion must lose to the later pattern")
	}
}

func TestLoadRelativePathBecomesRootScoped(t *testing.T) {
	// NewMatcher stores an absolute root, so a relative path to Load makes
	// filepath.Rel fail and the patterns fall back to root scope. Pinning that
	// means a caller that mixes absolute and relative paths gets a well-defined
	// answer rather than silently mis-scoped patterns.
	dir := t.TempDir()
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(old) })
	mustWriteIgnore(t, filepath.Join(dir, ".gitignore"), "x.log\n")

	m := NewMatcher(dir)
	mustLoad(t, m, ".", ".gitignore")
	if got := m.relSource(m.patterns[0].source); got != "" {
		t.Fatalf("relSource = %q, want empty for the root-scoped fallback", got)
	}
	if !m.Match("deep/x.log", false) {
		t.Error("a root-scoped fallback pattern must apply at an absolute depth")
	}
	if got := m.relSource(dir); got != "." {
		t.Errorf("relSource(root) = %q, want %q", got, ".")
	}
	// Relating a relative path against the absolute root fails, which is the
	// branch that makes a bad source fall back to root scope.
	if got := m.relSource("relative"); got != "" {
		t.Errorf("relSource for a relative path = %q, want empty", got)
	}
	if got := m.relSource(filepath.Join(dir, "..", "elsewhere")); got != "../elsewhere" {
		t.Errorf("relSource outside the root = %q, want %q", got, "../elsewhere")
	}
}

func TestLoadStripsLeadingWhitespace(t *testing.T) {
	// Git strips leading spaces and tabs unless they are escaped with a
	// backslash, so "  # comment" is a comment. Without the strip, an
	// indented comment silently becomes a pattern that ignores nothing useful.
	dir := t.TempDir()
	gi := filepath.Join(dir, ".gitignore")
	mustWriteIgnore(t, gi, "  # indented comment\n\t# tab comment\n  a.log\n")
	m := NewMatcher(dir)
	mustLoad(t, m, dir, ".gitignore")
	if got := len(m.patterns); got != 1 {
		t.Fatalf("len(patterns) = %d, want 1; got %v", got, describe(m))
	}
	if !m.Match("a.log", false) {
		t.Error("a leading-whitespace pattern must still apply")
	}
	if m.Match("# indented comment", false) {
		t.Error("an indented comment must not become a pattern")
	}
}

func TestLoadBackslashProtectsLeadingWhitespace(t *testing.T) {
	// A leading backslash escapes the whitespace strip: "\ #x" is a pattern
	// that matches the name "\ #x", not a comment.
	dir := t.TempDir()
	gi := filepath.Join(dir, ".gitignore")
	mustWriteIgnore(t, gi, `	# an escaped-hash line below
\#escaped
`)
	m := NewMatcher(dir)
	mustLoad(t, m, dir, ".gitignore")
	if got := len(m.patterns); got != 1 {
		t.Fatalf("len(patterns) = %d, want 1; got %v", got, describe(m))
	}
	if !m.Match("#escaped", false) {
		t.Error("the escaped hash must match a literal # name")
	}
}

// --- Scoping: a pattern must not escape its own directory ---

func TestScopedPatternDoesNotLeakOutsideItsDirectory(t *testing.T) {
	dir := t.TempDir()
	mustWriteIgnore(t, filepath.Join(dir, ".gitignore"), "node_modules/\n")
	mustWriteIgnore(t, filepath.Join(dir, "web", ".gitignore"), "*.js\n")

	m := NewMatcher(dir)
	mustLoad(t, m, dir, ".gitignore")
	mustLoad(t, m, dir, filepath.Join("web", ".gitignore"))

	cases := []struct {
		path  string
		isDir bool
		want  bool
	}{
		// The root pattern applies everywhere, including inside web/.
		{"node_modules", true, true},
		{"web/node_modules", true, true},
		{"lib/node_modules", true, true},

		// The web/ pattern only applies inside web/.
		{"web/app.js", false, true},
		{"web/child/app.js", false, true},
		{"app.js", false, false},     // same name, outside web/
		{"lib/app.js", false, false}, // same name, in a sibling dir
		{"web/app.ts", false, false}, // different extension
	}
	for _, c := range cases {
		if got := m.Match(c.path, c.isDir); got != c.want {
			t.Errorf("Match(%q, dir=%v) = %v, want %v", c.path, c.isDir, got, c.want)
		}
	}
}

func TestNestedScopeIsNarrowerThanItsOwnRoot(t *testing.T) {
	// A pattern that excludes a directory in web/ must not exclude the same
	// directory name at the root.
	dir := t.TempDir()
	mustWriteIgnore(t, filepath.Join(dir, "web", ".gitignore"), "dist/\n")
	m := NewMatcher(dir)
	mustLoad(t, m, dir, filepath.Join("web", ".gitignore"))

	if m.Match("dist", true) {
		t.Error("root dist/ must not be ignored by a web/ pattern")
	}
	if !m.Match("web/dist", true) {
		t.Error("web/dist must be ignored")
	}
	if !m.Match("web/dist/bundle.js", false) {
		t.Error("contents of web/dist must be excluded through the ancestor")
	}
}

func TestNestedNegationOverridesRoot(t *testing.T) {
	// Root excludes *.log everywhere; web/ keeps its report.log.
	dir := t.TempDir()
	mustWriteIgnore(t, filepath.Join(dir, ".gitignore"), "*.log\n")
	mustWriteIgnore(t, filepath.Join(dir, "web", ".gitignore"), "!report.log\n")

	m := NewMatcher(dir)
	mustLoad(t, m, dir, ".gitignore")
	mustLoad(t, m, dir, filepath.Join("web", ".gitignore"))

	cases := []struct {
		path string
		want bool
	}{
		{"app.log", true},
		{"lib/app.log", true},
		{"web/report.log", false}, // re-included by the nested negation
		{"web/app.log", true},     // the negation is named, not catch-all
	}
	for _, c := range cases {
		if got := m.Match(c.path, false); got != c.want {
			t.Errorf("Match(%q) = %v, want %v", c.path, got, c.want)
		}
	}
}

func TestLastPatternWins(t *testing.T) {
	dir := t.TempDir()
	gi := filepath.Join(dir, ".gitignore")
	mustWriteIgnore(t, gi, "*.log\n!important.log\nimportant.log\n")

	m := NewMatcher(dir)
	mustLoad(t, m, dir, ".gitignore")

	// The final pattern re-excludes important.log, overriding the negation.
	if !m.Match("important.log", false) {
		t.Error("the last matching pattern must win, so important.log is ignored")
	}
	if !m.Match("other.log", false) {
		t.Error("other.log must be ignored")
	}
}

// --- Wildcard and glob semantics ---

func TestQuestionMarkWildcard(t *testing.T) {
	cases := []struct {
		pattern string
		path    string
		want    bool
	}{
		{"a?c", "abc", true},
		{"a?c", "ac", false},   // ? needs exactly one character
		{"a?c", "abbc", false}, // ? needs exactly one character
		{"a?c", "axc", true},
		// ? must not cross a path separator.
		{"a?c", "a/c", false},
		{"a?c", "x/a?c", true}, // basename pattern matches at depth
		{"*.go", "a.go", true},
		{"*.go", "ab.go", true},
		// "?*" is one character plus zero or more, so a single character fits.
		{"?*", "a", true},
	}
	for _, c := range cases {
		re, err := CompileGlob(c.pattern)
		if err != nil {
			t.Fatalf("CompileGlob(%q): %v", c.pattern, err)
		}
		if got := re.MatchString(c.path); got != c.want {
			t.Errorf("CompileGlob(%q).Match(%q) = %v, want %v", c.pattern, c.path, got, c.want)
		}
	}
}

func TestDoubleStarForms(t *testing.T) {
	cases := []struct {
		pattern string
		path    string
		want    bool
	}{
		{"**", "a/b/c", true}, // ** crosses separators
		{"**", "a", true},
		{"**", "", true},              // .* legitimately matches nothing
		{"**/*.go", "a/b/c.go", true}, // a common form
		{"**/*.go", "c.go", true},     // zero directories
		{"**/*.go", "a/b/c.txt", false},
		{"src/**/*.go", "src/x.go", true},
		{"src/**/*.go", "src/x/y/z.go", true},
		{"src/**/*.go", "src-x/y.go", false}, // anchored by the internal "/"
		{"a/**", "a/b/c", true},
		{"a/**", "a", false}, // a/** requires something inside
		{"a/**/b", "a/x/y/b", true},
		{"a/**/b", "a/b", true},
		{"a/**/b", "a/x/b", true},
		{"a/**/b", "b", false},
		// The separator after "**/" is load-bearing: "a/**/b" must not collapse
		// into "a/.*b", which would match a/xby.
		{"a/**/b", "a/xby", false},
		{"a/**/b", "a/xb", false},
		// A single * never crosses a separator.
		{"a/*.go", "a/x.go", true},
		{"a/*.go", "a/x/y.go", false},
	}
	for _, c := range cases {
		re, err := CompileGlob(c.pattern)
		if err != nil {
			t.Fatalf("CompileGlob(%q): %v", c.pattern, err)
		}
		if got := re.MatchString(c.path); got != c.want {
			t.Errorf("CompileGlob(%q).Match(%q) = %v, want %v", c.pattern, c.path, got, c.want)
		}
	}
}

func TestLeadingSlashAnchorsToTheScopeRoot(t *testing.T) {
	dir := t.TempDir()
	mustWriteIgnore(t, filepath.Join(dir, ".gitignore"), "/secret.txt\n")
	m := NewMatcher(dir)
	mustLoad(t, m, dir, ".gitignore")

	if !m.Match("secret.txt", false) {
		t.Error("/secret.txt must match at the root")
	}
	if m.Match("deep/secret.txt", false) {
		t.Error("a leading-slash pattern must not match at depth")
	}
}

func TestEscapedSpecialCharactersAreLiteral(t *testing.T) {
	cases := []struct {
		pattern string
		path    string
		want    bool
	}{
		{"a.b", "a.b", true},
		{"a.b", "axb", false},
		{"a+b", "a+b", true},
		{"a+b", "aab", false},
		{"(x)", "(x)", true},
		{"[x]", "[x]", true},
		{"f$", "f$", true},
		{"a|b", "a|b", true},
		{"a|b", "a", false},
		{`a\b`, `a\b`, true},
	}
	for _, c := range cases {
		re, err := CompileGlob(c.pattern)
		if err != nil {
			t.Fatalf("CompileGlob(%q): %v", c.pattern, err)
		}
		if got := re.MatchString(c.path); got != c.want {
			t.Errorf("CompileGlob(%q).Match(%q) = %v, want %v", c.pattern, c.path, got, c.want)
		}
	}
}

// --- Directory-only and ancestor propagation ---

func TestDirOnlyDoesNotIgnoreAFileOfTheSameName(t *testing.T) {
	dir := t.TempDir()
	mustWriteIgnore(t, filepath.Join(dir, ".gitignore"), "build/\n")
	m := NewMatcher(dir)
	mustLoad(t, m, dir, ".gitignore")

	if !m.Match("build", true) {
		t.Error("the build directory must be ignored")
	}
	if m.Match("build", false) {
		t.Error("a *file* named build must not be ignored by a directory-only pattern")
	}
	if !m.Match("build/output.js", false) {
		t.Error("contents of the excluded directory must be excluded")
	}
}

func TestExclusionPropagatesThroughManyLevels(t *testing.T) {
	dir := t.TempDir()
	mustWriteIgnore(t, filepath.Join(dir, ".gitignore"), "vendor/\n*.go\n")
	m := NewMatcher(dir)
	mustLoad(t, m, dir, ".gitignore")

	for _, p := range []string{
		"a/b/c.go",
		"a/b/c/d/e/f.go",
		"vendor/x/y/z.go",
	} {
		if !m.Match(p, false) {
			t.Errorf("Match(%q) = false, want true", p)
		}
	}
	if m.Match("a/b/keep.txt", false) {
		t.Error("a/b/keep.txt must not be ignored")
	}
}

func TestMatchRejectsEmptyPath(t *testing.T) {
	dir := t.TempDir()
	mustWriteIgnore(t, filepath.Join(dir, ".gitignore"), "*\n")
	m := NewMatcher(dir)
	mustLoad(t, m, dir, ".gitignore")
	if m.Match("", false) {
		t.Error("an empty path must not be ignored")
	}
	if m.Match("", true) {
		t.Error("an empty directory path must not be ignored")
	}
}

// --- Debug surface ---

func TestPatternString(t *testing.T) {
	re, _ := CompileGlob("build/")
	cases := []struct {
		p    Pattern
		want string
	}{
		{Pattern{re: re, raw: "build/"}, `allow "build/" (dirOnly=false)`},
		{Pattern{re: re, negate: true, raw: "!keep.log"}, `unignore "!keep.log" (dirOnly=false)`},
		{Pattern{re: re, raw: "build/", dirOnly: true}, `allow "build/" (dirOnly=true)`},
		{Pattern{re: re, negate: true, raw: "!x", dirOnly: true}, `unignore "!x" (dirOnly=true)`},
	}
	for _, c := range cases {
		if got := c.p.String(); got != c.want {
			t.Errorf("Pattern{raw=%q,negate=%v,dirOnly=%v}.String() = %q, want %q",
				c.p.raw, c.p.negate, c.p.dirOnly, got, c.want)
		}
	}
}

func TestTranslateGlobOutputIsStable(t *testing.T) {
	cases := []struct {
		glob string
		want string
	}{
		{"*.go", `[^/]*\.go`},
		{"a?c", `a[^/]c`},
		{"**", `.*`},
		{"**/", `(?:.*/)?`}, // the slash becomes "any directories"
		{"a/**/b", `a/(?:.*/)?b`},
		{"a/**", `a/.*`},
		{`.`, `\.`},
		{"(", `\(`},
		{")", `\)`},
		{"[", `\[`},
		{"]", `\]`},
		{"|", `\|`},
		{"$", `\$`},
		{"\\", `\\`},
	}
	for _, c := range cases {
		if got := translateGlob(c.glob); got != c.want {
			t.Errorf("translateGlob(%q) = %q, want %q", c.glob, got, c.want)
		}
	}
}

func describe(m *Matcher) []string {
	out := make([]string, 0, len(m.patterns))
	for _, p := range m.patterns {
		out = append(out, p.String())
	}
	return out
}
