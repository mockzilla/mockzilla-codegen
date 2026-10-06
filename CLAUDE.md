# mockzilla-codegen

Generates Go models, HTTP servers, clients and MCP tools from OpenAPI 3.0, 3.1 and 3.2 specs.
Module `github.com/mockzilla/mockzilla-codegen`. Built on github.com/pb33f/libopenapi, which only
`internal/provider/libopenapi` may import.

## Commands

- `make help` lists every target.
- One test while developing: `make test PKG=./internal/naming RUN=TestIdent`.
- Before calling a change done: `make check` (lint, 100% coverage gate, tidy, examples).
- Golden examples: `make examples` regenerates `examples/`, `make examples-check` compares and
  builds them. `make generate` regenerates them and `config.schema.json`.
- One integration spec: `make test-integration SPEC=3.0/misc/<spec>.yml`. Never run the full
  integration or parse sweep unless asked; they cover 2,000+ specs. Each spec runs as models
  only and once per server framework in `FRAMEWORKS` (default `chi`, `all` for every one), whose
  router is also built in a test; `CLIENT=1` adds the client variant, whose client is built the
  same way, `MCP=1` the MCP variant, whose tools are built over the client, and `SPLIT=1` the
  variant that puts every part in a package of its own where Go allows, which is built. Server
  variants validate requests and responses, and the models variant generates `ValidateResponse`.
- The integration run skips jobs that passed with the same spec, variant and tool build
  (`.integration-cache.json`); `make test-integration-clear` runs all. Both sweeps leave out a spec
  whose name starts with `-` and `stash` folders.
- `make test-integration-ci` runs what CI runs: `.github/ci-specs.txt` on every variant with chi,
  `.github/ci-router-specs.txt` on every framework.
- Coverage gate exclusions live in `.covignore`.

## Rules

- Public repo: never name private repositories, internal services, accounts or deployment details in
  code, comments, docs, examples, commit messages or PR text.
- Run the `code-style` skill before declaring work done and before opening a PR.
- Every `.go` file starts with the MIT license header (copyright line, SPDX tag, and the MIT
  condition that the notice stays in every copy), then a blank line. Copy it from any existing file;
  the exact text is the goheader template in `.golangci.yaml`, which enforces it.
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
| `cmd/mockzilla-codegen` | CLI, installed as `mockzilla-codegen` |
| `internal/...` | provider, spec IR, transforms, naming, Go model, rendering, layout |
| `examples/` | separate module: golden examples and tests of generated code |
| `test/` | parse sweep, integration test, benchmarks (build tags) |
| `scripts/covercheck` | per-package coverage gate |
