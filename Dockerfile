# syntax=docker/dockerfile:1
# One image = the server with the phone app bundled in. Build from the repo
# root:  docker compose up --build
# Or by hand, publishing on this machine only:
#   docker build -t yume-server .
#   docker run -p 127.0.0.1:7654:7654 -e YUME_ALLOW_OPEN_NETWORK=1 -v yume-data:/data yume-server
# The server binds 0.0.0.0 inside the container, so open auth mode (anyone
# who reaches the port acts as the owner) starts only with
# YUME_ALLOW_OPEN_NETWORK=1. Publish it on 127.0.0.1 or a private address
# (Tailscale), never a public one, or run with YUME_AUTH_MODE=required.
# Base images are pinned by digest; Dependabot proposes updates weekly.

# 1. Export the app for the web. The export is the same on every
# architecture, so it always runs on the build machine's own platform.
FROM --platform=$BUILDPLATFORM node:22-alpine@sha256:0a7108bf6c7bf5de370ffb1a3ed6be93d405b43ff159f681a8d18c0e2bc2e402 AS web
WORKDIR /mobile
ENV CI=1 EXPO_NO_TELEMETRY=1
COPY mobile/package.json mobile/package-lock.json ./
RUN npm ci
COPY mobile/ ./
RUN npx expo export --platform web --output-dir /out

# 2. Build the server with the export embedded, cross-compiled for the
# target platform (pure Go, so no emulation).
FROM --platform=$BUILDPLATFORM golang:1.26.11-alpine@sha256:ae52b5bc6ed045a9ed26ab03dc19327a802a9cc9aa8b27ed57f5c6b60579b0bb AS build
WORKDIR /src
COPY server/go.mod server/go.sum ./
RUN go mod download
COPY server/ ./
RUN rm -rf internal/webapp/dist && mkdir -p internal/webapp/dist
COPY --from=web /out/ internal/webapp/dist/
RUN test -f internal/webapp/dist/index.html
# The build identity shown in /health and Settings > About. docker compose
# passes YUME_VERSION and YUME_COMMIT; release builds pass the git tag and SHA.
ARG TARGETOS TARGETARCH
ARG VERSION=dev
ARG COMMIT=
RUN version="${VERSION#v}"; \
    CGO_ENABLED=0 GOOS="$TARGETOS" GOARCH="$TARGETARCH" go build -trimpath \
      -ldflags="-s -w -X main.Version=${version:-dev} -X main.Commit=${COMMIT}" \
      -o /yume-server .

# 3. Runtime: the binary, a shell for the entrypoint, and /data for everything
# else. The server runs as uid 10001; the entrypoint starts as root only to
# hand /data to that user (see server/docker/entrypoint.sh).
FROM alpine:3.24@sha256:294b683cb724975bec92580e1e685676bd4b50bda910ddb8c51d4cabeaec77e6
LABEL org.opencontainers.image.licenses="AGPL-3.0-only"
RUN apk add --no-cache ca-certificates su-exec \
 && addgroup -g 10001 yume && adduser -D -H -u 10001 -G yume yume \
 && mkdir /data && chown yume:yume /data
# Notices for the data and every bundled dependency; also served at /licenses.
COPY LICENSE LICENSES.md server/internal/notices/THIRD_PARTY_NOTICES.txt /usr/share/doc/yume/
COPY --from=build /yume-server /usr/local/bin/yume-server
COPY server/docker/entrypoint.sh /usr/local/bin/yume-entrypoint
RUN sed -i 's/\r$//' /usr/local/bin/yume-entrypoint && chmod +x /usr/local/bin/yume-entrypoint
VOLUME /data
EXPOSE 7654
# "Up" means the process answers; /health's dict field says whether lookups
# are ready. The first start downloads the dictionaries before serving.
HEALTHCHECK --interval=30s --timeout=5s --start-period=5m --retries=3 \
  CMD wget -q -O /dev/null http://127.0.0.1:7654/health || exit 1
ENTRYPOINT ["yume-entrypoint"]
