// Package version holds the build-time identity shared by the CLI and the MCP
// server. It lives in its own package so the two do not have to import each
// other just to print the same version string.
package version

import "runtime"

// Module is the import path of the ctxpack module.
const Module = "github.com/la2278647-arch/ctxpack"

// Version is the semantic version of the running binary. Override it at build
// time with:
//
//	go build -ldflags "-X github.com/la2278647-arch/ctxpack/internal/version.Version=v1.2.3"
var Version = "0.1.9"

// BuildCommit is populated by CI when a tag is cut.
var BuildCommit = "dev"

// BuildDate is populated by CI when a tag is cut.
var BuildDate = "unknown"

// Info is a human-readable summary of how the binary was built.
func Info() string {
	return "ctxpack " + Version + " (" + runtime.GOOS + "/" + runtime.GOARCH +
		", " + runtime.Version() + ", commit " + BuildCommit + ", built " + BuildDate + ")"
}

// UserAgent is a short product token for HTTP clients.
func UserAgent() string {
	return "ctxpack/" + Version
}
