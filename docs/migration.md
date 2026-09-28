# Migrating from another generator

Guides for projects that generate Go code from an OpenAPI spec with another tool today. Each one
maps that tool's config keys and extensions to this config, names what has no equivalent, and
says what changes in the generated code. A pair of configs for the same spec, the tool's and its
translation, sits next to each guide's generated output in `examples/migration/`.

| From | Guide | Configs |
|---|---|---|
| oapi-codegen, and its fork goapi-gen | [oapi-codegen](migration/oapi-codegen.md) | [examples/migration/oapi-codegen](../examples/migration/oapi-codegen) |
| oapi-codegen-dd | [oapi-codegen-dd](migration/oapi-codegen-dd.md) | [examples/migration/oapi-codegen-dd](../examples/migration/oapi-codegen-dd) |
| ogen | [ogen](migration/ogen.md) | [examples/migration/ogen](../examples/migration/ogen) |

## The steps, whatever the tool

1. Add the CLI to the module, `go get -tool github.com/mockzilla/mockzilla-codegen/cmd/mockzilla-codegen`
   ([getting started](getting-started.md)). The old tool can stay until the build is green again.
2. Write `codegen.yml` next to the spec from the guide's table. Unknown keys are errors that name
   their path, so a key that did not carry over is caught on the first run, not silently ignored.
3. Keep the spec. Every version from 3.0 to 3.2 is read, and the `x-go-*` and `x-oapi-codegen-*`
   extensions the spec already carries keep working ([extensions](extensions.md)). An extension
   the guide lists as unsupported is ignored, and can stay in the spec while both tools run.
4. Delete the old generated files and the old `go:generate` lines, then generate. Every file the
   config names is written on every run, so nothing of the old output has to be kept in sync.
5. `go build ./...` lists every call site to update. The guides describe the new shapes: a
   service interface instead of handlers with `http.ResponseWriter`, a client that returns the
   success body, error types that are Go errors.
6. Put `mockzilla-codegen generate -check` in CI, so a spec change without a new run fails there.
