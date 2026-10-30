# Maintainers' commands that span Go and JavaScript; the JavaScript ones alone are pnpm scripts
# (package.json). `make` lists them.

# Your own settings, from .env.local (git-ignored; start from .env.local.example): NAME=value lines,
# exported to everything make runs.
-include .env.local
export

.DEFAULT_GOAL := help
.PHONY: help setup sidecars dev dev-tmux build build-dev test test-sidecars check

help: ## List the targets
	@grep -hE '^[a-z-]+:.*## ' $(MAKEFILE_LIST) | awk -F':.*## ' '{printf "  %-13s %s\n", $$1, $$2}'

TOOL := go tool -modfile=tools/go.mod

# The release a build from source comes after, from manifest.yml: anfra reports itself as
# <this>+<commit>[.dirty] (internal/meta). Exported, like every variable here, so air's build
# (make dev) injects it too.
ANFRA_BASE_VERSION := $(shell sed -n 's/^version: *//p' manifest.yml)
BUILD_FLAGS = -ldflags "-X github.com/holistics/anfra/internal/meta.base=$(ANFRA_BASE_VERSION) -X main.defaultCloudURL=$$cloud_url"

# The JavaScript dependencies, brought up to date before make dev starts its processes: after a
# branch switch or a lockfile change, they are what goes stale. Not --frozen-lockfile, as CI's is:
# a package.json you just edited would refuse it. The Go side fetches its modules on demand.
JS_DEPS := pnpm install

# The SDK, built once before make dev starts its processes when it never has been: the appserve
# dev server reads it from web/sdk/dist/, which the SDK's watcher only fills once it has built.
SDK_ONCE := @test -f web/sdk/dist/api.js || pnpm build:sdk

# The sidecars make dev and make test-sidecars run, and make build embeds, unless .env.local names
# your own: the releases manifest.yml pins, which scripts/sidecars.sh downloads into .sidecars/. It
# also checks, before any of them starts, that each sidecar is the pinned version.
SIDECARS := $(CURDIR)/.sidecars
dev dev-tmux test-sidecars build: ANFRA_NODE_BIN := $(or $(ANFRA_NODE_BIN),$(SIDECARS)/anfra-node)
dev dev-tmux test-sidecars build: ANFRA_CANAL_QUERY_BIN := $(or $(ANFRA_CANAL_QUERY_BIN),$(SIDECARS)/canal-query)

# Where make dev's Vite dev server serves the Data App frontend from source; anfra serve redirects
# its pages there. Override it in .env.local.
ANFRA_APPSERVE_DEV_URL ?= http://127.0.0.1:5173

setup: ## Set up a clone: the web workspace, git hooks, .env.local from its example, the sidecars
	pnpm install
	@if [ -e .env.local ]; then \
		echo ".env.local already exists: left as it is."; \
	else \
		cp .env.local.example .env.local; \
		echo "Wrote .env.local from .env.local.example. Set ANFRA_DEV_REPO: see DEVELOPMENT.md."; \
	fi
	scripts/sidecars.sh fetch

sidecars: ## Download the sidecar releases manifest.yml pins into .sidecars/, replacing others
	scripts/sidecars.sh fetch

dev: ## Run the Go server and the SDK, each rebuilt on change, in this terminal (hivemind)
	@test -n "$$ANFRA_DEV_REPO" || { echo "set ANFRA_DEV_REPO, in .env.local, to a repo with Data Apps for anfra serve to run in"; exit 1; }
	scripts/sidecars.sh check
	$(JS_DEPS)
	$(SDK_ONCE)
	$(TOOL) hivemind Procfile.dev

dev-tmux: ## The same, under overmind in tmux: restart or attach to each process on its own
	@command -v tmux >/dev/null || { echo "make dev-tmux needs tmux, which overmind runs the processes in"; exit 1; }
	@test -n "$$ANFRA_DEV_REPO" || { echo "set ANFRA_DEV_REPO, in .env.local, to a repo with Data Apps for anfra serve to run in"; exit 1; }
	scripts/sidecars.sh check
	$(JS_DEPS)
	$(SDK_ONCE)
	$(TOOL) overmind start -f Procfile.dev

# The sidecars are embedded whatever ANFRA_<NAME>_URL says, so the check must not leave them alone.
build: ## Build the Data App frontend, then anfra with it and the sidecars, into bin/, as a release is
	ANFRA_NODE_URL= ANFRA_CANAL_QUERY_URL= scripts/sidecars.sh check
	pnpm build:web
	mkdir -p internal/sidecar/anfranode/assets internal/sidecar/canalquery/assets
	cp "$$ANFRA_NODE_BIN" internal/sidecar/anfranode/assets/anfra-node
	cp "$$ANFRA_CANAL_QUERY_BIN" internal/sidecar/canalquery/assets/canal-query
	@cloud_url="$$(go run ./scripts/cloud-url)" && \
		go build -tags embed_sidecar $(BUILD_FLAGS) -o bin/ ./cmd/anfra

# Air rebuilds Go without rebuilding the frontend.
build-dev:
	@cloud_url="$$(go run ./scripts/cloud-url)" && \
		go build $(BUILD_FLAGS) -o ./tmp/anfra ./cmd/anfra

# The Go tests that run against real sidecars run only when .env.local names both; make
# test-sidecars runs them against the pinned releases.
REAL_SIDECARS = $(and $(ANFRA_NODE_BIN),$(ANFRA_CANAL_QUERY_BIN))

test: ## Run the Go, SDK, frontend and skills tests
	$(if $(REAL_SIDECARS),scripts/sidecars.sh check)
	go test ./...
	pnpm test:web
	pnpm skills:check
	$(if $(REAL_SIDECARS),,@echo "WARNING: the real-sidecar tests were skipped: ANFRA_NODE_BIN and ANFRA_CANAL_QUERY_BIN are not both set. make test-sidecars runs them.")

test-sidecars: ## Run the Go tests, those against real sidecars too (needs the dev warehouse: DEVELOPMENT.md)
	scripts/sidecars.sh check
	go test ./...

# The SDK's type declarations, which the frontend's typecheck reads (web/sdk/dist), are the one
# thing make check builds.
check: ## Lint and type-check everything, and check the API contract is fresh, as CI does
	go mod tidy --diff
	$(TOOL) golangci-lint run ./...
	scripts/openapi.sh diff
	pnpm --filter @holistics/anfra-sdk generate >/dev/null
	@git diff --exit-code --quiet -- web/sdk/src/api/schema.d.ts \
		|| { echo "web/sdk/src/api/schema.d.ts was stale: it is regenerated now, commit it"; exit 1; }
	pnpm --filter @holistics/anfra-sdk typecheck
	pnpm build:sdk >/dev/null
	pnpm --filter anfra-appserve typecheck
