.DEFAULT_GOAL := help
SHELL := /bin/bash

PKG ?= ./...
RUN ?=
MIN_COVERAGE ?= 100
# golangci-lint must be built with a Go at least as new as the go directive in go.mod; bump the two
# together.
GOLANGCI_VERSION ?= v2.14.0
GOLANGCI ?= bin/golangci-lint
GOLANGCI_ARCHIVE := golangci-lint-$(GOLANGCI_VERSION:v%=%)-$(shell go env GOOS)-$(shell go env GOARCH)

.PHONY: help
help: ## List targets
	@awk 'BEGIN {FS = ":.*## "} /^[a-zA-Z_-]+:.*## / {printf "  %-24s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

.PHONY: build
build: ## Build every package
	go build ./...

.PHONY: install
install: ## Install the mockzilla-codegen CLI
	go install ./cmd/mockzilla-codegen

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

.PHONY: lint-tools
lint-tools: ## Install golangci-lint GOLANGCI_VERSION into bin/ unless that version is there
	@if ! $(GOLANGCI) --version 2>/dev/null | grep -q 'version $(GOLANGCI_VERSION:v%=%) '; then \
		mkdir -p bin; \
		curl -sSfL https://github.com/golangci/golangci-lint/releases/download/$(GOLANGCI_VERSION)/$(GOLANGCI_ARCHIVE).tar.gz \
			| tar -xz -C bin --strip-components=1 $(GOLANGCI_ARCHIVE)/golangci-lint; \
	fi

.PHONY: lint
lint: lint-tools ## Run golangci-lint
	$(GOLANGCI) run ./...

.PHONY: lint-fix
lint-fix: lint-tools ## Run golangci-lint and apply fixes
	$(GOLANGCI) run --fix ./...

.PHONY: fmt
fmt: lint-tools ## Format code (gofumpt, goimports)
	$(GOLANGCI) fmt ./...

.PHONY: tidy
tidy: ## Tidy go.mod and go.sum, also of the examples module
	go mod tidy
	cd examples && go mod tidy

.PHONY: tidy-check
tidy-check: ## Fail when go.mod or go.sum are not tidy, also of the examples module
	$(MAKE) --no-print-directory tidy
	git diff --exit-code -- go.mod go.sum examples/go.mod examples/go.sum

.PHONY: schema
schema: ## Regenerate config.schema.json
	UPDATE=1 go test -count=1 -run TestSchemaUpToDate ./pkg/config

.PHONY: generate
generate: ## Run go generate
	go generate ./...

.PHONY: examples
examples: ## Regenerate the golden examples
	UPDATE=1 go test -count=1 -run '^TestExamples$$' ./pkg/codegen

.PHONY: examples-check
examples-check: ## Fail when the golden examples are stale, do not build or fail their tests
	go test -count=1 -run '^TestExamples' ./pkg/codegen
	cd examples && go build ./... && go vet ./... && GIN_MODE=release go test -count=1 ./...

.PHONY: test-parse
test-parse: ## Parse every spec in testdata/specs; SPEC=, SPECS= narrow it
	SPEC='$(SPEC)' SPECS='$(SPECS)' go test -tags parse -count=1 -timeout 60m -v ./test/parse

.PHONY: test-integration
test-integration: ## Generate, build and test every spec in testdata/specs; SPEC=, SPECS= narrow it, FRAMEWORKS= picks the server variants (chi, std-http, echo, or any of docs/server.md's routers), CLIENT=1 adds the client variant, MCP=1 the MCP variant
	SPEC='$(SPEC)' SPECS='$(SPECS)' $(if $(FRAMEWORKS),FRAMEWORKS='$(FRAMEWORKS)') $(if $(CLIENT),CLIENT='$(CLIENT)') $(if $(MCP),MCP='$(MCP)') go test -tags integration -count=1 -timeout 120m -v ./test/integration

.PHONY: test-integration-clear
test-integration-clear: ## Integration run with the result cache cleared
	@CLEAR_CACHE=1 $(MAKE) --no-print-directory test-integration

.PHONY: bench
bench: ## Benchmarks on large specs in testdata/specs
	go test -run '^$$' -bench . -benchmem -benchtime 3x ./test/bench

.PHONY: runtime-deps
runtime-deps: ## Fail when ./pkg/runtime imports anything outside the standard library
	@bad=$$(go list -deps ./pkg/runtime | grep -vx 'github.com/mockzilla/mockzilla-codegen/pkg/runtime' | awk -F/ '$$1 ~ /\./'); \
	if [ -n "$$bad" ]; then echo "pkg/runtime must import the standard library only, found:"; echo "$$bad"; exit 1; fi

.PHONY: check
check: lint cover-check runtime-deps examples-check tidy-check ## Run every check

.PHONY: clean
clean: ## Remove build and test output
	rm -rf .sandbox bin coverage.out coverage.html .integration-cache.json
