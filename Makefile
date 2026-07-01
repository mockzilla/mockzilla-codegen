.DEFAULT_GOAL := help
SHELL := /bin/bash

PKG ?= ./...
RUN ?=
MIN_COVERAGE ?= 100
GOLANGCI ?= golangci-lint

.PHONY: help
help: ## List targets
	@awk 'BEGIN {FS = ":.*## "} /^[a-zA-Z_-]+:.*## / {printf "  %-24s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

.PHONY: build
build: ## Build every package
	go build ./...

.PHONY: install
install: ## Install the codegen CLI
	@echo "install: not available yet"

.PHONY: test
test: ## Run unit tests; narrow with PKG=./internal/naming RUN=TestIdent
	go test -race -shuffle=on -count=1 $(if $(RUN),-run '$(RUN)') $(PKG)

.PHONY: cover
cover: ## Write coverage.out and coverage.html
	go test -race -shuffle=on -count=1 -covermode=atomic -coverprofile=coverage.out $(PKG)
	go tool cover -html=coverage.out -o coverage.html

.PHONY: cover-check
cover-check: cover ## Fail when a package is below MIN_COVERAGE (default 100)
	go run ./scripts/covercheck -profile coverage.out -min $(MIN_COVERAGE) -ignore .covignore

.PHONY: lint
lint: ## Run golangci-lint
	$(GOLANGCI) run ./...

.PHONY: lint-fix
lint-fix: ## Run golangci-lint and apply fixes
	$(GOLANGCI) run --fix ./...

.PHONY: fmt
fmt: ## Format code (gofumpt, goimports)
	$(GOLANGCI) fmt ./...

.PHONY: tidy
tidy: ## Tidy go.mod and go.sum
	go mod tidy

.PHONY: tidy-check
tidy-check: ## Fail when go.mod or go.sum are not tidy
	go mod tidy
	git diff --exit-code -- go.mod go.sum

.PHONY: schema
schema: ## Regenerate config.schema.json
	@echo "schema: not available yet"

.PHONY: generate
generate: ## Run go generate
	go generate ./...

.PHONY: examples
examples: ## Regenerate golden examples
	@echo "examples: not available yet"

.PHONY: examples-check
examples-check: ## Fail when golden examples are stale or do not build
	@echo "examples-check: not available yet"

.PHONY: test-parse
test-parse: ## Parse every spec in testdata/specs, no build
	@echo "test-parse: not available yet"

.PHONY: test-integration
test-integration: ## Generate and build specs; SPEC=, SPECS=, FRAMEWORKS=
	@echo "test-integration: not available yet"

.PHONY: test-integration-clear
test-integration-clear: ## Integration run with the result cache cleared
	@CLEAR_CACHE=1 $(MAKE) --no-print-directory test-integration

.PHONY: bench
bench: ## Benchmarks on large specs
	@echo "bench: not available yet"

.PHONY: runtime-deps
runtime-deps: ## Fail when ./runtime imports anything outside the standard library
	@echo "runtime-deps: not available yet"

.PHONY: check
check: lint cover-check runtime-deps examples-check tidy-check ## Run every check

.PHONY: clean
clean: ## Remove build and test output
	rm -rf .sandbox bin coverage.out coverage.html .integration-cache.json
