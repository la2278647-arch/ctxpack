// Package gitutil shells out to the system `git` binary to list changed files.
// It degrades gracefully: a non-repo directory returns an empty list + a
// sentinel error so callers can fall back to full-tree packing.
package gitutil

import (
	"bytes"
	"errors"
	"os/exec"
	"path/filepath"
	"strings"
)

// ErrNotARepo is returned when the path is not inside a git repository.
var ErrNotARepo = errors.New("not a git repository")

// ChangedFiles returns the set of files changed relative to a base ref.
//
//   - ref == "" or "HEAD" or "WORKTREE": returns the working-tree changes
//     (staged + unstaged + untracked, excluding deleted).
//   - otherwise: returns files differing between <ref> and the working tree
//     (git diff --name-status <ref>), excluding deletions.
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

	if extra, err := git(root, "ls-files", "--others", "--exclude-standard"); err == nil {
		for _, line := range strings.Split(extra, "\n") {
			line = strings.TrimSpace(line)
			if line != "" {
				files = append(files, filepath.ToSlash(line))
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
		path := strings.TrimSpace(line[3:])
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
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if line[0] == 'D' {
			continue
		}
		path := strings.TrimSpace(line[1:])
		if i := strings.Index(path, " -> "); i >= 0 {
			path = path[i+4:]
		}
		if path != "" {
			files = append(files, filepath.ToSlash(path))
		}
	}
	return files
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
