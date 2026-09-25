# ctxpack — single static binary, standard library only.
#
# Nothing in this Makefile touches the network. `go build`, `go vet` and `go
# test` operate entirely on the module cache, which for this project is empty
# by design (see CONTRIBUTING.md).

GO       ?= go
MOD      := github.com/la2278647-arch/ctxpack
VERSION  ?= 0.1.8
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

.PHONY: all build test vet fmt tidy check smoke ci release clean

all: check

## Build a binary for the host into ./bin
build:
	$(GO) build -trimpath -ldflags '$(LDFLAGS)' -o $(BIN) .

## Run the test suite
test:
	$(GO) test ./...

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
	$(GO) test ./...
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
ci: check smoke

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
	@cd dist && (sha256sum * > SHA256SUMS.txt 2>/dev/null || sha256 * > SHA256SUMS.txt) && cat SHA256SUMS.txt

clean:
	rm -rf bin dist
