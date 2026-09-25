// Package gitutil shells out to the system `git` binary to list changed files.
// It degrades gracefully: a non-repo directory returns an empty list + a
// sentinel error so callers can fall back to full-tree packing.
package gitutil

import (
	"bytes"
	"errors"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// ErrNotARepo is returned when the path is not inside a git repository.
var ErrNotARepo = errors.New("not a git repository")

// ChangedFiles returns the set of files changed relative to a base ref.
//
//   - ref == "" or "HEAD" or "WORKTREE": returns the working-tree changes
//     (staged + unstaged + untracked, excluding deleted).
//   - ref contains ".." (a range, e.g. "main..origin/main"): returns files
//     differing across the range (git diff <range>), excluding deletions.
//     This is a pure historical comparison; it never adds the working tree's
//     untracked files, because those exist in neither side of the range.
//   - otherwise: returns files differing between <ref> and the working tree
//     (git diff --name-status <ref>), excluding deletions. Untracked files are
//     appended so that a dirty tree is not silently ignored.
//
// Paths are returned slash-separated and relative to root.
func ChangedFiles(root, ref string) ([]string, error) {
	if _, err := git(root, "rev-parse", "--git-dir"); err != nil {
		return nil, ErrNotARepo
	}

	if ref == "" || ref == "HEAD" || ref == "WORKTREE" {
		out, err := git(root, "status", "--porcelain", "--untracked-files=all")
		if err != nil {
			return nil, err
		}
		return parsePorcelain(out), nil
	}

	out, err := git(root, "diff", "--name-status", ref)
	if err != nil {
		return nil, err
	}
	files := parseNameStatus(out)

	// A..B compares two revisions, not a revision against the working tree.
	// Appending untracked files here would report paths present in neither
	// side of the range.
	if !strings.Contains(ref, "..") {
		if extra, err := git(root, "ls-files", "--others", "--exclude-standard"); err == nil {
			for _, line := range strings.Split(extra, "\n") {
				line = strings.TrimSpace(line)
				if line != "" {
					files = append(files, filepath.ToSlash(line))
				}
			}
		}
	}
	return dedup(files), nil
}

func parsePorcelain(out string) []string {
	var files []string
	for _, line := range strings.Split(out, "\n") {
		if len(line) < 3 {
			continue
		}
		status := strings.TrimSpace(line[:2])
		path := unquote(strings.TrimSpace(line[3:]))
		if i := strings.Index(path, " -> "); i >= 0 {
			path = path[i+4:]
		}
		if strings.Contains(status, "D") {
			continue
		}
		if path != "" {
			files = append(files, filepath.ToSlash(path))
		}
	}
	return files
}

func parseNameStatus(out string) []string {
	var files []string
	for _, line := range strings.Split(out, "\n") {
		parts := strings.SplitN(line, "\t", 3)
		if len(parts) < 2 {
			continue
		}
		if strings.HasPrefix(parts[0], "D") {
			continue
		}
		path := unquote(strings.TrimSpace(parts[1]))
		// A rename or copy carries the new name in the third field; the old
		// name is not the file that needs packing.
		if len(parts) == 3 && len(parts[0]) > 0 &&
			(parts[0][0] == 'R' || parts[0][0] == 'C') {
			path = unquote(strings.TrimSpace(parts[2]))
		}
		if path != "" {
			files = append(files, filepath.ToSlash(path))
		}
	}
	return files
}

// unquote strips git's C-style quoting. With core.quotePath (the default) a
// path containing non-ASCII bytes arrives as "caf\303\251.go"; the quotes and
// octal escapes are not part of the path.
func unquote(p string) string {
	if len(p) < 2 || p[0] != '"' {
		return p
	}
	if u, err := strconv.Unquote(p); err == nil {
		return u
	}
	return p
}

func git(root string, args ...string) (string, error) {
	full := append([]string{"-C", root}, args...)
	cmd := exec.Command("git", full...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err != nil {
		return "", err
	}
	return stdout.String(), nil
}

func dedup(s []string) []string {
	seen := make(map[string]struct{}, len(s))
	out := make([]string, 0, len(s))
	for _, v := range s {
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out
}
