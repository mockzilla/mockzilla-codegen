# codegen

Generates Go models and HTTP servers (later clients and MCP servers) from OpenAPI 3.0, 3.1 and 3.2
specs. Module `github.com/mockzilla/codegen`. Built on github.com/pb33f/libopenapi, which only
`internal/provider/libopenapi` may import.

## Commands

- `make help` lists every target.
- One test while developing: `make test PKG=./internal/naming RUN=TestIdent`.
- Before calling a change done: `make check` (lint, 100% coverage gate, tidy, examples).
- One integration spec: `make test-integration SPEC=3.0/misc/<spec>.yml`. Never run the full
  integration or parse sweep unless asked; they cover 2,000+ specs.
- Coverage gate exclusions live in `.covignore`.

## Rules

- Public repo: never name private repositories, internal services, accounts or deployment details in
  code, comments, docs, examples, commit messages or PR text.
- Run the `code-style` skill before declaring work done and before opening a PR.
- Every `.go` file starts with `// Copyright <year> Mockzilla` and `// SPDX-License-Identifier: MIT`,
  then a blank line. MIT license, see `LICENSE`. The goheader linter enforces it.
- Library code never logs or prints; it returns errors and diagnostics. Only `cmd/` prints.
- Output must be deterministic: sort before ranging over maps, never use libopenapi hashes for names
  or ordering.
- Templates hold no logic beyond `range`/`if` on precomputed fields.
- Golden files change only through `UPDATE=1` runs, never by hand.

## Layout

Public packages live under `pkg/`, private ones under `internal/`. No Go files in the repo root.

| Path | Role |
|---|---|
| `pkg/codegen` | public API: `Generate`, `Prepare`, `Write`, `Version` |
| `pkg/config` | config structs, loading, validation, JSON schema |
| `pkg/runtime` | helpers imported by generated code, standard library only |
| `cmd/codegen` | CLI |
| `internal/...` | provider, spec IR, transforms, naming, Go model, rendering, layout |
| `examples/` | separate module: golden examples and tests of generated code |
| `test/` | parse sweep, integration test, benchmarks (build tags) |
| `scripts/covercheck` | per-package coverage gate |
