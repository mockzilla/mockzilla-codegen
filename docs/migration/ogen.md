# Migrating from ogen

For projects on `github.com/ogen-go/ogen`. Both tools read the same specs and generate a typed
server and client with validation. The generated code has a different shape.

The guide walks through [examples/migration/ogen](../../examples/migration/ogen): a petstore
spec, the ogen config, its translation, and the service and client calls after the move, which
are built and tested.

## What is different

- You choose how optional values look: see [Optional values](#optional-values).
- A handler returns response data, not a sum type: `&pet` becomes `NewGetPetResponseData200(&pet)`.
- With `models.error-mapping`, an error schema is a Go error. The service returns it, and it is
  written with the status the spec gives it.
- The client returns the success body, and an error for any other status. There is no `<Op>Res`
  to switch on.
- The params of an operation sit in one options struct, with a field for each place they go:
  `PathParams`, `Query`, `Headers` and `Cookies`. The body is `Body` in the same struct, not a
  second argument.
- JSON goes through `encoding/json`, not `jx`.
- A `uuid`, `uri` or `ipv4` format is a `string` checked by `Validate`, unless
  `models.format-types` names a type for it.
- There is no security handler and no OpenTelemetry. A middleware checks credentials, and a
  wrapped `http.Client` traces the client.
- The routes go on the router of `server.framework`. `std-http` is the standard library's
  `ServeMux`.

## Before and after

### The handler

The handler of `GET /pets/{id}` in ogen, and how it is mounted:

```go
func (s *Service) GetPet(ctx context.Context, params GetPetParams) (GetPetRes, error) {
	p, ok := s.pets.Get(params.ID)
	if !ok {
		return &Error{Code: http.StatusNotFound, Message: "no such pet"}, nil
	}
	return &p, nil
}

h, err := NewServer(&Service{})
```

The same handler after the move, from [service.go](../../examples/migration/ogen/service.go):

```go
func (s *Service) GetPet(_ context.Context, opts *GetPetServiceRequestOptions) (*GetPetResponseData, error) {
	p, ok := s.pets.Get(opts.PathParams.ID)
	if !ok {
		return nil, &Error{Code: http.StatusNotFound, Message: "no such pet"}
	}
	return NewGetPetResponseData200(&p), nil
}

h := NewRouter(&Service{})
```

`return NewGetPetResponseData404(&Error{...}), nil` gives the same response. Returning the error
lets code deep in the service fail with the type the spec documents.

### The client call

The client call in ogen:

```go
c, err := NewClient("http://localhost:8080")
if err != nil {
	return err
}
res, err := c.GetPet(ctx, GetPetParams{ID: 1})
if err != nil {
	return err
}
var pet *Pet
switch r := res.(type) {
case *Error:
	return fmt.Errorf("no pet: %s", r.Message)
case *Pet:
	pet = r
}
```

The same call after the move, as
[service_test.go](../../examples/migration/ogen/service_test.go) makes it:

```go
c, err := NewClient("http://localhost:8080")
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

### An optional field

An optional field in ogen:

```go
body := NewPet{Name: "Rex", Tag: NewOptNilString("dog")}
if tag, ok := pet.Tag.Get(); ok {
	fmt.Println(tag)
}
```

The same field after the move, with the default, a pointer:

```go
body := NewPet{Name: "Rex", Tag: new("dog")}
if pet.Tag != nil {
	fmt.Println(*pet.Tag)
}
```

### Optional values

ogen wraps an optional value in an `Opt` type, such as `OptString` or `OptNilString`. Here you
choose what it becomes:

| Choice | How | `OptString` becomes | null and absent |
|---|---|---|---|
| pointer | the default | `*string` | one state: `nil` |
| `runtime.Nullable` | `models.nullable: true`, or `x-go-nullable: true` on one field | `runtime.Nullable[string]`, with `Get` and `Or` as in ogen | two states, `runtime.Null[string]()` and absent |
| plain value | `x-go-type-skip-optional-pointer` on one field | `string` | the zero value counts as absent |

See [pointers](../types.md#pointers) and [nullable](../types.md#nullable).

## Steps

1. Add the CLI to the module: `go get -tool github.com/mockzilla/mockzilla-codegen/cmd/mockzilla-codegen`.
   ogen can stay until the build is green again.
2. Write `codegen.yaml` next to the spec, from the [command](#command) and [config](#config)
   tables. An unknown key is an error that names its path, so a key that did not carry over
   shows on the first run.
3. Keep the spec. Replace the `x-ogen-*` extensions it uses, from the
   [extensions table](#extensions). They are ignored here, so they can stay while both tools run.
4. Replace the `go:generate` line with `//go:generate go tool mockzilla-codegen generate -c codegen.yaml`,
   and delete the `oas_*_gen.go` files.
5. Generate and run `go build ./...`. Each error is a call site to move: handlers as above, client
   calls, `Opt` types.
6. Add `mockzilla-codegen generate -check` to CI, so a spec change without a new run fails there.

## Reference

### Command

ogen is driven by flags, with an optional config file. The same holds here: the output flags have
a counterpart, and the config file is optional. The rest moves into `codegen.yaml`.

| ogen | mockzilla-codegen |
|---|---|
| `ogen --target ./api --package api --clean api.yaml` | `mockzilla-codegen generate -o ./api/gen.go -package api api.yaml`, or `output.file: ./api/gen.go` and `package: api` in `codegen.yaml` |
| `--config ogen.yaml` | `-c codegen.yaml`; the spec path from `spec.path` or as the last argument |
| `--clean` | none needed: every file the config names is written on every run |
| `--initialisms`, `--initialisms-extra` | `naming.initialisms`, added to the built-in set; the set cannot be replaced |
| `--strict` | `-strict` exits 1 on a warning too; an error exits 1 without it. Files are written either way |
| `--version` | `mockzilla-codegen version` |
| `-v`, `--loglevel` | `-v` prints info diagnostics and the files written |

ogen writes one file per concern: `oas_client_gen.go`, `oas_server_gen.go`, `oas_schemas_gen.go`
and so on. mockzilla-codegen writes one file, or the files `output.files` names
([output files](../config.md#output-files)).

### Config

| ogen | mockzilla-codegen |
|---|---|
| `parser.infer_types` | always: a schema without `type` is read from its keywords ([type mapping](../types.md#type-mapping)) |
| `generator.features.enable: [paths/client]` | `client:` |
| `generator.features.enable: [paths/server]` | `server:` with a `framework` |
| `generator.features.enable: [webhooks/client, webhooks/server]` | always: a webhook gets its types and a service method, no route and no client method |
| `generator.features.enable: [client/editors]` | always: `WithRequestEditor` on the client, and `editors` per call |
| `generator.features.enable: [client/request/validation]` | `Validate` on every request options struct, called by you |
| `generator.features.enable: [server/response/validation]` | `server.validation.response: true` |
| `generator.features.enable: [ogen/unimplemented]` | `server.scaffold.service`: a stub per operation returning `ErrNotImplemented`, written once |
| `generator.features.disable_all` | leave `server` and `client` out: models only |
| `generator.filters.path_regex` | `spec.filter.include.paths`, the exact paths; `tags` and `operation-ids` filter too |
| `generator.convenient_errors` | `models.error-mapping`, per error schema, without a `default` response on every operation |
| `generator.initialisms` | `naming.initialisms`; `inherit` is implied |

### Not carried over

- `parser.allow_remote`, `depth_limit`, `authentication_schemes`, `allow_cross_type_constraints`,
  `disallow_duplicate_method_paths`: nothing to replace.
- `generator.features.enable: [client/request/options]`, `client/security/reentrant`: nothing to
  replace.
- `generator.features.enable: [ogen/otel]`: wrap the `http.Client` you pass, and add a middleware
  on the server.
- `generator.filters.methods`: filter by path, tag or operation ID.
- `generator.ignore_not_implemented`: not needed. What cannot be generated is reported as a
  diagnostic and left out, and the run goes on.
- `generator.content_type_aliases`, `wildcard_content_type_default`: a wildcard media type is
  JSON unless the field is a string or bytes ([HTTP adapter](../server.md#http-adapter)).
- `expand`: `-dry-run` prints what a run would write.
- `--cpuprofile`, `--memprofile`, `--color`: nothing to replace.

### Extensions

| ogen | mockzilla-codegen |
|---|---|
| `x-ogen-name` on a schema | `x-go-type-name` |
| `x-ogen-name` on a property | `x-go-name` |
| `x-ogen-properties: {prop: {name: X}}` | `x-go-name: X` on the property |
| `x-ogen-type` | `x-go-type` and `x-go-type-import` |
| `x-oapi-codegen-extra-tags` | `x-go-extra-tags`; the old name is ignored |
| `x-ogen-time-format` | none; `x-go-type` with a type of your own that parses the format |
| `x-ogen-validate`, `x-ogen-json-streaming`, `x-ogen-raw-response`, `x-ogen-custom-security`, `x-ogen-server-name`, `x-ogen-extension` | none |
| `x-ogen-operation-group` | none; one service interface |
| `x-ogen-sse-event-shape` | none; `client.streaming` reads the `data` of each event as one frame ([streaming](../client.md#streaming)) |

The full list is in [extensions](../extensions.md).

### Server

| ogen | mockzilla-codegen |
|---|---|
| `Handler` interface, methods `(ctx, params <Op>Params) (<Op>Res, error)` | `ServiceInterface`, methods `(ctx, *<Op>ServiceRequestOptions) (*<Op>ResponseData, error)` ([service interface](../server.md#service-interface)) |
| `<Op>Params` with every parameter | `opts.PathParams`, `opts.Query`, `opts.Headers`, `opts.Cookies`, one struct per location |
| the body as a second argument | `opts.Body` |
| `<Op>Res` sum type, one type per status: `&Pet{}`, `&<Op>NotFound{}` | `New<Op>ResponseData200(&pet)`, `New<Op>ResponseData404(&err)` ([response data](../server.md#response-data)) |
| `NewError(ctx, err) *ErrorStatusCode` on the handler | the service returns the mapped error type; the adapter writes it with the status of the first response carrying it |
| `NewServer(h, opts...)`, `WithMiddleware`, `WithErrorHandler`, `WithNotFound` | `NewRouter(svc, opts...)` with `WithMiddleware`, `WithErrorHandler`, `WithRouter` ([router](../server.md#router)) |
| `middleware.Middleware` | `func(http.Handler) http.Handler` |
| the static radix router | the router of `server.framework`; `std-http` is the standard library's `ServeMux` |
| `UnimplementedHandler` | the service scaffold |
| `SecurityHandler` | none; check credentials in a middleware, `opts.RawRequest` has the headers |
| `ogenerrors.DecodeParamsError` and friends | `*HandlerError` with a kind. Its `Body` is the mapped error type the operation documents for 400, and the default handler writes it ([errors](../server.md#errors)) |

### Client

| ogen | mockzilla-codegen |
|---|---|
| `NewClient(serverURL, opts...)`, with a `SecuritySource` when the spec has security | `NewClient(baseURL, opts...)`; credentials go in a `WithRequestEditor` ([client](../client.md#client)) |
| `Invoker` interface | `<Name>Interface` |
| `<Op>(ctx, params) (<Op>Res, error)` | `<Op>(ctx, opts) (body, error)`: the success body, and an error for every other status ([methods](../client.md#methods)) |
| `<Op>(ctx, request, params)` with a body | `opts.Body` |
| a `switch` on the `<Op>Res` type | `errors.As` on the error type, or `client.with-response: true` for an [envelope](../client.md#envelopes) with a field per status |
| `WithClient(ht.Client)` | `WithHTTPClient(d)`, anything with `Do` |
| `WithTracerProvider`, `WithMeterProvider` | none |

### Types

| ogen | mockzilla-codegen |
|---|---|
| `OptString`, `OptInt`, `OptPet` | `*string`, `*int`, `*Pet`: `.Get()` becomes a nil check, and `NewOptString(v)` becomes `new(v)` ([pointers](../types.md#pointers)). With `models.nullable: true`, `runtime.Nullable[string]`: `.Get()` and `.Or()` stay, and `NewOptString(v)` becomes `runtime.Some(v)` ([nullable](../types.md#nullable)). With `x-go-type-skip-optional-pointer` on a field, a plain `string`. See [Optional values](#optional-values) |
| `NilString`, `OptNilString` | `*string` for both, so `null` and absent are one state. With `models.nullable: true`, `runtime.Nullable[string]` for both, which keeps them apart: `runtime.Null[string]()`, `.IsNull()` |
| `[]T` for an optional array | the same |
| sum type `ID{Type IDType, String string, Int int}` with `NewStringID` | a union struct with a field per variant: `ID{String: &s}` ([unions](../types.md#unions)) |
| `<Schema>Sum` for an inline `oneOf` | a union type named after where it sits ([names](../naming.md#names-for-types-without-a-name)) |
| `uuid.UUID`, `url.URL`, `net.IP`, `time.Duration` for the formats | `string`, checked by `Validate`. `models.format-types` keeps a package type for a format, and `x-go-type` for one schema ([your own type for a format](../types.md#your-own-type-for-a-format)) |
| `time.Time` for `date` | `runtime.Date` |
| `jx` encoders, `Encode`/`Decode` methods | `encoding/json`, `MarshalJSON` only where the shape needs it |
| `Validate() error` | the same, plain code over the runtime helpers ([validation](../validation.md)) |
| `Pet` with `IsSet` helpers | none |

Both tools merge `allOf` into one struct.

An enum is a named string type with constants in both. Here the constants are prefixed with the
type name (`StatusSold`), and a `Validate` method replaces the `MarshalText` check of ogen.
