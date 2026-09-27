# ctxpack — single static binary, standard library only.
#
# Nothing in this Makefile touches the network. `go build`, `go vet` and `go
# test` operate entirely on the module cache, which for this project is empty
# by design (see CONTRIBUTING.md).

GO       ?= go
MOD      := github.com/la2278647-arch/ctxpack
VERSION  ?= 0.1.11
COMMIT   := $(shell git rev-parse --short HEAD 2>/dev/null || echo dev)
DATE     := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS  := -X $(MOD)/internal/version.Version=$(VERSION) -X $(MOD)/internal/version.BuildCommit=$(COMMIT) -X $(MOD)/internal/version.BuildDate=$(DATE)

# A Windows host runs the .exe; everywhere else the extensionless name.
# OS is make's built-in and is Windows_NT on Windows, including git-bash.
ifeq ($(OS),Windows_NT)
  BIN := bin/ctxpack.exe
else
  BIN := bin/ctxpack
endif

OSARCHES := windows-amd64 windows-386 windows-arm64 \
            linux-amd64 linux-386 linux-arm64 \
            darwin-amd64 darwin-arm64

.PHONY: all build test vet fmt tidy check smoke cicheck ci release clean

all: check

## Build a binary for the host into ./bin
build:
	$(GO) build -trimpath -ldflags '$(LDFLAGS)' -o $(BIN) .

## Run the test suite. -count=1: a warm build cache must not be able to report
## a green suite for code that has changed since the last real run.
test:
	$(GO) test ./... -count=1

vet:
	$(GO) vet ./...

## Rewrap all Go files
fmt:
	gofmt -w $$(find . -name '*.go' -not -path './.git/*')

tidy:
	$(GO) mod tidy

## The gate CI runs: formatting, vet, tests, and the stdlib-only assertion.
## Requires make + git + go plus a POSIX shell with find.
check:
	@echo "--- gofmt ---"
	@fmtout="$$(gofmt -l $$(find . -name '*.go' -not -path './.git/*'))" \
		&& if [ -n "$$fmtout" ]; then echo "gofmt needed on:"; echo "$$fmtout"; exit 1; else echo "gofmt clean"; fi
	@echo "--- go vet ---"
	$(GO) vet ./...
	@echo "--- go test ---"
	$(GO) test ./... -count=1
	@echo "--- stdlib only ---"
	@if [ -s go.sum ]; then echo "go.sum must stay empty; this module uses no external packages"; exit 1; fi
	@mods="$$( $(GO) list -m all | wc -l | tr -d ' ' )" \
		&& if [ "$$mods" != "1" ]; then echo "expected exactly one module, got $$mods"; exit 1; fi
	@echo "only the module itself — zero dependencies"
	@echo "check passed"

## Build, then exercise every command against the source tree itself.
## Delegates to scripts/smoke.sh so the suite has one source of truth that CI
## calls identically instead of keeping a second copy in this Makefile.
## Requires bash.
smoke: build
	bash scripts/smoke.sh '$(BIN)'

## check plus smoke: the whole CI gate from the command line.
ci: check smoke cicheck examplescheck commandscheck installerscheck

## Audit docs/ci.yml, the workflow that lives in docs/ because the token that
## publishes this repository lacks the `workflow` scope and so can never run it
## on GitHub. A workflow that is never executed rots silently, so this
## syntax-checks every run: block, verifies the scripts and targets it names
## exist, and fails if its gate drifts from `make check`. Requires bash + awk.
cicheck:
	bash scripts/check-ci.sh

## Check the committed examples. Regenerates the two self-snapshots and fails
## if they no longer match a fresh run, so a tree change that skips the
## regeneration step shows up as drift instead of shipping. Also re-derives the
## demo numbers documented in docs/examples.md from a ../ctxpack-demo checkout
## when one is beside this repository; without one that half reports SKIP rather
## than failing, so this target is still useful on a bare clone. Requires bash.
examplescheck: build
	bash scripts/check-examples.sh

## Run every ctxpack invocation the current docs show a reader. Extracts the
## commands from README.md, docs/ci.yml, docs/examples.md and docs/promote.md,
## runs each against a real scratch repository, and fails on the first one the
## binary rejects, so a wrong flag or a rejected value cannot ship as an
## instruction. Release notes are skipped on purpose: they are history, and one
## quotes `ctxpack models --bogus` as an example of an error message. It also
## confirms each CTXPACK_* variable README names is still read by the CLI or by
## an installer. Requires bash + awk.
commandscheck: build
	bash scripts/check-commands.sh

## Cross-check the two installers against each other and against what the Makefile
## publishes. The installers are what a reader runs blind, and nothing in `check`
## touches them. Runs every row of OSARCHES through the asset name each of the
## three sources computes, feeds a real Makefile-produced SHA256SUMS.txt through
## each installer's own parser, and asserts both installers retry the same number
## of times. Parses install.sh with `bash -n` and install.ps1 with
## [scriptblock]::Create; that second check reports SKIP when no pwsh or
## powershell.exe is on PATH, since the Parser API is denied in a restricted
## PowerShell. Requires bash + awk.
installerscheck:
	bash scripts/check-installers.sh

## Verify a published release against this repository. The only target that opens
## a network connection, so it is deliberately not in `ci`: a release is
## published from another machine by another process, and a green test suite
## says nothing about what a reader actually receives. Fetches the manifest and
## checks its line endings and text-mode format, confirms it names exactly the
## assets the Makefile would build, re-hashes the archive the Homebrew formula
## points at, and compares every scoop bucket hash with the manifest. --install
## also runs install.sh against a temp dir. --base takes a mirror url so the
## check can be pointed at a local fixture instead of upstream. The tap and
## bucket are siblings of this repository, read from ../homebrew-tap and
## ../scoop-bucket, or from CTXPACK_TAP and CTXPACK_BUCKET, and report SKIP
## when absent. Requires curl + awk + sha256sum (or shasum).
releasecheck:
	bash scripts/check-release.sh

## Cross-compile the release set into ./dist, then checksum them.
release:
	rm -rf dist && mkdir -p dist
	@for osarch in $(OSARCHES); do \
		os=$${osarch%-*}; arch=$${osarch#*-}; ext=""; \
		if [ "$$os" = windows ]; then ext=".exe"; fi; \
		echo "building $$os-$$arch"; \
		CGO_ENABLED=0 GOOS=$$os GOARCH=$$arch $(GO) build -trimpath -ldflags '$(LDFLAGS)' \
			-o "dist/ctxpack_$(VERSION)_$$os_$$arch$$ext" . || exit 1; \
	done
	@cd dist && (sha256sum -t * > SHA256SUMS.txt 2>/dev/null || sha256 -a 256 * > SHA256SUMS.txt) && cat SHA256SUMS.txt

## Remove build output and coverage profiles. .gitignore covers every one of
## these, so a stale working tree still reads "clean" in git status while a
## month-old cover.out from an earlier go test -coverprofile sits in it.
clean:
	rm -rf bin dist
	rm -f cover cover.out coverage.txt coverage.html *.out *.test
