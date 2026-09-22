package walker

import (
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

func names(res *Result) []string {
	out := make([]string, 0, len(res.Files))
	for _, f := range res.Files {
		out = append(out, f.RelPath)
	}
	return out
}

// --- Walk error paths ---

func TestWalkRootNotADirectory(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "not-a-dir.txt")
	if err := os.WriteFile(file, []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Walk(file, Options{RespectGitignore: false, ReadContent: false})
	if err == nil {
		t.Fatal("expected an error when the root is a regular file")
	}
	if !strings.Contains(err.Error(), "is not a directory") {
		t.Fatalf("error = %v, want it to mention \"is not a directory\"", err)
	}
}

func TestWalkMissingRoot(t *testing.T) {
	if _, err := Walk(filepath.Join(t.TempDir(), "does-not-exist"), Options{}); err == nil {
		t.Fatal("expected an error when the root does not exist")
	}
}

func TestWalkMalformedRootGitignore(t *testing.T) {
	dir := t.TempDir()
	// One line longer than the 1 MiB scanner cap makes Matcher.Load fail.
	long := strings.Repeat("x", 1024*1024+1)
	if err := os.WriteFile(filepath.Join(dir, ".gitignore"), []byte(long), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Walk(dir, Options{RespectGitignore: true}); err == nil {
		t.Fatal("expected the malformed root .gitignore to surface as an error")
	}
}

// A symlink whose target does not exist makes d.Info() fail. The walker must
// count it as skipped rather than reporting a broken result or a phantom file.
// Skipped on Windows: os.Symlink needs SeCreateSymbolicLinkPrivilege, and a
// directory junction is a real directory to WalkDir, so the error cannot be
// produced there.
func TestWalkSkipsABrokenSymlink(t *testing.T) {
	dir := t.TempDir()
	link := filepath.Join(dir, "missing-link")
	if err := os.Symlink(filepath.Join(dir, "no-such-target"), link); err != nil {
		t.Skip("cannot create symlinks here (" + err.Error() + "), skipping")
	}
	mustWrite(t, filepath.Join(dir, "real.txt"), []byte("ok\n"))

	res, err := Walk(dir, Options{RespectGitignore: false, ReadContent: true})
	if err != nil {
		t.Fatal(err)
	}
	got := names(res)
	if len(got) != 1 || got[0] != "real.txt" {
		t.Fatalf("files = %v, want [real.txt]", got)
	}
	if res.Skipped != 1 {
		t.Errorf("Skipped = %d, want 1 for the broken symlink", res.Skipped)
	}
}

// A subdirectory whose ACL denies read makes WalkDir hand the walker a
// non-nil error for that entry. The walk must continue and drop the subtree,
// not fail the whole call. icacls is required, and the ACL is restored before
// the TempDir is deleted.
func TestWalkToleratesAnUnreadableSubtree(t *testing.T) {
	if _, err := exec.LookPath("icacls"); err != nil {
		t.Skip("icacls unavailable")
	}
	dir := t.TempDir()
	sub := filepath.Join(dir, "locked")
	mustWrite(t, filepath.Join(sub, "hidden.txt"), []byte("secret\n"))
	mustWrite(t, filepath.Join(dir, "open.txt"), []byte("ok\n"))

	if err := exec.Command("icacls", sub, "/deny", "Everyone:(OI)(CI)(R)").Run(); err != nil {
		t.Skip("could not deny read on a subdirectory")
	}
	t.Cleanup(func() {
		_ = exec.Command("icacls", sub, "/remove", "Everyone").Run()
		_ = exec.Command("icacls", sub, "/grant", "Everyone:(OI)(CI)(RX)").Run()
	})

	res, err := Walk(dir, Options{RespectGitignore: false, ReadContent: true})
	if err != nil {
		t.Fatalf("Walk errored on an unreadable subtree: %v", err)
	}
	got := names(res)
	if !has(got, "open.txt") {
		t.Errorf("the readable file was lost: %v", got)
	}
	if has(got, "locked/hidden.txt") {
		t.Errorf("the unreadable subtree leaked into the result: %v", got)
	}
}

// A file whose ACL denies read can still be stat'ed, so d.Info() succeeds and
// os.ReadFile is the call that fails. The walker must skip the entry rather
// than fail the walk, and must not lose the files around it. Unlike the
// unreadable-subtree case above, this is a per-file deny with no (OI)(CI), so
// enumeration of the parent directory is unaffected.
func TestWalkSkipsAFileItCannotRead(t *testing.T) {
	if _, err := exec.LookPath("icacls"); err != nil {
		t.Skip("icacls unavailable")
	}
	dir := t.TempDir()
	blocked := filepath.Join(dir, "blocked.txt")
	mustWrite(t, blocked, []byte("secret\n"))
	mustWrite(t, filepath.Join(dir, "open.txt"), []byte("ok\n"))

	if err := exec.Command("icacls", blocked, "/deny", "Everyone:(R)").Run(); err != nil {
		t.Skip("could not deny read on a file")
	}
	t.Cleanup(func() {
		_ = exec.Command("icacls", blocked, "/remove", "Everyone").Run()
	})

	// Proving the premise: stat succeeds and read fails. Without that the
	// test would skip silently while covering nothing.
	if _, err := os.Stat(blocked); err != nil {
		t.Skipf("cannot stat %s, so the branch is unreachable here: %v", blocked, err)
	}
	if _, err := os.ReadFile(blocked); err == nil {
		t.Skip("the deny was not applied: read succeeded, so the branch is unreachable")
	}

	res, err := Walk(dir, Options{RespectGitignore: false, ReadContent: true})
	if err != nil {
		t.Fatalf("Walk errored on an unreadable file: %v", err)
	}
	got := names(res)
	if !has(got, "open.txt") {
		t.Errorf("the readable file was lost: %v", got)
	}
	if has(got, "blocked.txt") {
		t.Errorf("an unreadable file leaked into the result: %v", got)
	}
	if res.Skipped == 0 {
		t.Error("the unreadable file was not counted as skipped")
	}
}

func has(list []string, want string) bool {
	for _, v := range list {
		if v == want {
			return true
		}
	}
	return false
}

// --- Size and content gating ---

func TestWalkMaxFileSizeListsWithoutContent(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "small.txt"), []byte("small\n"))
	mustWrite(t, filepath.Join(dir, "big.txt"), make([]byte, 5000))

	res, err := Walk(dir, Options{MaxFileSize: 1000, ReadContent: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Files) != 2 {
		t.Fatalf("files = %v, want both small.txt and big.txt listed", names(res))
	}
	if res.Skipped != 1 {
		t.Fatalf("Skipped = %d, want 1 (big.txt)", res.Skipped)
	}
	var big *FileEntry
	for i := range res.Files {
		if res.Files[i].RelPath == "big.txt" {
			big = &res.Files[i]
		}
	}
	if big == nil {
		t.Fatal("big.txt missing from result")
	}
	if big.Size != 5000 {
		t.Errorf("big.txt Size = %d, want 5000", big.Size)
	}
	if big.Content != nil {
		t.Errorf("big.txt Content = %q, want nil", big.Content)
	}
	if big.IsBinary {
		t.Errorf("big.txt IsBinary = true, want false")
	}
}

func TestWalkReadContentFalseOnlyMetadata(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "a.txt"), []byte("hello\n"))
	mustWrite(t, filepath.Join(dir, "nul.txt"), []byte{0x78, 0x00, 0x79})

	res, err := Walk(dir, Options{ReadContent: false})
	if err != nil {
		t.Fatal(err)
	}
	if res.Skipped != 0 {
		t.Fatalf("Skipped = %d, want 0: ReadContent=false must not sniff binaries", res.Skipped)
	}
	if len(res.Files) != 2 {
		t.Fatalf("files = %v, want 2 entries", names(res))
	}
	for _, f := range res.Files {
		if f.Content != nil {
			t.Errorf("%s: Content = %q, want nil", f.RelPath, f.Content)
		}
		if f.IsBinary {
			t.Errorf("%s: IsBinary = true, want false", f.RelPath)
		}
		if f.Size <= 0 {
			t.Errorf("%s: Size = %d, want > 0", f.RelPath, f.Size)
		}
	}
}

