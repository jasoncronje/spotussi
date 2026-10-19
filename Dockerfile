# syntax=docker/dockerfile:1
# Multi-arch image (linux/amd64, linux/arm64): build stages run on the
# build platform and cross-compile, so no emulation is needed.

# ---- web: SvelteKit static build ----
FROM --platform=$BUILDPLATFORM node:26-alpine AS web
RUN npm install -g pnpm@12.14.0
WORKDIR /src/web
COPY web/package.json web/pnpm-lock.yaml web/pnpm-workspace.yaml web/.npmrc ./
RUN pnpm install --frozen-lockfile
COPY api/openapi.yaml /src/api/openapi.yaml
COPY web/ ./
RUN pnpm build

# ---- go: static binary with the UI embedded ----
FROM --platform=$BUILDPLATFORM golang:1.27-alpine AS go
ARG TARGETOS TARGETARCH
ARG VERSION=dev
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd cmd
COPY internal internal
COPY api api
COPY web/embed.go web/embed.go
COPY --from=web /src/web/build web/build
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" -o /out/spotussi ./cmd/spotussi \
 && mkdir -p /out/data

# ---- final: distroless static (CA certs, tzdata, nonroot user, no shell) ----
FROM gcr.io/distroless/static-debian13:nonroot
COPY --from=go /out/spotussi /usr/local/bin/spotussi
COPY --from=go --chown=65532:65532 /out/data /data
ENV DATA_DIR=/data \
    PORT=8080
VOLUME /data
EXPOSE 8080
USER nonroot:nonroot
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD ["/usr/local/bin/spotussi", "health"]
ENTRYPOINT ["/usr/local/bin/spotussi"]
CMD ["serve"]
