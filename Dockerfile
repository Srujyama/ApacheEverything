# syntax=docker/dockerfile:1.7
#
# Sunny — open-source observability for physical infrastructure.
# Multi-stage build: web → server → minimal runtime.
#
# go-duckdb/v2 ships per-platform precompiled bindings via cgo, so we cannot
# use CGO_ENABLED=0. The runtime stage stays small (~30MB) because the cgo
# bindings are statically linked into the binary.

# ---------- Stage 1: build the React frontend ----------
FROM node:20-alpine AS web

RUN corepack enable && corepack prepare pnpm@10.33.0 --activate
WORKDIR /repo

# Copy what pnpm needs to resolve the workspace, for cache friendliness.
COPY package.json pnpm-workspace.yaml ./
COPY apps/web/package.json apps/web/
COPY packages/core/package.json packages/core/
COPY packages/sdk-ts/package.json packages/sdk-ts/

RUN pnpm install --frozen-lockfile=false

# Copy the rest of the web sources and build.
COPY apps/web ./apps/web
COPY packages/core ./packages/core
COPY packages/sdk-ts ./packages/sdk-ts

RUN pnpm --filter @sunny/web build

# ---------- Stage 2: build the Go server ----------
FROM golang:1.25-alpine AS server

# musl-dev + gcc for cgo (go-duckdb requires it).
RUN apk add --no-cache gcc musl-dev

WORKDIR /repo

# Cache module downloads. Copy go.work and every module's go.mod first.
COPY go.work ./
COPY apps/server/go.mod apps/server/
COPY packages/sdk-go/go.mod packages/sdk-go/
COPY packages/cli/go.mod packages/cli/
COPY connectors/go.mod connectors/

RUN cd apps/server && go mod download

# Copy Go sources.
COPY apps/server ./apps/server
COPY packages/sdk-go ./packages/sdk-go
COPY packages/cli ./packages/cli
COPY connectors ./connectors

# Pull the freshly built frontend into the embed dir before building.
COPY --from=web /repo/apps/web/dist ./apps/server/internal/web/dist

# Statically link cgo via -extldflags so the resulting binary runs on plain alpine.
RUN cd apps/server && \
    CGO_ENABLED=1 go build -trimpath \
        -ldflags="-s -w -extldflags=-static" \
        -o /out/sunny ./cmd/sunny

# Build the helper CLI too.
RUN cd packages/cli && \
    CGO_ENABLED=0 go build -trimpath \
        -ldflags="-s -w" \
        -o /out/sunny-cli ./cmd/sunny

# ---------- Stage 3: minimal runtime image ----------
FROM alpine:3.20

RUN apk add --no-cache ca-certificates tzdata && \
    addgroup -S sunny && adduser -S sunny -G sunny && \
    mkdir -p /data && chown sunny:sunny /data

COPY --from=server /out/sunny /usr/local/bin/sunny
COPY --from=server /out/sunny-cli /usr/local/bin/sunny-cli

USER sunny
WORKDIR /data
ENV SUNNY_ADDR=:3000 \
    SUNNY_DATA_DIR=/data
EXPOSE 3000

# Health check hits the always-public /api/health endpoint.
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD wget -q -O - http://localhost:3000/api/health > /dev/null || exit 1

ENTRYPOINT ["/usr/local/bin/sunny"]