func TestWalkBinaryByContent(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "payload.txt"), []byte{0x78, 0x00, 0x79})

	res, err := Walk(dir, Options{ReadContent: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Files) != 1 {
		t.Fatalf("files = %v, want [payload.txt]", names(res))
	}
	f := res.Files[0]
	if !f.IsBinary {
		t.Errorf("payload.txt IsBinary = false, want true")
	}
	if f.Content != nil {
		t.Errorf("payload.txt Content = %q, want nil", f.Content)
	}
	if res.Skipped != 1 {
		t.Fatalf("Skipped = %d, want 1", res.Skipped)
	}
}

// --- Hidden entries ---

func TestWalkIncludeHidden(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "plain.go"), []byte("package main\n"))
	mustWrite(t, filepath.Join(dir, ".env"), []byte("SECRET=1\n"))
	mustWrite(t, filepath.Join(dir, ".github"), nil)
	mustWrite(t, filepath.Join(dir, ".github", "workflow.yml"), []byte("on: push\n"))
	mustWrite(t, filepath.Join(dir, ".vscode"), nil)
	mustWrite(t, filepath.Join(dir, ".vscode", "settings.json"), []byte("{}\n"))
	mustWrite(t, filepath.Join(dir, ".git"), nil)
	mustWrite(t, filepath.Join(dir, ".git", "config"), []byte("x\n"))

	res, err := Walk(dir, Options{IncludeHidden: true, ReadContent: true})
	if err != nil {
		t.Fatal(err)
	}
	got := names(res)
	sort.Strings(got)
	// .vscode stays in the default denylist and .git is always a VCS dir.
	want := []string{".env", ".github/workflow.yml", "plain.go"}
	if !equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestWalkHiddenExcludedByDefault(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "plain.go"), []byte("package main\n"))
	mustWrite(t, filepath.Join(dir, ".env"), []byte("SECRET=1\n"))
	mustWrite(t, filepath.Join(dir, ".github"), nil)
	mustWrite(t, filepath.Join(dir, ".github", "workflow.yml"), []byte("x\n"))

	res, err := Walk(dir, Options{IncludeHidden: false, ReadContent: true})
	if err != nil {
		t.Fatal(err)
	}
	got := names(res)
	if len(got) != 1 || got[0] != "plain.go" {
		t.Fatalf("got %v, want [plain.go]", got)
	}
}

