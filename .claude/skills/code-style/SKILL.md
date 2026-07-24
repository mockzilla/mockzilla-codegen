---
name: code-style
description: "Go code-style gate for the codegen repo. MUST run before declaring any feature, phase or task complete, and before every `gh pr create` (or any pull-request creation). Reviews the changed `.go` files and templates against the project's Go style rules (comments, tests, naming, errors, structs, argument count, blank lines, file structure, sugar wrappers, no printing, package names, determinism, logic-free templates, package state, public-repo hygiene) and blocks completion until violations are fixed or explicitly overridden. Also invoke on request: 'style gate', 'check go style', 'lint go', 'code-style review'."
---

# Code Style

Go style gate for this repo, a library that generates Go code from OpenAPI specs. It runs at two
checkpoints:

1. **Before you declare a feature, phase or task complete.**
2. **Before any `gh pr create`** (or other pull-request creation).

Scope: Go source (`.go`) and generator templates (`.tmpl`). Other files are out of scope, except
that rule 16 (public repo) applies to every file and every commit message.

Two layers, in precedence order:

1. **Uber Go Style Guide** - the baseline (digest under "Baseline").
2. **Project rules** (1-16) - override or supplement the baseline.

On conflict the project rule wins.

## Running the gate

### Step 1 - Enumerate the diff

```bash
git diff --name-only origin/main...HEAD -- '*.go' '*.tmpl'
```

For the feature-complete checkpoint (work may be uncommitted) also include the working tree:

```bash
git diff --name-only -- '*.go' '*.tmpl'; git diff --name-only --cached -- '*.go' '*.tmpl'
git ls-files --others --exclude-standard -- '*.go' '*.tmpl'
```

Generated files under `examples/` are output, not source: check them only for rule 13 (they must
come from an `UPDATE=1` run) and rule 16.

If nothing changed, the gate doesn't apply.

### Step 2 - Self-review against the rules

Run `make lint` first; fix what it reports. Then, for every changed file: read it in full, walk the
Uber baseline and the project rules. Record each violation as one line:

```
<file>:<line> - rule <N> - <one-line fix>
```

### Step 3 - Report

- **Zero violations**: proceed.
- **Any violations**: stop. Ask the user to fix, accept (override), or go case-by-case. Do not
  complete the task or create the PR until the answer is in.

### Step 4 - PR footer (PR-create checkpoint only)

```
---
Code style gate: passed (N files reviewed)
```

or `Code style gate: overridden (<one-line reason>)`.

## Baseline: Uber Go Style Guide

Canonical source: https://github.com/uber-go/guide/blob/master/style.md. For anything not listed,
defer to it.

### Errors

- Sentinel errors are package-level `var Err... = errors.New(...)`: lowercase message, no trailing punctuation.
- Wrap with context: `fmt.Errorf("parse spec: %w", err)`, never `errors.New(err.Error())`.
- One mapping point for translating errors from a dependency (libopenapi) into our own.

### Naming

- Package names: lowercase, single word, no underscores.
- Receiver names: 1-3 chars, consistent across methods. Never `self`/`this`/`me`.
- Initialisms keep consistent casing: `specURL`, `parseJSONBody`.

### Function and method shape

- Functional options for constructors that may grow: `New(name, opts...)`.
- Return early; indent the error path.
- No `else` after `return`/`break`/`continue`.
- Naked returns only in very short funcs.

### Variables and scope

- Declare close to use: `if x, err := f(); err != nil { ... }`.
- `nil` is a valid slice: `var s []string` then `append`.
- No `init()` (see rule 14).

### Structs

- Initialize with field names.
- Zero-value mutexes are valid.
- No embedding in public structs unless intentional and the embedded type is public.

### Performance

- `strconv` over `fmt` for primitive-to-string.
- Specify slice/map capacity when known.
- Large specs (5-50 MB) go through every stage: no quadratic loops over schemas or operations.

### Patterns

- Table tests are the default test shape.
- `t.Parallel()` at the top of each test and `t.Run` unless it mutates shared state.
- Verify interface compliance: `var _ Provider = (*provider)(nil)`.

### What NOT to flag

- Positions the project rules override.
- Formatting: gofumpt and goimports are the source of truth.
- Missing doc comments.

## Project rules

### 1. Comments default to none

Write none by default. Add one short line only when it tells the reader something the code does
not: a hidden constraint, a subtle invariant, a workaround.

Forbidden:
- Inline comments that restate the code.
- Comments referencing the current task, phase, ticket or caller (`// added in phase 3`). They rot.
- Multi-paragraph docstrings on internal helpers.
- Narration or divider comments in tests (`// setup`, `// act`, `// --- cases ---`).

Allowed:
- One short line for a non-obvious why.
- Short godoc (a sentence or two) on an exported identifier, starting with its name.

Never flag "missing doc comment". Doc comments are permitted, not required.

### 2. Tests

