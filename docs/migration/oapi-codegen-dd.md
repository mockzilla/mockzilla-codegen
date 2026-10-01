# Migrating from oapi-codegen-dd

For projects on `github.com/doordash-oss/oapi-codegen-dd/v3`. Its config has the same blocks as
this one under other keys, its generated server has the same shape, and it honours the same
extensions, so most of the move is renaming keys. The config pair in
[examples/migration/oapi-codegen-dd](../../examples/migration/oapi-codegen-dd) is the one this
guide walks through. A project still on v2 of the fork follows the
[oapi-codegen guide](oapi-codegen.md) instead.

## The command

| oapi-codegen-dd | mockzilla-codegen |
|---|---|
| `oapi-codegen --config cfg.yaml api.yaml` | `mockzilla-codegen generate -c codegen.yaml`, the spec path from `spec.path` or as the last argument |
| `go run github.com/doordash-oss/oapi-codegen-dd/v3/cmd/oapi-codegen ...` | `go tool mockzilla-codegen generate ...` ([getting started](../getting-started.md)) |
| `oapi-codegen --version` | `mockzilla-codegen version` |

## Config

| oapi-codegen-dd | mockzilla-codegen |
|---|---|
| `package` | `package` |
| `copyright-header` | `header` |
| `skip-prune` | `spec.prune: false` |
| `overlay.sources` | `spec.overlays` |
| `base-path` | none; a relative `$ref` is resolved against the spec file |
| `output.use-single-file: true`, `output.directory`, `output.filename` | `output.file`, the path of that file |
| `output.use-single-file: false` | `output.files`, each part to the file you name ([output files](../../README.md#output-files)) |
| `output.skip-fmt` | `output.format: false` |
| `generate.models` | always on; models for another package go there with `output.files` |
| `generate.client` | `client:` |
| `generate.client-with-response` | `client.with-response` |
| `generate.client-streaming` | `client.streaming` |
| `generate.omit-description` | `models.descriptions: false` |
| `generate.default-int-type` | `models.int-type` |
| `generate.always-prefix-enum-values` | `naming.enum-prefix` |
| `generate.additional-tags` | `models.extra-tags` |
| `generate.validation.skip`, `response` | `models.validation.skip`, `response` |
| `generate.validation.simple` | none; validation is plain generated code ([validation](../validation.md)) |
| `generate.handler.kind` | `server.framework`, same names |
| `generate.handler.name` | `server.name` |
| `generate.handler.multipart-max-memory: 64` | `server.multipart-max-memory: 64MB`, with a unit |
| `generate.handler.validation.request`, `response` | `server.validation.request`, `response` |
| `generate.handler.service: {}` | `server.scaffold.service: ./service.go` |
| `generate.handler.middleware: {}` | `server.scaffold.middleware: ./middleware.go` |
| `generate.handler.output.directory`, `package` | the folder of the scaffold paths; its package from `output.packages` |
| `generate.handler.output.overwrite` | `server.scaffold.overwrite` |
| `generate.handler.server.directory` | `server.scaffold.main: ./server/main.go` |
| `generate.handler.server.port`, `timeout: 30` | `server.scaffold.port`, `timeout: 30s` |
| `generate.handler.server.handler-package` | none; the module path comes from `go.mod`, or `output.module` |
| `generate.handler.models-package`, `handler-package-alias`, `models-package-alias` | none; one run writes every folder and the imports between them |
| `generate.mcp-server` | `mcp:` |
| `generate.mcp-server.default-skip` | `mcp.default-skip` |
| `filter.include`, `filter.exclude` and their keys | `spec.filter.include`, `spec.filter.exclude`, unchanged |
| `additional-imports` | [`imports`](../plugins.md#imports); no `.` alias |
| `error-mapping` | `models.error-mapping` |
| `client.name`, `client.timeout` | `client.name`, `client.timeout` |
| `user-templates` | `templates` for the [blocks that may be replaced](../plugins.md#template-overrides), else a [plugin](../plugins.md) |
| `user-context` | `user-context` |

The keys of `error-mapping` are type names in both. An error response written inline in an
operation is `<Op>Response<Status>` here ([names](../naming.md#names-for-types-without-a-name)),
not `<Op>ErrorResponse`.

## Extensions

The list is the same, `x-mcp` included ([extensions](../extensions.md)). `x-go-type-name` on a
component declares the type under the new name only, without an alias under the component name.

## Generated code

### Server

The service interface, `<Op>ServiceRequestOptions`, `<Op>ResponseData` and the
`New<Op>ResponseData` constructors keep their names and shapes ([server](../server.md)), so a
service implementation moves as it is. What differs:

- The scaffolds live where `server.scaffold` says, not in a folder next to the output, and
  `main` imports the others by module path.
- The router is `NewRouter(svc, opts...)` with `WithMiddleware`, `WithErrorHandler` and
  `WithRouter` ([router](../server.md#router)); check the option names against yours.
- An error type the service returns is answered with the status of the first response that
  carries it ([HTTP adapter](../server.md#http-adapter)).

### Client

| oapi-codegen-dd | mockzilla-codegen |
|---|---|
| `NewDefaultClient(baseURL, opts...)`, `NewClient(runtime.APIClient)` | `NewClient(baseURL, opts...)`; `WithHTTPClient` takes anything with `Do` ([client](../client.md#client)) |
| `<Op>(ctx, options, reqEditors...)` returning `*<Op>Response` | `<Op>(ctx, opts)` returning the success body, an error otherwise ([methods](../client.md#methods)) |
| `reqEditors` per call | `WithRequestEditor` on the client |
| `<Op>WithResponse` | the same, with `HTTPResponse`, `Body`, `JSON<status>` and `Headers<status>` ([envelopes](../client.md#envelopes)) |
| `<Op>Stream` over `runtime.Stream[T]` | the same ([streaming](../client.md#streaming)) |

### MCP

The tools are generated for the official Go SDK, `github.com/modelcontextprotocol/go-sdk`,
instead of `github.com/mark3labs/mcp-go` ([MCP](../mcp.md)):

```go
// oapi-codegen-dd
s := server.NewMCPServer("petstore", "1.0.0", server.WithToolCapabilities(true))
api.NewMCPTools(s, api.WithClient(client))
server.ServeStdio(s)

// mockzilla-codegen
s := mcp.NewServer(&mcp.Implementation{Name: "petstore", Version: "1.0.0"}, nil)
api.NewMCPTools(client).Register(s)
s.Run(ctx, &mcp.StdioTransport{})
```

### Types

- A `oneOf` or `anyOf` is a struct with one field per variant, whatever their number
  ([unions](../types.md#unions)), instead of `runtime.Either[A, B]` for two variants and a raw
  message with accessors for more.
- A `number` without a format is `float64` ([type mapping](../types.md#type-mapping)).
- The [pointer rules](../types.md#pointers) and [naming rules](../naming.md) are written down;
  compare a generated file against the old one before touching call sites.