func TestWalkHiddenStillExcludedWhenIncluded(t *testing.T) {
	// A user glob may target a dotfile, but hidden files stay out until
	// IncludeHidden is set — .env is exactly what a leak would look like.
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, ".env"), []byte("SECRET=1\n"))
	mustWrite(t, filepath.Join(dir, "src"), nil)
	mustWrite(t, filepath.Join(dir, "src", ".env"), []byte("SECRET=2\n"))
	mustWrite(t, filepath.Join(dir, "src", "deep"), nil)
	mustWrite(t, filepath.Join(dir, "src", "deep", ".config"), []byte("x\n"))

	for _, glob := range []string{".env", "*.env", "**/.env", "**/*.env", "**/.config"} {
		res, err := Walk(dir, Options{Include: []string{glob}, ReadContent: true})
		if err != nil {
			t.Fatal(err)
		}
		if len(res.Files) != 0 {
			t.Fatalf("Include %q leaked hidden files: %v", glob, names(res))
		}
	}
}

func TestWalkHiddenRootIsStillWalked(t *testing.T) {
	// The hidden check skips its own root, so a dot-directory passed as the
	// walk root is not treated as a hidden entry.
	base := t.TempDir()
	dir := filepath.Join(base, ".repo")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(dir, "a.txt"), []byte("hello\n"))

	res, err := Walk(dir, Options{IncludeHidden: false, ReadContent: true})
	if err != nil {
		t.Fatal(err)
	}
	got := names(res)
	if len(got) != 1 || got[0] != "a.txt" {
		t.Fatalf("got %v, want [a.txt]", got)
	}
}

// --- .gitignore layering ---

