# Maintainers' commands that span Go and JavaScript; the JavaScript ones alone are pnpm scripts
# (package.json). `make` lists them.

# Your own settings, from .env.local (git-ignored; start from .env.local.example): NAME=value lines,
# exported to everything make runs.
-include .env.local
export

.DEFAULT_GOAL := help
.PHONY: help dev dev-tmux build test

help: ## List the targets
	@grep -E '^[a-z-]+:.*## ' $(MAKEFILE_LIST) | awk -F':.*## ' '{printf "  %-9s %s\n", $$1, $$2}'

TOOL := go tool -modfile=tools/go.mod

dev: ## Run the Go server and the SDK, each rebuilt on change, in this terminal (hivemind)
	@test -n "$$ANFRA_DEV_REPO" || { echo "set ANFRA_DEV_REPO, in .env.local, to a repo with Data Apps for anfra serve --apps to run in"; exit 1; }
	$(TOOL) hivemind Procfile.dev

dev-tmux: ## The same, under overmind in tmux: restart or attach to each process on its own
	@command -v tmux >/dev/null || { echo "make dev-tmux needs tmux, which overmind runs the processes in"; exit 1; }
	@test -n "$$ANFRA_DEV_REPO" || { echo "set ANFRA_DEV_REPO, in .env.local, to a repo with Data Apps for anfra serve --apps to run in"; exit 1; }
	$(TOOL) overmind start -f Procfile.dev

build: ## Build the Data App frontend, then anfra with it, into bin/, as a release is
	pnpm build:apps
	go build -o bin/ ./cmd/anfra

test: ## Run the Go and SDK tests
	go test ./...
	pnpm test:sdk
