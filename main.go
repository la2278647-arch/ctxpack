// Package main is the entry point for ctxpack.
//
// ctxpack is a fast, single-binary tool that packs a repository (or any
// directory tree) into an LLM-optimized context bundle. The same binary can
// also run as a Model Context Protocol (MCP) server over stdio, so AI agents
// (Claude Desktop, Cursor, Codex, ...) can call it to ingest a repo on demand.
//
// Build:
//
//	go build -o ctxpack .
//
// Usage:
//
//	ctxpack pack ./myrepo            # pack a repo to stdout (XML by default)
//	ctxpack map ./myrepo             # show a token-aware tree outline
//	ctxpack diff ./myrepo            # pack only files changed vs HEAD
//	ctxpack tokens ./myrepo          # total token estimate
//	ctxpack mcp                      # run as an MCP server on stdio
package main

import (
	"os"

	"github.com/la2278647-arch/ctxpack/internal/cli"
)

func main() {
	code := cli.Run(os.Args[1:])
	os.Exit(code)
}
