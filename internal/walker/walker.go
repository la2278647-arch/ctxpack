// Package walker walks a directory tree in a gitignore-aware manner and
// returns the set of files worth feeding to an LLM.
//
// It is self-contained (stdlib only) and composes the ignore.Matcher with a
// built-in default denylist (VCS dirs, dependency dirs, build output, common
// binary extensions) so that a fresh checkout with no .gitignore still produces
// a sensible bundle. User-provided include/exclude globs layer on top.
package walker

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"github.com/la2278647-arch/ctxpack/internal/ignore"
)

// Options controls Walk behavior.
type Options struct {
	// Include globs (relative to root). Empty means "match everything not
	// excluded". A path is included only if it matches at least one include
	// glob (when set) and no exclude glob. Include does not reach hidden
	// entries: use IncludeHidden for those.
	Include []string
	// Exclude globs, applied after include.
	Exclude []string
	// MaxFileSize is the largest file (bytes) whose content is read. Larger
	// files are listed but their content is skipped. <=0 means unlimited.
	MaxFileSize int64
	// RespectGitignore disables .gitignore handling when false. The built-in
	// default denylist still applies.
	RespectGitignore bool
	// IncludeHidden includes dotfiles/dotdirs (.env, .github, ...). Hidden
	// entries are otherwise skipped regardless of Include, and hidden VCS dirs
	// (.git, .hg, .svn) are always skipped.
	IncludeHidden bool
	// ReadContent controls whether FileEntry.Content is populated. When false,
	// only metadata (path, size, binary) is collected — used by the repo map.
	ReadContent bool
}

// FileEntry describes one collected file.
type FileEntry struct {
	RelPath  string // slash-separated, relative to root
	AbsPath  string
	Size     int64
	IsBinary bool
	Content  []byte // populated only when Options.ReadContent is true
}

// Result holds the walk outcome.
type Result struct {
	Files   []FileEntry
	Skipped int // files skipped due to size/binary/exclude
	Root    string
}

// Walk walks root and returns the collected files.
//
// Every error other than a bad root is tolerated: the walk keeps going and the
// offending entry is either skipped or dropped from the result. The
// unreadable-subtree case is covered by TestWalkToleratesAnUnreadableSubtree.
// The rest need a file to vanish between two calls (d.Info, os.ReadFile) or
// WalkDir itself to be called with a callback that returns an error, which
// this callback never does.
func Walk(root string, opts Options) (*Result, error) {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(absRoot)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("walk root %q is not a directory", root)
	}

	matcher := ignore.NewMatcher(absRoot)
	if opts.RespectGitignore {
		if err := matcher.Load(filepath.Join(absRoot, ".gitignore")); err != nil {
			return nil, err
		}
	}

	res := &Result{Root: absRoot}
	err = filepath.WalkDir(absRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			// Keep walking past unreadable entries.
			return nil
		}
		rel, rerr := filepath.Rel(absRoot, path)
		if rerr != nil {
			rel = path
		}
		rel = filepath.ToSlash(rel)
		name := d.Name()

		// Always skip VCS metadata directories.
		if d.IsDir() && isVCSDir(name) {
			return filepath.SkipDir
		}

		// Hidden entries.
		if !opts.IncludeHidden && strings.HasPrefix(name, ".") && rel != "." {
			if d.IsDir() {
				return filepath.SkipDir
			}
			// A hidden file is skipped even when an include glob matches it.
			// --include is a way to narrow a scan, not an escape hatch from
			// the hidden-file default: if it did leak .env, the user would
			// have a secret in a prompt without ever having opted in. Hidden
			// files are brought in only by --hidden.
			return nil
		}

		if d.IsDir() {
			// Load nested .gitignore.
			if opts.RespectGitignore {
				gi := filepath.Join(path, ".gitignore")
				if _, e := os.Stat(gi); e == nil {
					_ = matcher.Load(gi)
				}
			}
			// Built-in default denylist for directories.
			if defaultIgnoreDir(name) {
				return filepath.SkipDir
			}
			// User exclude globs.
			if matchesAny(rel, opts.Exclude) {
				return filepath.SkipDir
			}
			// gitignore match on the directory itself.
			if matcher.Match(rel, true) {
				return filepath.SkipDir
			}
			return nil
		}

		// --- file ---
		// Built-in default denylist for files.
		if defaultIgnoreFile(name) {
			res.Skipped++
			return nil
		}
		// gitignore match.
		if opts.RespectGitignore && matcher.Match(rel, false) {
			res.Skipped++
			return nil
		}
		// User include/exclude.
		if len(opts.Include) > 0 && !matchesAny(rel, opts.Include) {
			res.Skipped++
			return nil
		}
		if matchesAny(rel, opts.Exclude) {
			res.Skipped++
			return nil
		}

		fi, err := d.Info()
		if err != nil {
			res.Skipped++
			return nil
		}
		entry := FileEntry{
			RelPath: rel,
			AbsPath: path,
			Size:    fi.Size(),
		}

		// Binary / size gating.
		binary := isBinaryByExt(name)
		tooLarge := opts.MaxFileSize > 0 && fi.Size() > opts.MaxFileSize
		if binary {
			entry.IsBinary = true
			res.Skipped++
			res.Files = append(res.Files, entry)
			return nil
		}
		if tooLarge {
			res.Skipped++
			res.Files = append(res.Files, entry)
			return nil
		}

		if opts.ReadContent {
			content, err := os.ReadFile(path)
			if err != nil {
				res.Skipped++
				return nil
			}
			if isBinaryByContent(content) {
				entry.IsBinary = true
				entry.Content = nil
				res.Skipped++
				res.Files = append(res.Files, entry)
				return nil
			}
			entry.Content = content
		}

		res.Files = append(res.Files, entry)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return res, nil
}