func TestWalkNestedGitignore(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "pkg"), nil)
	mustWrite(t, filepath.Join(dir, "pkg", ".gitignore"), []byte("*_gen.go\n"))
	mustWrite(t, filepath.Join(dir, "pkg", "real.go"), []byte("package pkg\n"))
	mustWrite(t, filepath.Join(dir, "pkg", "gen_gen.go"), []byte("package pkg\n"))

	res, err := Walk(dir, Options{RespectGitignore: true, ReadContent: true})
	if err != nil {
		t.Fatal(err)
	}
	got := names(res)
	if len(got) != 1 || got[0] != "pkg/real.go" {
		t.Fatalf("got %v, want [pkg/real.go]", got)
	}
	if res.Skipped != 1 {
		t.Fatalf("Skipped = %d, want 1 (pkg/gen_gen.go)", res.Skipped)
	}
}

func TestWalkNestedGitignoreDisabled(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "pkg"), nil)
	mustWrite(t, filepath.Join(dir, "pkg", ".gitignore"), []byte("*_gen.go\n"))
	mustWrite(t, filepath.Join(dir, "pkg", "real.go"), []byte("package pkg\n"))
	mustWrite(t, filepath.Join(dir, "pkg", "gen_gen.go"), []byte("package pkg\n"))

	res, err := Walk(dir, Options{RespectGitignore: false, ReadContent: true})
	if err != nil {
		t.Fatal(err)
	}
	got := names(res)
	sort.Strings(got)
	want := []string{"pkg/gen_gen.go", "pkg/real.go"}
	if !equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	if res.Skipped != 0 {
		t.Fatalf("Skipped = %d, want 0", res.Skipped)
	}
}

func TestWalkGitignoreNegation(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, ".gitignore"), []byte("*.log\n!important.log\n"))
	mustWrite(t, filepath.Join(dir, "drop.log"), []byte("x\n"))
	mustWrite(t, filepath.Join(dir, "important.log"), []byte("x\n"))
	mustWrite(t, filepath.Join(dir, "keep.txt"), []byte("x\n"))

	res, err := Walk(dir, Options{RespectGitignore: true, ReadContent: true})
	if err != nil {
		t.Fatal(err)
	}
	got := names(res)
	sort.Strings(got)
	want := []string{"important.log", "keep.txt"}
	if !equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	if res.Skipped != 1 {
		t.Fatalf("Skipped = %d, want 1 (drop.log)", res.Skipped)
	}
}

func TestWalkGitignoreExcludesDirectory(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, ".gitignore"), []byte("scratch/\n"))
	mustWrite(t, filepath.Join(dir, "scratch"), nil)
	mustWrite(t, filepath.Join(dir, "scratch", "junk.go"), []byte("package junk\n"))
	mustWrite(t, filepath.Join(dir, "keep.go"), []byte("package main\n"))

	res, err := Walk(dir, Options{RespectGitignore: true, ReadContent: true})
	if err != nil {
		t.Fatal(err)
	}
	got := names(res)
	if len(got) != 1 || got[0] != "keep.go" {
		t.Fatalf("got %v, want [keep.go]", got)
	}
}

func TestWalkExcludeDirectory(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "keep.go"), []byte("package main\n"))
	mustWrite(t, filepath.Join(dir, "vendor"), nil)
	mustWrite(t, filepath.Join(dir, "vendor", "mod.go"), []byte("package mod\n"))

	res, err := Walk(dir, Options{Exclude: []string{"vendor"}, ReadContent: true})
	if err != nil {
		t.Fatal(err)
	}
	got := names(res)
	if len(got) != 1 || got[0] != "keep.go" {
		t.Fatalf("got %v, want [keep.go]", got)
	}
}

// --- Built-in default denylist ---

func TestWalkDefaultIgnoreDirs(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "keep.go"), []byte("package main\n"))
	for _, d := range []string{"__pycache__", "dist", "coverage", "Pods", "build"} {
		mustWrite(t, filepath.Join(dir, d), nil)
		mustWrite(t, filepath.Join(dir, d, "junk.go"), []byte("package junk\n"))
	}

	res, err := Walk(dir, Options{RespectGitignore: false, ReadContent: true})
	if err != nil {
		t.Fatal(err)
	}
	got := names(res)
	if len(got) != 1 || got[0] != "keep.go" {
		t.Fatalf("got %v, want [keep.go]", got)
	}
}

