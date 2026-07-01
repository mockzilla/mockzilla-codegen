---
name: code-style
description: "Go code-style gate for the lambdas repo. MUST run before declaring any feature or task complete, and before every `gh pr create` (or any pull-request creation). Reviews the changed `.go` files against the project's Go style rules (comments, tests, naming, errors, structs, argument count, blank lines, file structure, sugar wrappers, logging, package names) and blocks PR creation until violations are fixed or explicitly overridden. Scope is Go source only. Also invoke on request: 'style gate', 'check go style', 'lint go', 'code-style review'."
---

# Code Style

Go style gate for the repo. It runs at two checkpoints:

1. **Before you declare a feature or task complete.**
2. **Before any `gh pr create`** (or other pull-request creation).

Scope is Go source only (`.go`); non-Go files are out of scope.

Two layers, in precedence order:

1. **Uber Go Style Guide** - the baseline ruleset (digest under "Baseline" below).
2. **Project rules** (1-11) - override or supplement that baseline for this repo.

On conflict the project rule wins. Cite the winning source when a reviewer
pushes back.

## Running the gate

### Step 1 - Enumerate the diff

```bash
git diff --name-only origin/main...HEAD -- '*.go'
```

For the feature-complete checkpoint (no PR yet, work may be uncommitted) also
include the working tree:

```bash
git diff --name-only -- '*.go'; git diff --name-only --cached -- '*.go'
```

If no `.go` files changed, the gate doesn't apply. Exit and let the normal
flow proceed.

### Step 2 - Self-review against the rules

For every changed `.go` file: read it in full, then walk the Uber baseline and
the project rules (1-11). The project rule wins on conflict. Record each
violation as one line, no summary:

```
<file>:<line> - rule <N> - <one-line fix>
```

### Step 3 - Report

Output the violation list verbatim. Two outcomes:

- **Zero violations**: proceed (complete the feature, or run `gh pr create`).
- **Any violations**: stop. Ask the user to fix, accept (override), or go
  case-by-case. Do not complete the task or create the PR until the answer is in.

### Step 4 - PR footer (PR-create checkpoint only)

When the gate passes for a PR, append to the PR body:

```
---
Code style gate: passed (N files reviewed)
```

or, on override:

```
---
Code style gate: overridden (<one-line reason>)
```

## Baseline: Uber Go Style Guide

Canonical source: https://github.com/uber-go/guide/blob/master/style.md. The
digest below is inlined so the gate runs without opening the link; it restates
the rules that produce the most review findings. For anything not listed,
defer to the canonical guide. The project rules in the next section overwrite
or concretize these.

### Errors

- Sentinel errors are package-level `var Err... = errors.New(...)`: lowercase message, no trailing punctuation.
- Wrap with context, not `errors.New(err.Error())`. Use `fmt.Errorf("doing X: %w", err)` so callers can `errors.Is`/`errors.As`.
- One mapping point for translating downstream errors into your own; don't `errors.Is` deep in business logic.

### Naming

- Package names: lowercase, single word, no underscores.
- Receiver names: 1-3 chars, consistent across all methods on the type. Never `self`/`this`/`me`.
- Initialisms (URL, ID, HTTP) keep consistent casing: `userID`, `parseHTTPRequest`. Never `Id` mid-name or `Url`.

### Function and method shape

- Functional options for constructors that may grow: `New(name, opts...)` over wide parameter lists.
- Return early; indent the error path. `if err != nil { return ... }` keeps the happy path at column 0.
- No unnecessary `else` after a `return`/`break`/`continue`.
- Naked returns only in very short funcs (<= ~5 lines).
- Named results for documentation only, not as implicit assignments.

### Variables and scope

- Reduce scope: declare close to use. `if x, err := f(); err != nil { ... }` over hoisting `x` to the function top.
- `nil` is a valid slice: `var s []string` then `append`. Don't init to `[]string{}` unless an API requires non-nil.
- No `init()` for anything that can be a constructor. `init()` is fine for `flag.Var` registration, not business logic.

