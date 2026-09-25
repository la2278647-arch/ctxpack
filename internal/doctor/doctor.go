// Package doctor gathers environment diagnostics shared by the CLI's
// `doctor` command and the MCP server's `doctor` tool. Both frontends call
// this package rather than duplicating the checks, so the two cannot report
// different facts about the same machine — the same "shared engine" rule the
// packer and repomap packages already follow.
package doctor

import (
	"fmt"
	"io"
	"os/exec"
	"runtime"
	"sort"
	"strings"

	"github.com/la2278647-arch/ctxpack/internal/counter"
	"github.com/la2278647-arch/ctxpack/internal/version"
)

// VendorCount is one vendor and the number of registered models it owns.
type VendorCount struct {
	Vendor string `json:"vendor"`
	Models int    `json:"models"`
}

// Report is a snapshot of the environment a ctxpack binary runs in: the build
// identity, the Go runtime, git availability, and the model registry. Every
// field is populated by Gather, so a zero Report is only useful for unit
// testing the rendering paths.
type Report struct {
	Version         string        `json:"version"`
	GoVersion       string        `json:"go_version"`
	Platform        string        `json:"platform"`
	Git             string        `json:"git"`
	GitVersion      string        `json:"git_version,omitempty"`
	GitError        string        `json:"git_error,omitempty"`
	GitVersionError string        `json:"git_version_error,omitempty"`
	ModelCount      int           `json:"model_count"`
	ModelVendors    int           `json:"model_vendors"`
	Vendors         []VendorCount `json:"vendors,omitempty"`
}

// Gather snapshots the real environment: build identity, runtime, git, and the
// model registry.
func Gather() Report {
	r := Report{
		Version:   version.Info(),
		GoVersion: runtime.Version(),
		Platform:  runtime.GOOS + "/" + runtime.GOARCH,
	}
	r.Git, r.GitError = gitPath()
	if r.GitError == "" {
		out, err := exec.Command("git", "--version").Output()
		if err != nil {
			r.GitVersion = "unknown"
			r.GitVersionError = err.Error()
		} else {
			r.GitVersion = strings.TrimSpace(string(out))
		}
	}
	models := counter.Models()
	r.ModelCount = len(models)
	r.Vendors = VendorBreakdown(models)
	r.ModelVendors = len(r.Vendors)
	return r
}

// gitPath returns the path of the git executable and an error string when git
// is missing. A missing git means `diff` cannot run at all, so the absence is
// worth surfacing rather than hiding.
func gitPath() (string, string) {
	p, err := exec.LookPath("git")
	if err != nil {
		return "", err.Error()
	}
	return p, ""
}

// VendorBreakdown groups the registered models by vendor, sorted by model
// count descending and vendor name ascending for ties. The sort order is what
// makes a truncated report deterministic: a `--top 3` report keeps the three
// biggest vendors and would otherwise vary from run to run.
func VendorBreakdown(models []counter.Model) []VendorCount {
	byVendor := make(map[string]int)
	for _, m := range models {
		byVendor[m.Vendor]++
	}
	out := make([]VendorCount, 0, len(byVendor))
	for name, n := range byVendor {
		out = append(out, VendorCount{Vendor: name, Models: n})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Models != out[j].Models {
			return out[i].Models > out[j].Models
		}
		return out[i].Vendor < out[j].Vendor
	})
	return out
}

// Text writes the human-readable report to w. When top is greater than zero
// and smaller than the number of vendors, the vendor breakdown is truncated to
// the top N entries. The "N models, M vendors" line always reports the full
// counts, so truncating the breakdown cannot understate the registry — a
// reader of a truncated report must still learn how big it actually is.
func (r Report) Text(w io.Writer, top int) {
	fmt.Fprintln(w, "ctxpack diagnostics:")
	fmt.Fprintf(w, "  Version:   %s\n", r.Version)
	fmt.Fprintf(w, "  Go:        %s\n", r.GoVersion)
	fmt.Fprintf(w, "  Platform:  %s\n", r.Platform)
	if r.Git != "" {
		gitVer := r.GitVersion
		if gitVer == "" {
			gitVer = "unknown"
		}
		fmt.Fprintf(w, "  Git:       %s (%s)\n", r.Git, gitVer)
	} else {
		fmt.Fprintln(w, "  Git:       not found (diff command will not work)")
	}
	fmt.Fprintf(w, "  Models:    %d models, %d vendors\n", r.ModelCount, r.ModelVendors)
	if len(r.Vendors) > 0 {
		fmt.Fprintln(w, "  Vendor breakdown:")
		for _, vc := range TopVendors(r.Vendors, top) {
			fmt.Fprintf(w, "    %-15s %d model(s)\n", vc.Vendor, vc.Models)
		}
	}
}

// TopVendors truncates a breakdown when top is between 1 and len(vs)-1.
// A zero or negative top, and a top at or above the length, return the whole
// slice unchanged rather than an empty one — the caller asked for "all of them".
func TopVendors(vs []VendorCount, top int) []VendorCount {
	if top > 0 && top < len(vs) {
		return vs[:top]
	}
	return vs
}