func TestWalkDefaultIgnoreFiles(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "keep.go"), []byte("package main\n"))
	mustWrite(t, filepath.Join(dir, "go.sum"), []byte("module\n"))
	mustWrite(t, filepath.Join(dir, "Thumbs.db"), []byte{0x00, 0x00})
	mustWrite(t, filepath.Join(dir, "app.exe"), []byte("MZ"))
	// .sqlite is binary by extension but not in the default file denylist, so
	// it is still listed with IsBinary set.
	mustWrite(t, filepath.Join(dir, "data.sqlite"), []byte("\x00SQLite format 3"))

	res, err := Walk(dir, Options{ReadContent: true})
	if err != nil {
		t.Fatal(err)
	}
	got := names(res)
	sort.Strings(got)
	want := []string{"data.sqlite", "keep.go"}
	if !equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	if res.Skipped != 4 {
		t.Fatalf("Skipped = %d, want 4 (go.sum, Thumbs.db, app.exe, data.sqlite)", res.Skipped)
	}
	var sqlite *FileEntry
	for i := range res.Files {
		if res.Files[i].RelPath == "data.sqlite" {
			sqlite = &res.Files[i]
		}
	}
	if sqlite == nil || !sqlite.IsBinary {
		t.Fatalf("data.sqlite IsBinary = %v, want true", sqlite)
	}
}

// --- Result shape ---

func TestWalkResultRoot(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "a.txt"), []byte("x\n"))
	res, err := Walk(dir, Options{ReadContent: false})
	if err != nil {
		t.Fatal(err)
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		t.Fatal(err)
	}
	if res.Root != abs {
		t.Fatalf("Root = %q, want %q", res.Root, abs)
	}
	if res.Files[0].AbsPath != filepath.Join(abs, "a.txt") {
		t.Fatalf("AbsPath = %q, want %q", res.Files[0].AbsPath, filepath.Join(abs, "a.txt"))
	}
}

// --- Pure helpers ---

func TestIsVCSDir(t *testing.T) {
	for _, name := range []string{".git", ".hg", ".svn"} {
		if !isVCSDir(name) {
			t.Errorf("isVCSDir(%q) = false, want true", name)
		}
	}
	for _, name := range []string{".github", ".vscode", "git", ".git2", ""} {
		if isVCSDir(name) {
			t.Errorf("isVCSDir(%q) = true, want false", name)
		}
	}
}

func TestDefaultIgnoreDir(t *testing.T) {
	for _, name := range []string{"node_modules", "__pycache__", ".venv", "venv",
		"env", ".next", ".nuxt", ".turbo", ".svelte-kit", "dist", "build",
		"out", "target", "bin", "obj", ".gradle", ".terraform", "coverage",
		".cache", ".parcel-cache", ".pytest_cache", ".mypy_cache",
		".ruff_cache", ".idea", ".vscode", "Pods", "DerivedData", ".DS_Store"} {
		if !defaultIgnoreDir(name) {
			t.Errorf("defaultIgnoreDir(%q) = false, want true", name)
		}
	}
	for _, name := range []string{"src", "pkg", "vendor", "docs", "internal", "test"} {
		if defaultIgnoreDir(name) {
			t.Errorf("defaultIgnoreDir(%q) = true, want false", name)
		}
	}
}