### Structs

- Initialize with field names: `Foo{Amount: 100}`, never positional `Foo{100, "USD"}`.
- Zero-value mutexes are valid: don't write `sync.Mutex{}`, use the field's zero value.
- No embedding in public structs unless the embedded type is also public and the relationship is intentional.

### Performance (review hot-spots)

- `strconv` over `fmt` for primitive-to-string: `strconv.Itoa(n)`, not `fmt.Sprintf("%d", n)`.
- Specify slice/map capacity when known: `make([]Item, 0, len(input))`.
- Avoid string<->byte conversion in hot paths (the dispatcher); the conversion allocates.

### Patterns

- Test tables are the default test shape: one table per function under test.
- `t.Parallel()` at the top of each `t.Run` unless the test mutates shared state.
- Verify interface compliance with a blank assignment at package scope: `var _ Store = (*store)(nil)`.

### What NOT to flag

- Positions Uber takes that conflict with a project rule below: the project rule wins.
- Formatting: gofmt is the source of truth.
- Doc-comment presence or paraphrasing on internal-only code.

## Project rules (overwrite or concretize the baseline)

### 1. Comments default to none

Write none by default. Add one short line only when it tells the reader
something the code does not: a hidden constraint, a subtle invariant, a
non-obvious workaround.

Forbidden:
- Inline body comments that restate the code (`// increment counter` above `counter++`).
- Comments referencing the current task, fix, ticket, or caller (`// added for DJANGO-14`, `// used by the enqueuer`). These rot.
- Multi-paragraph docstrings on internal helpers.
- Narration or divider comments in tests (`// setup`, `// act`, `// assert`, `// --- mocks ---`).

Allowed:
- One short line for a non-obvious why.
- Short godoc (a sentence or two) on an exported identifier: starts with the identifier name, sits directly above the declaration with no blank line, reads as a sentence.

Gate rule: never flag "missing doc comment" or "godoc restates the name" as a
violation. Doc comments are permitted, not required. The gate looks only for
the forbidden shapes above.

**Why:** short godoc is a cheap norm (the repo's `errors.go` files ship with
it). The real hazards are rotting ticket refs, internal-helper essays, and
inline restatements.

### 2. Tests

- **Sentence-case names that state the scenario.** `"Enqueue fails when the target account is unknown"`, not `"EnqFailAcct"`. The name alone says what's tested.
- **Prefer table tests.** Cases that share shape go in a `tests []struct{...}` slice with `t.Run(tc.name, ...)`. Standalone subtests only when setup is genuinely incompatible.
- **`t.Helper()` in any helper that can fail.** A helper calling `t.Fatal`/`t.Error` or `require`/`assert` starts with `t.Helper()`, else the failure points at the helper, not the caller.
- **Prefer complete assertions.** One `assert.Equal` on the whole struct/map over field-by-field; new fields then surface as deltas instead of going uncovered.
- **Test file mirrors source.** `foo.go` is tested in `foo_test.go`. No orphan `helpers_test.go` with no source pair, no `foo_feature_test.go` split. When `foo.go` grows too big, split the source first; the test files follow.

**Why:** names and file names are the first thing a reviewer sees on failure; they must locate the broken behavior without reading the body. Table tests + complete assertions shrink both authoring and review surface.

### 3. Variable naming

- **Initialisms keep canonical case.** All-caps when exported or in the middle/end of an unexported name (`SimID`, `OrgID`, `parsedURL`, `marshalToJSON`, `HTTPClient`); all-lowercase only at the start of an unexported name (`httpClient`, `simID`). Repo initialisms: ID, URL, HTTP, JSON, API, SQS, DDB, KVS, ARN, AWS, S3, PK, SK, TTL, RPS. Never title-case: `HttpClient`, `OrgId`, `Url`, `Sqs` are all wrong.
- **New bool fields and vars take an `Is` prefix.** `IsDedicated`, `IsBuildCounted`, not bare `Dedicated`. Existing fields stay as they are.
- **Don't shadow built-ins.** Not `min`, `max`, `cap`, `len`, `new`, `copy`, `delete`, `error`, `string`, etc. Use a domain name: `minAmount`/`maxAmount`, `capacity`, `count`.
- **Default to unexported.** Lowercase unless a caller in another package actually needs it. Exporting "just in case" enlarges the package's public surface.