- **Sentence-case names that state the scenario**: `"Clash with a component keeps the component name"`.
- **Prefer table tests.** Standalone subtests only when setup is incompatible.
- **`t.Helper()`** in any helper that can fail.
- **Complete assertions**: one `assert.Equal` on the whole value over field-by-field checks.
- **Test file mirrors source**: `foo.go` is tested in `foo_test.go`. No orphan test files.
- **Fixtures** for specs live in `testdata/` next to the test; keep each fixture minimal.
- **Coverage**: every package passes the 100% gate (`make cover-check`). Exclusions go in
  `.covignore` with a reason in the commit message, never by skipping tests.

### 3. Variable naming

- **Initialisms keep canonical case.** Repo initialisms: ID, URL, URI, HTTP, JSON, YAML, API, IR,
  MCP, SSE, UUID, OAS, EOF. Never `Url`, `Json`, `Id`.
- **New bool fields and vars take an `Is`/`Has` prefix** where it reads naturally (`IsNullable`,
  `HasBody`). Config structs mirror their YAML keys instead.
- **Don't shadow built-ins** (`min`, `max`, `len`, `new`, `copy`, `string`, ...).
- **Don't shadow anything else either.** A local variable or parameter never reuses the name of a
  type, function, var or const of its own package (`part := at.part` next to `type part struct`),
  an imported package (`spec := ...` in a file that imports `spec`), or a variable of an outer
  scope, named results included (`if err := f()` in a function returning `err error`). Rename the
  local. Lint catches imports and outer variables (govet `shadow`, gocritic `importShadow`, revive
  `import-shadowing`); names from the own package are a manual check: for every new local and
  parameter, grep the package for a declaration with the same name.
- **Default to unexported.** Export only what another package needs. Anything not in the public API
  (`pkg/codegen`, `pkg/config`, `pkg/runtime`) lives under `internal/`.

### 4. Errors

- **Sentinel errors always**, as package-level `var ErrX = errors.New("...")`.
- **Errors live in `errors.go`**, one per package.
- **Wrap with `%w`**. Never compare errors by string.
- **Spec problems are diagnostics, not errors**, unless generation cannot continue. A diagnostic
  carries a code, a JSON pointer and an origin (file:line:col).

### 5. Structs

- Exported fields before unexported, separated by a blank line.
- Receiver methods directly below the struct (constructors may sit between).
- No other code between methods of the same struct.
- Exported methods before unexported.

### 6. Argument count

Receiver does not count; `ctx` does. At most 4. Five or more: use a parameters struct.

### 7. Logical blank lines

Group statements into paragraphs (validate, fetch, transform, return) separated by one blank line.
Don't glue independent `if` blocks together.

### 8. File structure

- Every `.go` file starts with the license header, then a blank line:
  ```go
  // Copyright (c) 2026 Mockzilla
  // SPDX-License-Identifier: MIT
  // Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
  // permission notice shall be included in all copies or substantial portions of the Software.
  ```
  The year is when the file was created. goheader enforces it in `make lint`.
- One file per concern; split before a file mixes concerns.
- `var`, `const`, `type` declarations at the top of the file, below imports.
- Exported functions before unexported; caller before callee.
- No divider comments. Split the file instead.

### 9. No sugar wrappers for trivial operations

Don't wrap a one-line map lookup, field access or conversion in a named helper. Wrap only when the
helper adds behavior: validation, defaults, an error return, type translation.

### 10. No logging or printing in library code

Library packages never log and never print (`fmt.Print*`, `println`, `log.*`, `slog.*`). They return
errors and diagnostics; the caller decides what to show. Only `cmd/` and `scripts/` print. forbidigo
enforces the print half.

### 11. Package naming and location

Lowercase, single word, short, concrete: `naming`, `layout`, `oasdoc`. No underscores. Alias
multi-word upstream packages at the import site.

Public packages live under `pkg/`, private ones under `internal/`, binaries under `cmd/`. No `.go`
files in the repo root.

### 12. Determinism

The same spec and config must give byte-identical output on every run and machine.

- Never range over a map in a generation path without sorting its keys first.
- Never use libopenapi hashes (seeded per process), pointer addresses, `time.Now` or unseeded
  randomness for names, ordering or output.
- Sort anything that comes back from a dependency in unspecified order (e.g. circular references).

### 13. Templates are logic-free

- Templates only `range` and `if` over precomputed view-model fields and call registered funcs.
- Every decision (naming, pointer or not, which decoder, which imports) lives in Go and is unit
  tested.
- No Go types or code assembled as strings in Go code outside `internal/gocode`.
- Golden files change only through `UPDATE=1` runs, never by hand.

### 14. No mutable package state

No package-level variables that change after start-up, no `init()`. Registries and caches are
built by constructors and passed in. Package-level `var` is fine for sentinel errors, compiled
regexps and read-only tables.

### 15. One boundary per dependency

Only `internal/provider/libopenapi` imports libopenapi. Only `internal/gocode` formats Go source.
The public packages expose our own types, never a dependency's.

### 16. Public repo

This repository is public. Never name private repositories, internal services, accounts, buckets or
deployment details anywhere: code, comments, docs, examples, test fixtures, commit messages, PR
titles and bodies. Describe consumers by role ("downstream consumers").

## Updating this skill

When a project style preference changes, update the matching rule here. Everything lives in this
one file.
