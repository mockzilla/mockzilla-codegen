# Migrating from oapi-codegen-dd

For projects on `github.com/doordash-oss/oapi-codegen-dd/v3`. A project still on v2 of the fork
follows the [oapi-codegen guide](oapi-codegen.md) instead.

The guide walks through
[examples/migration/oapi-codegen-dd](../../examples/migration/oapi-codegen-dd): a petstore spec,
the oapi-codegen-dd config, its translation, and the service and client calls after the move,
which are built and tested.

## What is different

Most of the move is renaming keys:

- The config has the same blocks under other keys.
- The service interface keeps its methods.
- The same extensions are read.

What changes in the code:

- There is a response data constructor per status: `NewGetPetResponseData(&pet)` becomes
  `NewGetPetResponseData200(&pet)`.
- An error response is the error type, returned as the error. The adapter writes it with the
  status the spec gives it, so `WithStatus(404)` next to the error goes.
- Param structs have other names. `GetPetPath` becomes `GetPetPathParams`, the `Header` field
  becomes `Headers`, and cookie params are read into `Cookies`.
- The client returns `*Error`, where it returned `Error`. An `errors.As` needs `var e *Error`.
- `GetPetWithResponse` returns a `GetPetResponse`, where it returned a `GetPetResp`. It returns no
  error for a status the spec documents. `StatusCode()` is a method.
- `GetPetResponse` was the success body in oapi-codegen-dd. It is the envelope now.
- `NewDefaultPetClient(baseURL)` becomes `NewPetClient(baseURL)`. There is no `runtime.APIClient`.
- The MCP tools are built on the official Go SDK, and named in snake case.
- A union is a struct with one field per variant, whatever their number. There is no
  `runtime.Either`.
- `Validate()` is plain generated code. The `validate` struct tags are gone.

## Before and after

### The handler

The handler of `GET /pets/{id}` in oapi-codegen-dd:

```go
func (s *Service) GetPet(ctx context.Context, opts *GetPetServiceRequestOptions) (*GetPetResponseData, error) {
	p, ok := s.pets.Get(opts.PathParams.ID)
	if !ok {
		return NewGetPetResponseData(nil).WithStatus(http.StatusNotFound), NewError("no such pet")
	}
	return NewGetPetResponseData(&p), nil
}
```

The same handler after the move, from
[service.go](../../examples/migration/oapi-codegen-dd/service.go):

```go
func (s *Service) GetPet(_ context.Context, opts *GetPetServiceRequestOptions) (*GetPetResponseData, error) {
	p, ok := s.pets.Get(opts.PathParams.ID)
	if !ok {
		return nil, &Error{Code: http.StatusNotFound, Message: "no such pet"}
	}
	return NewGetPetResponseData200(&p), nil
}
```

`return NewGetPetResponseData404(&Error{...}), nil` gives the same response. `NewRouter(svc)`
mounts the service in both.

### The client call

The client call in oapi-codegen-dd:

```go
c, err := NewDefaultPetClient("http://localhost:8080")
if err != nil {
	return err
}
pet, err := c.GetPet(ctx, &GetPetRequestOptions{PathParams: &GetPetPath{ID: 1}})
var notFound Error
if errors.As(err, &notFound) {
	return fmt.Errorf("no pet: %s", notFound.Message)
}
if err != nil {
	return err
}
```

The same call after the move, as
[service_test.go](../../examples/migration/oapi-codegen-dd/service_test.go) makes it:

```go
c, err := NewPetClient("http://localhost:8080")
if err != nil {
	return err
}
pet, err := c.GetPet(ctx, &GetPetRequestOptions{PathParams: &GetPetPathParams{ID: 1}})
var notFound *Error
if errors.As(err, &notFound) {
	return fmt.Errorf("no pet: %s", notFound.Message)
}
if err != nil {
	return err
}
```

### The MCP server

The MCP server, before and after:

```go
s := server.NewMCPServer("petstore", "1.0.0", server.WithToolCapabilities(true))
api.NewMCPTools(s, api.WithClient(client))
server.ServeStdio(s)
```

```go
s := mcp.NewServer(&mcp.Implementation{Name: "petstore", Version: "1.0.0"}, nil)
api.NewMCPTools(client).Register(s)
s.Run(ctx, &mcp.StdioTransport{})
```

The tools are `list_pets` and `get_pet` now, where they were `ListPets` and `GetPet`.

