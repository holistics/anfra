# Maintainers' commands that span Go and JavaScript; the JavaScript ones alone are pnpm scripts
# (package.json). `make` lists them.

# Your own settings, from .env.local (git-ignored; start from .env.local.example): NAME=value lines,
# exported to everything make runs.
-include .env.local
export

.DEFAULT_GOAL := help
.PHONY: help setup dev dev-tmux build test check

help: ## List the targets
	@grep -hE '^[a-z-]+:.*## ' $(MAKEFILE_LIST) | awk -F':.*## ' '{printf "  %-9s %s\n", $$1, $$2}'

TOOL := go tool -modfile=tools/go.mod

# The release a build from source comes after, from manifest.yml: anfra reports itself as
# <this>+<commit>[.dirty] (internal/meta). Exported, like every variable here, so air's build
# (make dev) injects it too.
ANFRA_BASE_VERSION := $(shell sed -n 's/^version: *//p' manifest.yml)
BUILD_FLAGS := -ldflags "-X github.com/holistics/anfra/internal/meta.base=$(ANFRA_BASE_VERSION)"

# The JavaScript dependencies, brought up to date before make dev starts its processes: after a
# branch switch or a lockfile change, they are what goes stale. Not --frozen-lockfile, as CI's is:
# a package.json you just edited would refuse it. The Go side fetches its modules on demand.
JS_DEPS := pnpm install

# The SDK, built once before make dev starts its processes when it never has been: the appserve
# dev server reads it from web/sdk/dist/, which the SDK's watcher only fills once it has built.
SDK_ONCE := @test -f web/sdk/dist/api.js || pnpm build:sdk

# Where make dev's Vite dev server serves the Data App frontend from source; anfra serve redirects
# its pages there. Override it in .env.local.
ANFRA_APPSERVE_DEV_URL ?= http://127.0.0.1:5173

setup: ## Set up a clone: the web workspace and git hooks, and .env.local from its example
	pnpm install
	@if [ -e .env.local ]; then \
		echo ".env.local already exists: left as it is."; \
	else \
		cp .env.local.example .env.local; \
		echo "Wrote .env.local from .env.local.example. Set ANFRA_DEV_REPO, and the sidecars (ANFRA_NODE_BIN"; \
		echo "and ANFRA_CANAL_QUERY_BIN, or their _URL): see DEVELOPMENT.md."; \
	fi

dev: ## Run the Go server and the SDK, each rebuilt on change, in this terminal (hivemind)
	@test -n "$$ANFRA_DEV_REPO" || { echo "set ANFRA_DEV_REPO, in .env.local, to a repo with Data Apps for anfra serve to run in"; exit 1; }
	$(JS_DEPS)
	$(SDK_ONCE)
	$(TOOL) hivemind Procfile.dev

dev-tmux: ## The same, under overmind in tmux: restart or attach to each process on its own
	@command -v tmux >/dev/null || { echo "make dev-tmux needs tmux, which overmind runs the processes in"; exit 1; }
	@test -n "$$ANFRA_DEV_REPO" || { echo "set ANFRA_DEV_REPO, in .env.local, to a repo with Data Apps for anfra serve to run in"; exit 1; }
	$(JS_DEPS)
	$(SDK_ONCE)
	$(TOOL) overmind start -f Procfile.dev

build: ## Build the Data App frontend, then anfra with it, into bin/, as a release is
	pnpm build:web
	go build $(BUILD_FLAGS) -o bin/ ./cmd/anfra

test: ## Run the Go, SDK and frontend tests
	go test ./...
	pnpm test:web

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