func isVCSDir(name string) bool {
	switch name {
	case ".git", ".hg", ".svn":
		return true
	}
	return false
}

func defaultIgnoreDir(name string) bool {
	switch name {
	case "node_modules", "__pycache__", ".venv", "venv", "env",
		".next", ".nuxt", ".turbo", ".svelte-kit",
		"dist", "build", "out", "target", "bin", "obj",
		".gradle", ".terraform", "coverage", ".cache", ".parcel-cache",
		".pytest_cache", ".mypy_cache", ".ruff_cache",
		".idea", ".vscode",
		"Pods", "DerivedData",
		".DS_Store":
		return true
	}
	return false
}

func defaultIgnoreFile(name string) bool {
	if strings.HasSuffix(name, ".pyc") || strings.HasSuffix(name, ".pyo") ||
		strings.HasSuffix(name, ".class") || strings.HasSuffix(name, ".o") ||
		strings.HasSuffix(name, ".obj") || strings.HasSuffix(name, ".so") ||
		strings.HasSuffix(name, ".dll") || strings.HasSuffix(name, ".dylib") ||
		strings.HasSuffix(name, ".exe") || strings.HasSuffix(name, ".bin") ||
		strings.HasSuffix(name, ".png") || strings.HasSuffix(name, ".jpg") ||
		strings.HasSuffix(name, ".jpeg") || strings.HasSuffix(name, ".gif") ||
		strings.HasSuffix(name, ".bmp") || strings.HasSuffix(name, ".ico") ||
		strings.HasSuffix(name, ".webp") || strings.HasSuffix(name, ".mp3") ||
		strings.HasSuffix(name, ".mp4") || strings.HasSuffix(name, ".mov") ||
		strings.HasSuffix(name, ".zip") || strings.HasSuffix(name, ".gz") ||
		strings.HasSuffix(name, ".tar") || strings.HasSuffix(name, ".rar") ||
		strings.HasSuffix(name, ".7z") || strings.HasSuffix(name, ".pdf") ||
		strings.HasSuffix(name, ".lock") || strings.HasSuffix(name, ".min.js") ||
		strings.HasSuffix(name, ".min.css") {
		return true
	}
	switch name {
	case ".DS_Store", "Thumbs.db", "package-lock.json", "yarn.lock",
		"pnpm-lock.yaml", "Cargo.lock", "go.sum", "poetry.lock",
		"Pipfile.lock", "composer.lock":
		return true
	}
	return false
}

// isBinaryByExt is a cheap extension-based heuristic.
func isBinaryByExt(name string) bool {
	exts := []string{".png", ".jpg", ".jpeg", ".gif", ".bmp", ".ico", ".webp",
		".tiff", ".mp3", ".mp4", ".mov", ".avi", ".mkv", ".wav", ".flac",
		".zip", ".gz", ".tar", ".tgz", ".rar", ".7z", ".bz2", ".xz",
		".pdf", ".doc", ".docx", ".xls", ".xlsx", ".ppt", ".pptx",
		".exe", ".dll", ".so", ".dylib", ".bin", ".o", ".obj", ".class",
		".wasm", ".jar", ".war", ".pak", ".sqlite", ".db"}
	lower := strings.ToLower(name)
	for _, e := range exts {
		if strings.HasSuffix(lower, e) {
			return true
		}
	}
	return false
}

// isBinaryByContent sniffs for a NUL byte in the first 8KB — the same heuristic
// git uses.
func isBinaryByContent(content []byte) bool {
	n := len(content)
	if n > 8192 {
		n = 8192
	}
	for i := 0; i < n; i++ {
		if content[i] == 0 {
			return true
		}
	}
	return false
}

// matchesAny reports whether path matches any of the globs. Empty globs =>
// no match. Globs use gitignore-style semantics (basename when no "/").
func matchesAny(path string, globs []string) bool {
	if len(globs) == 0 {
		return false
	}
	for _, g := range globs {
		re, err := globRegex(g)
		if err != nil {
			continue
		}
		if re.MatchString(path) {
			return true
		}
	}
	return false
}

var globCache sync.Map

// globRegex compiles a glob to an anchored regex, caching the result.
func globRegex(pattern string) (*regexp.Regexp, error) {
	if v, ok := globCache.Load(pattern); ok {
		return v.(*regexp.Regexp), nil
	}
	re, err := ignore.CompileGlob(pattern)
	if err != nil {
		// Unreachable: ignore.translateGlob escapes every regex-special
		// character, so its output always compiles. Kept for parity with the
		// exported error return.
		return nil, err
	}
	globCache.Store(pattern, re)
	return re, nil
}
