# syntax=docker/dockerfile:1

# lgtm evaluates a code change and nothing else: no GitHub API, no network.
# Anything platform-specific (trust, approvals, check runs) belongs to the
# caller, which passes trust in as --trusted and reads the JSON report back.

# go-tree-sitter is cgo, so the builder needs a C toolchain and the runtime a
# matching glibc. Both stages are bookworm for that reason.
FROM golang:1.25-bookworm AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .

# `docker build --target test .` runs the suite. git is needed by the git-mode
# tests and is already in the golang image.
FROM builder AS test
RUN go test ./...

FROM builder AS build
ARG VERSION=dev
RUN CGO_ENABLED=1 go build -trimpath -ldflags="-s -w -X main.version=${VERSION}" -o /out/lgtm ./cmd/lgtm

FROM debian:bookworm-slim
LABEL org.opencontainers.image.source="https://github.com/donaldwasserman/lgtm" \
      org.opencontainers.image.description="Decides whether a pull request needs a human review." \
      org.opencontainers.image.licenses="MIT"
RUN apt-get update \
 && apt-get install -y --no-install-recommends git ca-certificates \
 && rm -rf /var/lib/apt/lists/* \
 && useradd --create-home --uid 10001 lgtm \
 # A mounted checkout is owned by the host's uid, not ours; without this git
 # refuses to read it ("dubious ownership").
 && git config --system --add safe.directory '*'
COPY --from=build /out/lgtm /usr/local/bin/lgtm
USER lgtm
WORKDIR /repo
ENTRYPOINT ["lgtm"]
