# codegen

Generate Go models and HTTP servers from OpenAPI 3.0, 3.1 and 3.2 specs.

Status: early development. The config format and the generated API may still change.

## Goals

- Every file's location is written in the config. One file or many, one package or several.
- Consistent, deterministic names, with clashes resolved the same way on every run.
- One shape for `oneOf`/`anyOf` unions, whatever the number of variants.
- Plain Go validation code, no reflection.
- Filters, overlays, pruning and spec simplification before generation.

## Development

```sh
make help                                   # list targets
make test PKG=./scripts/covercheck          # run one package's tests
make check                                  # lint, 100% coverage gate, tidy
```
