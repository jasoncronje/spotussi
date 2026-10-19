# Dev workflow. Toolchains come from mise.toml (project-local Go and Node);
# `mise exec --` works without activating mise in your shell.
#
#   make dev    mock API + Vite dev server (HMR) at https://spotussi.localhost
#   make mock   the built binary in mock mode at https://spotussi-mock.localhost
#   make build  web build + static Go binary in bin/spotussi
#   make test   Go tests (incl. OpenAPI contract tests) + svelte-check
#   make lint   gofmt, go vet, prettier, eslint, generated-client freshness
#   make bench  performance budgets on the reference dataset (as CI runs them)
#
# Browser-facing servers run through Portless (named HTTPS .localhost URLs,
# worktree-prefixed in git worktrees). The underlying commands still work
# directly: `cd web && pnpm dev`, `go run ./cmd/spotussi serve --mock`.

MISE    ?= mise exec --
GO      := $(MISE) go
PNPM    := $(MISE) pnpm
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X main.version=$(VERSION)

# The Go API for `make dev` listens on a free loopback port unless API_PORT
# is set (so parallel worktrees don't collide); Vite proxies /api and
# /_mock to it.
DEV_FLAGS ?= --mock

.PHONY: dev mock build web go test lint bench gen-api check-api docker clean

web/node_modules/.modules.yaml: web/package.json web/pnpm-lock.yaml
	cd web && $(PNPM) install --frozen-lockfile

## dev: mock API (Go, loopback port) + Vite dev server through Portless
dev: web/node_modules/.modules.yaml
	$(GO) build -o bin/spotussi-dev ./cmd/spotussi
	@set -e; \
	port=$${API_PORT:-$$(python3 -c 'import socket;s=socket.socket();s.bind(("127.0.0.1",0));print(s.getsockname()[1])')}; \
	HOST=127.0.0.1 PORT=$$port ./bin/spotussi-dev serve $(DEV_FLAGS) & api=$$!; \
	trap 'kill $$api 2>/dev/null' EXIT INT TERM; \
	cd web && SPOTUSSI_API_URL=http://127.0.0.1:$$port portless run --name spotussi pnpm dev

## mock: built binary (embedded UI) in mock mode through Portless
mock: build
	portless run --name spotussi-mock ./bin/spotussi serve --mock

## build: production web build embedded into a static Go binary
build: web go

web: web/node_modules/.modules.yaml
	cd web && $(PNPM) build

go:
	CGO_ENABLED=0 $(GO) build -trimpath -ldflags '$(LDFLAGS)' -o bin/spotussi ./cmd/spotussi

## test: Go unit + contract tests, svelte-check
test: web/node_modules/.modules.yaml
	$(GO) test ./...
	cd web && $(PNPM) check

## lint: formatting, vet, eslint/prettier, generated client up to date
lint: web/node_modules/.modules.yaml check-api
	@out=$$($(MISE) gofmt -l cmd internal web/embed.go); if [ -n "$$out" ]; then echo "gofmt needed:"; echo "$$out"; exit 1; fi
	$(GO) vet ./...
	cd web && $(PNPM) lint

## bench: rebuild, dashboard and History budgets (docs/project/plan.md) on the full
## reference dataset; fails past BENCH_FACTOR × budget (CI uses 2.5)
BENCH_FACTOR ?= 1
bench:
	SPOTUSSI_BENCH_BUDGET_FACTOR=$(BENCH_FACTOR) $(GO) test -run '^$$' -bench Live -benchtime 1x -timeout 15m ./internal/live

## gen-api: regenerate the TypeScript client types from api/openapi.yaml
gen-api: web/node_modules/.modules.yaml
	cd web && $(PNPM) gen:api

check-api: gen-api
	@git diff --exit-code -- web/src/lib/api/schema.d.ts || (echo "schema.d.ts is stale: run make gen-api and commit"; exit 1)

docker:
	docker build --build-arg VERSION=$(VERSION) -t spotussi:dev .

clean:
	rm -rf bin web/build/spa web/.svelte-kit