- Prompts and host settings that name the old tool break. `x-mcp: {name: ListPets}` on the
  operation keeps the old name ([MCP](../mcp.md#x-mcp)).
- A tool without a summary is described by its method and path, `GET /pets`. It used to be
  described by its name.

## Steps

1. Add the CLI to the module: `go get -tool github.com/mockzilla/mockzilla-codegen/cmd/mockzilla-codegen`.
   oapi-codegen-dd can stay until the build is green again.
2. Write `codegen.yaml` next to the spec, from the [config table](#config). An unknown key is an
   error that names its path, so a key that did not carry over shows on the first run.
3. Keep the spec. Rename two extensions: `x-oapi-codegen-extra-tags` is `x-go-extra-tags`, and
   `x-oapi-codegen-only-honour-go-name` is `x-go-name-exact`. The old names are ignored, so keep
   both while both tools run.
4. Replace the `go:generate` line with `//go:generate go tool mockzilla-codegen generate -c codegen.yaml`,
   and delete the old generated file. A service written from the old scaffold stays: it is your
   code.
5. Generate and run `go build ./...`. Each error is a call site to move: constructors, param
   structs, `*Error`, envelopes.
6. Add `mockzilla-codegen generate -check` to CI, so a spec change without a new run fails there.

## Reference

### Command

| oapi-codegen-dd | mockzilla-codegen |
|---|---|
| `oapi-codegen -config cfg.yaml api.yaml` | `mockzilla-codegen generate -c codegen.yaml`, the spec path from `spec.path` or as the last argument |
| `go run github.com/doordash-oss/oapi-codegen-dd/v3/cmd/oapi-codegen ...` | `go tool mockzilla-codegen generate ...` ([getting started](../getting-started.md)) |

### Config

| oapi-codegen-dd | mockzilla-codegen |
|---|---|
| `package` | `package` |
| `copyright-header` | `header` |
| `skip-prune` | `spec.prune: false` |
| `overlay.sources` | `spec.overlays` |
| `output.use-single-file: true`, `output.directory`, `output.filename` | `output.file`, the path of that file |
| `output.use-single-file: false` | `output.files`, each part to the file you name ([output files](../config.md#output-files)) |
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
| `generate.mcp-server` | `mcp:` |
| `generate.mcp-server.default-skip` | `mcp.default-skip` |
| `filter.include`, `filter.exclude` and their keys | `spec.filter.include`, `spec.filter.exclude`, unchanged |
| `additional-imports` | [`imports`](../templates.md#imports); no `.` alias |
| `error-mapping` | `models.error-mapping` |
| `client.name`, `client.timeout` | `client.name`, `client.timeout` |
| `user-templates` | `templates` for the [blocks that may be replaced](../templates.md#blocks), else [`extra-files`](../templates.md#extra-files) |
| `user-context` | `user-context` |

The keys of `error-mapping` are type names in both. An error response written inline in an
operation is `<Op>Response<Status>` here ([names](../naming.md#names-for-types-without-a-name)),
where it was `<Op>ErrorResponse`.

### Not carried over

- `base-path`: a relative `$ref` is resolved against the spec file.
- `generate.validation.simple`: validation is plain generated code ([validation](../validation.md)).
- `generate.handler.server.handler-package`: the module path comes from `go.mod`, or
  `output.module`.
- `generate.handler.models-package`, `handler-package-alias`, `models-package-alias`: one run
  writes every folder and the imports between them.

### Extensions

The list is the same, `x-mcp` included ([extensions](../extensions.md)), apart from these:

| oapi-codegen-dd | mockzilla-codegen |
|---|---|
| `x-oapi-codegen-extra-tags` | `x-go-extra-tags`; the old name is ignored |
| `x-oapi-codegen-only-honour-go-name` | `x-go-name-exact`; the old name is ignored |
| `x-go-type-name` on a component | declares the type under the new name only, without an alias under the component name |

### Server

| oapi-codegen-dd | mockzilla-codegen |
|---|---|
| `<Name>Interface`, methods `(ctx, *<Op>ServiceRequestOptions) (*<Op>ResponseData, error)` | the same ([server](../server.md)) |
| `New<Op>ResponseData(body)`, `.WithStatus(code)` | `New<Op>ResponseData<Status>(body)` per status; `WithStatus` stays ([response data](../server.md#response-data)) |
| `(resp.WithStatus(404), err)` for an error response | `(nil, err)` with the mapped error type, written with the status of the first response that carries it ([HTTP adapter](../server.md#http-adapter)) |
| `<Op>Path`, `opts.Header` | `<Op>PathParams`, `opts.Headers`; `opts.Cookies` too |
| `NewRouter(svc, opts...)` | the same, with `WithMiddleware`, `WithErrorHandler` and `WithRouter` ([router](../server.md#router)) |
| `NewHTTPAdapter(svc, errHandler)` | `NewHTTPAdapter(svc, opts...)`, the error handler through `WithErrorHandler` |
| scaffolds in a folder next to the output | where `server.scaffold` says; `main` imports the others by module path |

### Client

| oapi-codegen-dd | mockzilla-codegen |
|---|---|
| `NewDefaultPetClient(baseURL, opts...)`, `NewPetClient(runtime.APIClient)` | `NewPetClient(baseURL, opts...)`; `WithHTTPClient` takes anything with `Do` ([client](../client.md#client)) |
| `<Op>(ctx, options, reqEditors...)` returning the success body | the same, `editors` after those of `WithRequestEditor` on the client ([methods](../client.md#methods)) |
| an error that unwraps to `Error` | an error that unwraps to `*Error` |
| `<Op>WithResponse` returning `<Op>Resp` and an error for a 4xx | `<Op>WithResponse` returning `<Op>Response` with `HTTPResponse`, `Body`, `JSON<status>` and `Headers<status>`, and no error for a documented status ([envelopes](../client.md#envelopes)) |
| `<Op>Response`, the success body | the envelope; the body is its own type, `Pet` here |
| `<Op>Stream` over `runtime.Stream[T]` | the same ([streaming](../client.md#streaming)) |

### Types

- A `oneOf` or `anyOf` is a struct with one field per variant, whatever their number
  ([unions](../types.md#unions)). Before, it was `runtime.Either[A, B]` for two variants and a raw
  message with accessors for more.
- A `number` without a format is `float64` ([type mapping](../types.md#type-mapping)).
- The [pointer rules](../types.md#pointers) and [naming rules](../naming.md) are written down.
  Compare a generated file against the old one before touching call sites.
