# Migrating from another generator

Guides for Go projects that generate code from an OpenAPI spec with another tool today. Each guide
has:

- what is different
- one handler and one client call, before and after
- the steps of the move
- a closing section that maps every command flag, config key and extension of that tool to this one

## How big the move is

| From | Config | Server code | Client code |
|---|---|---|---|
| [oapi-codegen-dd](migration/oapi-codegen-dd.md) v3 | rename keys | a constructor per status, other param struct names | `*Error` in `errors.As`, other envelope names |
| [oapi-codegen](migration/oapi-codegen.md), strict server | rename keys | new signatures, same flow | the call returns the body; other statuses are Go errors |
| [oapi-codegen](migration/oapi-codegen.md), plain server | rename keys | rewritten: no `http.ResponseWriter` | the same as above |
| [ogen](migration/ogen.md) | flags and keys move to `codegen.yaml` | response data instead of sum types; `Opt` types become pointers, `runtime.Nullable` or plain values, [your choice](migration/ogen.md#optional-values) | no `switch` on the response type |

Your spec stays as it is. Every version from 3.0 to 3.2 is read, and the `x-go-*` extensions it
already carries keep working ([extensions](extensions.md)).

## The examples

Each guide walks through a folder of `examples/migration/`. A folder holds the same petstore spec,
and:

- the old tool's config
- its translation to `codegen.yaml`
- the generated code
- the service and client calls after the move

The examples are built and tested with the other examples, so the code the guides quote compiles.

| From | Example |
|---|---|
| oapi-codegen | [examples/migration/oapi-codegen](../examples/migration/oapi-codegen) |
| oapi-codegen-dd | [examples/migration/oapi-codegen-dd](../examples/migration/oapi-codegen-dd) |
| ogen | [examples/migration/ogen](../examples/migration/ogen) |
