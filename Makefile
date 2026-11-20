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
BUILD_FLAGS = -ldflags "-X github.com/holistics/anfra/internal/meta.base=$(ANFRA_BASE_VERSION) -X 'main.defaultCloudURL=$$cloud_url' -X 'main.defaultCloudSupportEmail=$$ANFRA_CLOUD_SUPPORT_EMAIL'"

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
dev dev-tmux test-sidecars build: ANFRA_CANAL_BIN := $(or $(ANFRA_CANAL_BIN),$(SIDECARS)/anfra-canal)

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
# A release embeds anfra-canal, never a service built alone.
build: ## Build the Data App frontend, then anfra with it and the sidecars, into bin/, as a release is
	ANFRA_NODE_URL= ANFRA_CANAL_QUERY_BIN= scripts/sidecars.sh check
	pnpm build:web
	mkdir -p internal/sidecar/anfranode/assets internal/sidecar/anfracanal/assets
	cp "$$ANFRA_NODE_BIN" internal/sidecar/anfranode/assets/anfra-node
	cp "$$ANFRA_CANAL_BIN" internal/sidecar/anfracanal/assets/anfra-canal
	@cloud_url="$$(go run ./scripts/cloud-url)" && \
		go build -tags embed_sidecar $(BUILD_FLAGS) -o bin/ ./cmd/anfra

# Air rebuilds Go without rebuilding the frontend.
build-dev:
	@cloud_url="$$(go run ./scripts/cloud-url)" && \
		go build $(BUILD_FLAGS) -o ./tmp/anfra ./cmd/anfra

# The Go tests that run against real sidecars run only when .env.local names anfra-node and
# anfra-canal (or canal-query alone); make test-sidecars runs them against the pinned releases.
REAL_SIDECARS = $(and $(ANFRA_NODE_BIN),$(or $(ANFRA_CANAL_BIN),$(ANFRA_CANAL_QUERY_BIN)))

test-sidecars: ## Run the Go tests, those against real sidecars too (needs the dev warehouse: DEVELOPMENT.md)
	scripts/sidecars.sh check
	go test ./...

# What CI checks (docs/designs/ci.md), by CI job: make ci-<area> runs what that job runs, and CI's
# job runs it too, so the two can't drift. Each area's checks are in two halves: <area>-static
# (lint, type-checks, generated files fresh) and <area>-runtime (builds and tests). make check runs
# every static half, make test every runtime half, and make ci the areas your branch changed.
#
# What only CI can check stays in the workflow: commitlint, the dependency review, and whether a
# release PR is the bot's.
.PHONY: ci ci-lint ci-go ci-api ci-web ci-skills ci-manifest
.PHONY: lint-static go-static go-runtime api-static web-static web-runtime skills-runtime manifest-static manifest-runtime

CI_AREAS := lint go api web skills manifest

# The branch a change lands on, which make ci and the API's breaking-change check compare with.
CI_BASE ?= origin/main

# The Go tests' flags. CI's are the same: -race finds data races the tests run into.
GO_TEST_FLAGS ?= -race

ci: ## Run the checks CI would for your branch: the areas it changed since origin/main, uncommitted changes included
	@base="$$(git merge-base $(CI_BASE) HEAD)" && \
		areas="$$(scripts/ci-areas.sh "$$base" | sed -n 's/=true$$//p')" && \
		run="" && skipped="" && \
		for area in $(CI_AREAS); do \
			if echo "$$areas" | grep -qx "$$area"; then run="$${run:+$$run }$$area"; else skipped="$${skipped:+$$skipped }$$area"; fi; \
		done && \
		echo "Compared with $(CI_BASE) (merge base $$(git rev-parse --short "$$base")), uncommitted changes included:" && \
		echo "  run:     $${run:-none}" && \
		echo "  skipped: $${skipped:-none}" && \
		echo "  CI only: commitlint, dependency review, the release PR check" && \
		for area in $$run; do $(MAKE) --no-print-directory ci-$$area || exit 1; done

ci-lint: lint-static ## Run CI's lint job: golangci-lint
ci-go: go-static go-runtime ## Run CI's go job: tidy, generated SQL code, vet, build, the Go tests
ci-api: api-static ## Run CI's api job: the API contract is fresh, and a breaking change is declared
ci-web: web-static web-runtime ## Run CI's web job: the SDK's types, typechecks, the build, the tests
ci-skills: skills-runtime ## Run CI's skills job: front matter, links, the hook's tests, the facade
ci-manifest: manifest-static manifest-runtime ## Run CI's manifest job: the versions agree, the cloud servers parse

check: lint-static go-static api-static web-static manifest-static ## Run every static check CI runs: lint, vet, type-checks, generated files fresh, versions agree

test: go-runtime web-runtime skills-runtime manifest-runtime ## Run every build and test CI runs: Go, SDK, frontend, skills
	$(if $(REAL_SIDECARS),,@echo "WARNING: the real-sidecar tests were skipped: ANFRA_NODE_BIN and ANFRA_CANAL_BIN are not both set. make test-sidecars runs them.")

lint-static:
	$(TOOL) golangci-lint run ./...

go-static:
	go mod tidy --diff
	scripts/sqlc.sh diff
	go vet ./...

go-runtime:
	$(if $(REAL_SIDECARS),scripts/sidecars.sh check)
	go build ./...
	go test $(GO_TEST_FLAGS) ./...

api-static:
	scripts/openapi.sh diff
	scripts/openapi.sh declared "$$(git merge-base $(CI_BASE) HEAD)"

# The SDK's type declarations, which the frontend's typecheck reads (web/sdk/dist), are the one
# thing the static half builds.
web-static:
	pnpm --filter @holistics/anfra-sdk generate >/dev/null
	@git diff --exit-code --quiet -- web/sdk/src/api/schema.d.ts \
		|| { echo "web/sdk/src/api/schema.d.ts was stale: it is regenerated now, commit it"; exit 1; }
	pnpm --filter @holistics/anfra-sdk typecheck
	pnpm build:sdk >/dev/null
	pnpm --filter anfra-appserve typecheck

# The build a release embeds, then the tests: the frontend's import the built SDK.
web-runtime:
	pnpm build:web
	pnpm test:web

skills-runtime:
	pnpm skills:check

manifest-static:
	@release="$$(sed -n 's/^version: *//p' manifest.yml)" && \
		for pkg in web/sdk web/design; do \
			version="$$(jq -r .version "$$pkg/package.json")"; \
			test "$$version" = "$$release" \
				|| { echo "$$pkg/package.json version $$version != manifest.yml version $$release: pnpm bump sets them all" >&2; exit 1; }; \
		done

manifest-runtime:
	go test -count=1 ./scripts/cloud-url
