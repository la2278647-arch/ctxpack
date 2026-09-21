// Package packer orchestrates the walker, token counter, and formatter into a
// single Pack entry point. When a token budget is supplied it greedily
// selects the most relevant files until the budget is reached, prioritizing
// documentation and entry points over large low-signal files.
package packer

import (
	"sort"
	"strings"

	"github.com/la2278647-arch/ctxpack/internal/counter"
	"github.com/la2278647-arch/ctxpack/internal/format"
	"github.com/la2278647-arch/ctxpack/internal/walker"
)

// Options configures packing.
type Options struct {
	// Walker options.
	Walker walker.Options
	// Budget is the max tokens to include (0 = unlimited).
	Budget int
	// Files overrides selection: when non-empty, only these relative paths are
	// packed (used by diff mode). Ignored when empty.
	Files []string
}

// Pack walks root, counts tokens, applies an optional budget, and returns a
// format.Bundle ready for rendering.
func Pack(root string, opts Options) (*format.Bundle, error) {
	res, err := walker.Walk(root, opts.Walker)
	if err != nil {
		return nil, err
	}
	cnt := counter.NewDefault()

	var files []format.File
	for _, fe := range res.Files {
		if len(opts.Files) > 0 && !inSet(fe.RelPath, opts.Files) {
			continue
		}
		f := format.File{
			Path:   fe.RelPath,
			Bytes:  int(fe.Size),
			Binary: fe.IsBinary,
		}
		if !fe.IsBinary && fe.Content != nil {
			e := cnt.EstimateBytes(fe.Content)
			f.Tokens = e.Tokens
			f.Content = string(fe.Content)
		} else {
			e := cnt.Estimate(fe.RelPath)
			f.Tokens = e.Tokens
		}
		files = append(files, f)
	}

	if opts.Budget > 0 {
		files = applyBudget(files, opts.Budget)
	} else {
		sort.SliceStable(files, func(i, j int) bool {
			return files[i].Path < files[j].Path
		})
	}

	bundle := &format.Bundle{
		Root:    res.Root,
		Files:   files,
		Skipped: res.Skipped,
	}
	for _, f := range files {
		bundle.TotalTokens += f.Tokens
		bundle.TotalBytes += f.Bytes
	}
	return bundle, nil
}

// applyBudget greedily selects files by priority until the token budget is
// reached, then returns the selected subset in path order.
func applyBudget(files []format.File, budget int) []format.File {
	type scored struct {
		f     format.File
		score int
	}
	scoredFiles := make([]scored, len(files))
	for i, f := range files {
		scoredFiles[i] = scored{f: f, score: priority(f)}
	}
	sort.SliceStable(scoredFiles, func(i, j int) bool {
		if scoredFiles[i].score != scoredFiles[j].score {
			return scoredFiles[i].score > scoredFiles[j].score
		}
		return scoredFiles[i].f.Tokens < scoredFiles[j].f.Tokens
	})

	var kept []format.File
	used := 0
	for _, s := range scoredFiles {
		if used+s.f.Tokens > budget {
			continue
		}
		kept = append(kept, s.f)
		used += s.f.Tokens
	}
	sort.SliceStable(kept, func(i, j int) bool { return kept[i].Path < kept[j].Path })
	return kept
}

// priority scores a file for budget selection. Higher = more important.
func priority(f format.File) int {
	low := strings.ToLower(f.Path)
	base := low
	if i := strings.LastIndex(low, "/"); i >= 0 {
		base = low[i+1:]
	}
	switch {
	case strings.HasPrefix(base, "readme"):
		return 1000
	case strings.HasSuffix(low, ".md") || strings.HasSuffix(low, ".txt"):
		return 500
	case strings.HasSuffix(low, "main.go") || strings.HasSuffix(low, "index.ts") ||
		strings.HasSuffix(low, "index.js") || strings.HasSuffix(low, "mod.go"):
		return 400
	case strings.HasSuffix(low, "_test.go") || strings.HasSuffix(low, ".test.js") ||
		strings.HasSuffix(low, ".spec.ts") || strings.HasSuffix(low, ".test.ts"):
		return 50
	case strings.HasSuffix(low, ".go") || strings.HasSuffix(low, ".ts") ||
		strings.HasSuffix(low, ".js") || strings.HasSuffix(low, ".py") ||
		strings.HasSuffix(low, ".rs") || strings.HasSuffix(low, ".java") ||
		strings.HasSuffix(low, ".rb") || strings.HasSuffix(low, ".cs"):
		return 200
	case strings.HasSuffix(low, ".yml") || strings.HasSuffix(low, ".yaml") ||
		strings.HasSuffix(low, ".toml") || strings.HasSuffix(low, ".json"):
		return 150
	case strings.HasSuffix(low, ".sh"):
		return 100
	}
	return 10
}

func inSet(s string, set []string) bool {
	for _, x := range set {
		if x == s {
			return true
		}
	}
	return false
}
