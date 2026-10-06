# Migrating from another generator

Guides for Go projects that generate code from an OpenAPI spec with another tool today. Each guide
says what is different, shows one handler and one client call before and after, lists the steps,
and ends with every command flag, config key and extension of that tool mapped to this one.

## How big the move is

| From | Config | Server code | Client code |
|---|---|---|---|
| [oapi-codegen-dd](migration/oapi-codegen-dd.md) v3 | rename keys | a constructor per status, other param struct names | `*Error` in `errors.As`, other envelope names |
| [oapi-codegen](migration/oapi-codegen.md), strict server | rename keys | new signatures, same flow | the call returns the body; other statuses are Go errors |
| [oapi-codegen](migration/oapi-codegen.md), plain server | rename keys | rewritten: no `http.ResponseWriter` | the same as above |
| [ogen](migration/ogen.md) | flags and keys to `codegen.yaml` | response data instead of sum types; `Opt` types are pointers | no `switch` on the response type |

The fork goapi-gen has a section at the end of the oapi-codegen guide.

The spec stays as it is. Every version from 3.0 to 3.2 is read, and the `x-go-*` extensions it
already carries keep working ([extensions](extensions.md)).

## The examples

Each guide walks through a folder of `examples/migration/` that holds the same petstore spec, the
old tool's config, its translation to `codegen.yaml`, the generated code, and the service and
client calls after the move. They are built and tested with the other examples, so the code the
guides quote compiles.

| From | Example |
|---|---|
| oapi-codegen | [examples/migration/oapi-codegen](../examples/migration/oapi-codegen) |
| oapi-codegen-dd | [examples/migration/oapi-codegen-dd](../examples/migration/oapi-codegen-dd) |
| ogen | [examples/migration/ogen](../examples/migration/ogen) |