**Why:** initialisms rendered as pseudo-words (`Orgid`, `Httpclient`) break the scan that says "this is an acronym". Shadowing a built-in silently disables it in scope. Over-exporting turns future renames into cross-package migrations.

### 4. Errors

- **Sentinel errors always.** Every static error is a package-level `var ErrX = errors.New("...")`. Never build the same static error inline at the throw site; callers can't `errors.Is` it and the message drifts across copies.
- **Errors live in `errors.go`.** One file per package, at the package root. New error? Add it there, not next to the function that raises it.
- **Wrap with `%w`.** `fmt.Errorf("get org: %w", err)` so callers can `errors.Is`/`errors.As`. Never `errors.New(err.Error())`.
- **Log an error exactly once,** at the layer that swallows it or where it leaves the system. Wrapping-and-returning is not the place to log.
- **Never compare errors by string.** No `err.Error() == "..."`, no `strings.Contains(err.Error(), "...")`. Use `errors.Is`/`errors.As`. String compare breaks the moment the error is wrapped with `%w`.

**Why:** identity matching survives wrapping; one `errors.go` makes the full error surface auditable in one place; string compare is locale- and format-sensitive in a way `errors.Is` is not.

### 5. Structs

- **Exported fields before unexported, separated by a blank line.** Public contract on top, internal state below.
- **Receiver methods stay next to the struct.** Declare them immediately below `type Foo struct {...}`. Constructors (`NewFoo(...) *Foo`) are allowed between the struct and its methods.
- **No other code between receiver methods of the same struct.** Once `Foo`'s method block starts, every following declaration is another method (or constructor) of `Foo` until the block ends. No package vars, consts, helpers, or other types interleaved.
- **Exported methods before unexported.**

**Why:** a reviewer should scroll to a struct and see its full surface (fields + behavior) without bouncing around the file.

### 6. Argument count

Receiver does **not** count; `ctx context.Context` **does**. Cap is **4**.

- **<= 3 args:** fine.
- **4 args:** allowed, but consider whether a parameters struct reads better.
- **5 or more:** not allowed. Refactor to a parameters struct before the gate passes.

```go
// Bad - 6 positional args (ctx + 5); every caller breaks when one is added or reordered
func Enqueue(ctx context.Context, simPK, ref, account, region, action string) error

// Good - one struct; new fields slot in without touching call sites
type EnqueueRequest struct {
    SimPK   string
    Ref     string
    Account string
    Region  string
    Action  string
}

func Enqueue(ctx context.Context, req EnqueueRequest) error
```

**Why:** long positional lists are silent migration hazards, and two same-typed neighbors get swapped at a call site without the compiler noticing. A struct names every field and stays backwards-compatible.

### 7. Logical blank lines

Group statements into paragraphs separated by a single blank line; each
paragraph does one thing (validate, fetch, transform, return). Don't run
unrelated steps together as a wall of code, and don't glue independent `if`
blocks back-to-back.

```go
// Good - one paragraph per logical step
func (s *Service) Enqueue(ctx context.Context, req EnqueueRequest) error {
    if req.SimPK == "" {
        return ErrSimPKRequired
    }
    if req.Action == "" {
        return ErrActionRequired
    }

    org, err := s.orgs.Get(ctx, req.OrgID)
    if err != nil {
        return fmt.Errorf("get org: %w", err)
    }

    msg := buildMessage(req, org)

    if err := s.sqs.Send(ctx, msg); err != nil {
        return fmt.Errorf("send: %w", err)
    }

    return nil
}
```

The bad shape is the same body with every step glued together and the sibling
`if` checks touching, so the phases can't be seen at a glance.

