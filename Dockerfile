# ctxpack — multi-stage build, static binary, minimal runtime image.
#
# Build:
#   docker build -t ctxpack .
#
# Run:
#   docker run --rm -v "$(pwd):/repo:ro" ctxpack map /repo
#   docker run --rm -v "$(pwd):/repo:ro" ctxpack pack /repo --budget 8000 --format markdown
#
# The final image is a single static binary on alpine:latest (~50 MB total).
# CGO_ENABLED=0 produces a statically-linked binary that needs no shared
# libraries at runtime, so the image stays small and portable.

# --- build stage ---
FROM golang:1.21-alpine AS build

WORKDIR /src

# Install git for the version stamp, then fetch only go.mod.
RUN apk add --no-cache git

COPY go.mod ./
RUN go mod download 2>/dev/null || true

# Copy the source and build.
COPY . .

ARG VERSION=dev
ARG COMMIT=dev
ARG DATE=unknown

RUN CGO_ENABLED=0 go build -trimpath \
    -ldflags "-X github.com/la2278647-arch/ctxpack/internal/version.Version=${VERSION} \
              -X github.com/la2278647-arch/ctxpack/internal/version.BuildCommit=${COMMIT} \
              -X github.com/la2278647-arch/ctxpack/internal/version.BuildDate=${DATE}" \
    -o /ctxpack .

# --- runtime stage ---
FROM alpine:latest

RUN apk add --no-cache ca-certificates

COPY --from=build /ctxpack /usr/local/bin/ctxpack

# The entrypoint is the binary itself: any arguments become ctxpack arguments.
# A read-only bind mount (-v "$(pwd):/repo:ro") is safe: ctxpack never writes
# to the tree it reads.
ENTRYPOINT ["ctxpack"]