func TestDefaultIgnoreFile(t *testing.T) {
	for _, name := range []string{"a.pyc", "a.pyo", "a.class", "a.o", "a.obj",
		"a.so", "a.dll", "a.dylib", "a.exe", "a.bin", "a.png", "a.jpg",
		"a.jpeg", "a.gif", "a.bmp", "a.ico", "a.webp", "a.mp3", "a.mp4",
		"a.mov", "a.zip", "a.gz", "a.tar", "a.rar", "a.7z", "a.pdf",
		"a.lock", "a.min.js", "a.min.css", ".DS_Store", "Thumbs.db",
		"package-lock.json", "yarn.lock", "pnpm-lock.yaml", "Cargo.lock",
		"go.sum", "poetry.lock", "Pipfile.lock", "composer.lock"} {
		if !defaultIgnoreFile(name) {
			t.Errorf("defaultIgnoreFile(%q) = false, want true", name)
		}
	}
	for _, name := range []string{"a.go", "a.txt", "a.md", "a.sqlite", "a.db", "a.js", "a.css"} {
		if defaultIgnoreFile(name) {
			t.Errorf("defaultIgnoreFile(%q) = true, want false", name)
		}
	}
}

func TestIsBinaryByExt(t *testing.T) {
	for _, name := range []string{"img.PNG", "song.MP3", "vid.MOV", "zip.TGZ",
		"doc.DOCX", "app.WASM", "core.JAR", "state.DB", "core.WAR", "core.PAK",
		"state.SQLITE", "scan.TIFF", "clip.AVI", "clip.MKV", "tone.WAV",
		"tone.FLAC", "pkg.BZ2", "pkg.XZ", "doc.DOC", "sheet.XLSX", "slide.PPTX",
		"obj.O", "obj.OBJ", "class.CLASS"} {
		if !isBinaryByExt(name) {
			t.Errorf("isBinaryByExt(%q) = false, want true", name)
		}
	}
	for _, name := range []string{"a.go", "a.txt", "a.json", "a.yaml", "a.js", "a.css"} {
		if isBinaryByExt(name) {
			t.Errorf("isBinaryByExt(%q) = true, want false", name)
		}
	}
}

func TestIsBinaryByContent(t *testing.T) {
	if !isBinaryByContent([]byte{0x78, 0x00, 0x79}) {
		t.Error("NUL byte in the first bytes must be reported as binary")
	}
	if isBinaryByContent([]byte("all plain text")) {
		t.Error("plain text must not be reported as binary")
	}
	if isBinaryByContent(nil) {
		t.Error("empty content must not be reported as binary")
	}
	// A NUL past the 8 KiB sniff window is intentionally not detected.
	long := []byte(strings.Repeat("x", 8192+4))
	long[8193] = 0
	if isBinaryByContent(long) {
		t.Error("NUL beyond the 8 KiB window must not be reported as binary")
	}
}

func TestMatchesAny(t *testing.T) {
	cases := []struct {
		path  string
		globs []string
		want  bool
	}{
		{"a.go", nil, false},
		{"a.go", []string{}, false},
		{"a.go", []string{"*.go"}, true},
		{"sub/a.go", []string{"*.go"}, true},
		{"sub/a.go", []string{"/sub/*.go"}, true},
		{"sub/a.go", []string{"/a.go"}, false},
		{"sub/a.go", []string{"a.go"}, true},
		{"a.go", []string{"a?"}, false},
		{"ab", []string{"a?"}, true},
		{"deep/nest/a.go", []string{"**/*.go"}, true},
		{"a.go", []string{"**/*.txt"}, false},
		{"b.go", []string{"[unclosed"}, false},
		{"sub/a.go", []string{"*.txt", "**/*.go"}, true},
	}
	for _, tc := range cases {
		if got := matchesAny(tc.path, tc.globs); got != tc.want {
			t.Errorf("matchesAny(%q, %v) = %v, want %v", tc.path, tc.globs, got, tc.want)
		}
	}
}

func TestGlobRegexCaches(t *testing.T) {
	a, err := globRegex("*.go")
	if err != nil {
		t.Fatal(err)
	}
	b, err := globRegex("*.go")
	if err != nil {
		t.Fatal(err)
	}
	if a != b {
		t.Fatal("globRegex must return the cached regex for a repeated pattern")
	}
	c, err := globRegex("*.md")
	if err != nil {
		t.Fatal(err)
	}
	if c == a {
		t.Fatal("distinct patterns must not share a compiled regex")
	}
	if !a.MatchString("x.go") || a.MatchString("x.txt") {
		t.Fatalf("compiled glob wrong: %s", a)
	}
}