**Why:** blank lines are punctuation. A reviewer should see a function's phases (validate / fetch / build / call) without parsing a continuous block.

### 8. File structure

- **One file per concern.** Each `.go` file holds one struct or one cohesive group of helpers; split by concern (`routing.go`, `headers.go`, `timings.go`) rather than dumping everything into `handler.go`. Combine only when a concern is genuinely small (a type and one or two short helpers); once it grows past that, give it its own file.
- **`var`, `const`, `type` declarations live at the top of the file,** below imports (and below any type-alias `const` block like `FeatureCodegen`). Never scatter them through function bodies and never dump them at the bottom. A reader after the package's constants should not grep for them.
- **Exported functions before unexported.** Public surface on top, private helpers below.
- **Caller before callee within the unexported block.** `foo` calls `bar`: declare `foo` first, `bar` below. Read top-to-bottom in rough call order.
- **No structural-divider comments anywhere.** No `// --- types ---`, `// === handlers ===`, `// region`, ASCII-art bars, on any kind of declaration. If a file needs dividers, split it into multiple files.

**Why:** a new reader should see what the package exposes, then how it works, then the plumbing, in that order. Dividers signal a file doing too many things; the fix is to split, not to label.

### 9. No sugar wrappers for trivial operations

Don't wrap a one-line operation (map lookup, field access, slice index, simple
conversion) in a named helper just to give it a name. Leave it inline.

```go
// Bad - a helper that only does a map lookup
func accountFor(targets map[string]string, alias string) string {
    return targets[alias]
}

// Good - inline, no indirection to chase
account := targets[alias]
```

Exception: wrap when the helper adds behavior the inline expression can't:
validation, default handling, an error return, type translation, telemetry.

```go
// Good - validates and returns an error, not just a lookup
func accountFor(targets map[string]string, alias string) (string, error) {
    acct, ok := targets[alias]
    if !ok {
        return "", fmt.Errorf("unknown target %q", alias)
    }
    return acct, nil
}
```

**Why:** each named helper is a layer the reader has to mentally inline. Save abstraction for code that adds behavior, not aliases.

### 10. Logging

This repo logs via `slog`, and `slog.Error` reaches Sentry through the slog
handler (`internal/sentryutil`).

- **Prefix the message with the lambda/package name and a colon.** `slog.Info("enqueuer: enqueued provision", ...)`. Matches every handler in the repo.
- **Log attribute keys are `snake_case`.** `org_id`, `sim_pk`, `deployment_id`, `account`, `ref`, `code`. Carry the error under the `err` key: `"err", err`. Don't introduce a second spelling for an existing concept; scan the file and align.
- **No level prefixes in the message.** The call already encodes the level. Forbidden: `slog.Error("enqueuer: ERROR: ...")`, `"[DEBUG] ..."`, bracketed scope tags.
- **Log an error once.** `slog.Error` already reaches Sentry; do not also call Sentry directly or re-log the same error at another layer.
- **Don't re-log context already attached** to the row/operation (the `deployment_id`/`sim_pk` logged upstream). Bind only the attributes this step adds.

**Why:** `snake_case` keys keep queries consistent with the rest of the repo; level prefixes break level-filtered dashboards; double-logging triples incident noise when `slog.Error`-to-Sentry already covers reporting.

### 11. Package naming

- **No underscores in new package names.** Lowercase, single word, short, concrete: `enqueuer`, `simchange`, `kvswriter`, `regionreconcile`. Forbidden: `sim_change`, `kvs_writer`, `region_reconcile`.
- **Alias underscored or multi-word upstream packages to `camelCase` at the import site** rather than referencing the raw name in the body.

**Why:** Go canon (Effective Go, the Uber guide) requires lowercase single-word package names; underscored names read awkwardly at every call site.

## Out of scope

- **Non-Go files.** The gate ignores them.
- **Test correctness and functional review.** This gate is style only; a bug in correct-style code still passes. Behavior is a separate review.

## Updating this skill

When a project style preference changes, update the matching rule here. There
are no reference files: everything lives in this one file.
