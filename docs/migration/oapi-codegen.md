# Migrating from oapi-codegen

For projects on `github.com/oapi-codegen/oapi-codegen/v2`, or the older `deepmap/oapi-codegen`
import path.

The guide walks through [examples/migration/oapi-codegen](../../examples/migration/oapi-codegen):
a petstore spec, the oapi-codegen config, its translation, and the service and client calls after
the move, which are built and tested.

## What is different

- There is one server shape. A handler is a method of the service interface: it gets `ctx` and
  one options struct, and returns response data or an error. There is no `http.ResponseWriter`.
  A strict server handler keeps its flow. A plain `ServerInterface` handler is rewritten.
- Error schemas are Go errors. With `models.error-mapping`, `Error` gets an `Error()` method. The
  service returns it, and it is written with the status the spec gives it.
- The client returns the success body. `GetPet` returns `*Pet`, and any other status is an error.
  `client.with-response: true` adds the `<Op>WithResponse` methods, on the same client.
- The params of an operation sit in one options struct, by where they go: `PathParams`, `Query`,
  `Headers`, `Cookies` and `Body`.
- `Id` is `ID`. The [naming rules](../naming.md) are fixed, and close to
  `ToCamelCaseWithInitialisms`.
- A union is a struct with one field per variant, not a raw message with `As` and `From` methods.
- Every type gets `Validate() error`. The server can check requests and responses with plain
  generated code, without kin-openapi ([validation](../validation.md)).

## Before and after

The handler of `GET /pets/{id}` in the strict server, and how it is mounted:

```go
func (s *Server) GetPet(ctx context.Context, req GetPetRequestObject) (GetPetResponseObject, error) {
	p, ok := s.pets.Get(req.Id)
	if !ok {
		return GetPet404JSONResponse{Code: http.StatusNotFound, Message: "no such pet"}, nil
	}
	return GetPet200JSONResponse(p), nil
}

h := HandlerFromMux(NewStrictHandler(&Server{}, nil), chi.NewRouter())
```

The same handler after the move, from
[service.go](../../examples/migration/oapi-codegen/service.go):

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

The client call, with `ClientWithResponses`:

```go
c, err := NewClientWithResponses("http://localhost:8080")
if err != nil {
	return err
}
rsp, err := c.GetPetWithResponse(ctx, 1)
if err != nil {
	return err
}
if rsp.JSON404 != nil {
	return fmt.Errorf("no pet: %s", rsp.JSON404.Message)
}
pet := rsp.JSON200
```

The same call after the move, as
[service_test.go](../../examples/migration/oapi-codegen/service_test.go) makes it:

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

`GetPetWithResponse` is still there: it returns the envelope with `JSON404` set and no error.

## Steps

1. Add the CLI to the module: `go get -tool github.com/mockzilla/mockzilla-codegen/cmd/mockzilla-codegen`.
   oapi-codegen can stay until the build is green again.
2. Write `codegen.yaml` next to the spec, from the [config table](#config). An unknown key is an
   error that names its path, so a key that did not carry over shows on the first run.
3. Keep the spec. Rename two extensions: `x-oapi-codegen-extra-tags` is `x-go-extra-tags`, and
   `x-oapi-codegen-only-honour-go-name` is `x-go-name-exact`. The old names are ignored, so keep
   both while both tools run.
4. Replace the `go:generate` line with `//go:generate go tool mockzilla-codegen generate -c ../codegen.yaml`.
   Delete the old generated file, or point `output.file` at it.
5. Generate and run `go build ./...`. Each error is a call site to move: handlers as above, client
   calls, `Id` to `ID`, unions.
6. Add `mockzilla-codegen generate -check` to CI, so a spec change without a new run fails there.

## Reference

### Command

| oapi-codegen | mockzilla-codegen |
|---|---|
| `oapi-codegen -config cfg.yaml api.yaml` | `mockzilla-codegen generate -c codegen.yaml`, the spec path from `spec.path` or as the last argument |
| `//go:generate go tool oapi-codegen -config cfg.yaml ../api.yaml` | `//go:generate go tool mockzilla-codegen generate -c ../codegen.yaml` |
| `oapi-codegen -version` | `mockzilla-codegen version` |
| `-package api -o api.gen.go` | `-package api -o api.gen.go` |
| `-generate types,client,chi-server` | `-client -server chi`; models are always written |
| the other command line flags | config keys |

### Config

| oapi-codegen | mockzilla-codegen |
|---|---|
| `package` | `package` |
| `output` | `output.file` |
| `generate.models` | always on |
| `generate.chi-server`, `echo-server`, `echo5-server`, `gin-server`, `gorilla-server`, `iris-server`, `std-http-server`, `fiber-v3-server` | `server.framework: chi`, `echo`, `echo-v5`, `gin`, `gorilla-mux`, `iris`, `std-http`, `fiber` |
| `generate.strict-server` | always: the [service interface](../server.md#service-interface) is the only server shape |
| `generate.client` | `client:` |
| `output-options.client-type-name` | `client.name` |
| `output-options.skip-fmt` | `output.format: false` |
| `output-options.skip-prune` | `spec.prune: false` |
| `output-options.include-tags`, `exclude-tags` | `spec.filter.include.tags`, `spec.filter.exclude.tags` |
| `output-options.include-operation-ids`, `exclude-operation-ids` | `spec.filter.include.operation-ids`, `spec.filter.exclude.operation-ids` |
| `output-options.exclude-schemas` | `spec.filter.exclude.schema-properties` drops properties; an [overlay](../config.md) removes a whole schema |
| `output-options.overlay.path` | `spec.overlays: [path]`, several in order |
| `output-options.additional-initialisms` | `naming.initialisms`, in effect without a `name-normalizer` |
| `output-options.yaml-tags`, `struct-tags` | `models.extra-tags: [yaml]`; each tag repeats the JSON name |
| `output-options.skip-enum-validate` | `models.validation.skip`, for every `Validate` method |
| `output-options.user-templates` | `templates` for the [blocks that may be replaced](../templates.md#blocks), else [`extra-files`](../templates.md#extra-files) |
| `output-options.prefer-skip-optional-pointer-on-container-types` | the default: a slice or map never gets a pointer |
| `output-options.nullable-type` | `models.nullable: true`: every field that may be absent or null is a `runtime.Nullable[T]`, optional ones too ([nullable](../types.md#nullable)) |
| `output-options.streaming-content-types` | `client.streaming` reads `text/event-stream` and line-delimited JSON; the list is fixed |
| `output-options.resolve-type-name-collisions` | always ([naming](../naming.md)) |
| `output-options.generate-types-for-anonymous-schemas` | always: every inline object is a named type |
| `compatibility.always-prefix-enum-values` | `naming.enum-prefix`, on by default |
| `compatibility.allow-unexported-struct-field-names` | `x-go-name-exact` per field |
| `compatibility.apply-chi-middleware-first-to-last`, `apply-gorilla-middleware-first-to-last` | the default: `WithMiddleware` wraps outermost first |
| `compatibility.disable-required-readonly-as-pointer` | the default: a required `readOnly` field is a plain value with `omitempty` |
| `additional-imports` | [`imports`](../templates.md#imports), same `package` and `alias`; no `.` alias |

`models.error-mapping` has no oapi-codegen key. It turns an error schema into a Go error, as in
[Before and after](#before-and-after).

### Not carried over

- `generate.fiber-server`: `fiber` is Fiber v3; Fiber v2 has no router here.
- `generate.embedded-spec` and `GetSwagger()`: embed the spec yourself with `//go:embed`.
- `generate.server-urls`: the client takes a base URL string.
- `output-options.name-normalizer`: the [naming rules](../naming.md) are fixed. They are closest
  to `ToCamelCaseWithInitialisms`, so a project on `ToCamelCase` sees `Id` become `ID`.
- `output-options.prefer-skip-optional-pointer`: no global switch; `x-go-type-skip-optional-pointer`
  per field.
- `output-options.response-type-suffix`, `content-types`: type names follow fixed rules,
  `<Op>Response<Status>` and `<Op>JSONRequestBody` ([names](../naming.md#names-for-types-without-a-name)).
- `output-options.type-mapping`, `disable-type-aliases-for-type`: `x-go-type` on a schema, and
  `models.int-type` for integers without a format.
- `output-options.client-response-bytes-function`, `skip-client-response-content-type`,
  `skip-response-body-getters`: an [envelope](../client.md#envelopes) always has `Body` and
  `HTTPResponse`, and a field per documented body.
- `output-options.lenient-union-accessors`, `skip-enum-via-oneof`, `prefer-skip-optional-pointer-with-omitzero`:
  unions and enums have one shape ([unions](../types.md#unions)); `omitzero` is added where a
  struct has `omitempty`, and an optional slice or map has `omitzero` alone, so an empty one is
  sent.
- `compatibility.schema-merging-behavior`, `old-merge-schemas`, `old-allof-sibling-merging`,
  `old-enum-conflicts`, `old-aliasing`: an `allOf` is merged into one type, with its sibling
  properties ([allOf](../types.md#allof)), and enum constants are prefixed. The old behaviours
  cannot be brought back.
- `compatibility.disable-flatten-additional-properties`: an object without properties is a map.
- `compatibility.headers-implicitly-required`: response headers follow their `required` flag.
- `compatibility.sort-handler-registrations`: routes are registered in spec order.
- `compatibility.enable-auth-scopes-on-context`, `circular-reference-limit`,
  `preserve-original-operation-id-casing-in-embedded-spec`: nothing to replace.
- `import-mapping`: a `$ref` into another file is resolved and its types generated with the rest.
  To keep several packages, run once with `output.files` and `output.packages` sending parts to
  their folders ([output files](../config.md#output-files)); the imports between them are
  written for you.

### Extensions

Every extension oapi-codegen documents keeps its meaning, apart from these
([extensions](../extensions.md)):

| oapi-codegen | mockzilla-codegen |
|---|---|
| `x-oapi-codegen-extra-tags` | `x-go-extra-tags`; the old name is ignored |
| `x-oapi-codegen-only-honour-go-name` | `x-go-name-exact`; the old name is ignored |
| `x-enum-varnames`, `x-enumNames` | `x-enum-names` |
| `x-omitzero` | none; `omitzero` is written where a struct, slice or map needs it |
| `x-order` | none; fields keep the order of the spec |
| `x-oapi-codegen-enum-merge` | none |
| `x-go-type-name` on a component | declares the type under the new name only, no alias under the component name |

### Server

| oapi-codegen | mockzilla-codegen |
|---|---|
| `ServerInterface`, methods `(w http.ResponseWriter, r *http.Request, params P)` | `ServiceInterface`, methods `(ctx, *<Op>ServiceRequestOptions) (*<Op>ResponseData, error)` |
| `StrictServerInterface`, methods `(ctx, <Op>RequestObject) (<Op>ResponseObject, error)` | the same interface; `<Op>RequestObject.Params.Limit` is `opts.Query.Limit`, a path param `req.Id` is `opts.PathParams.ID`, `.Body` is `opts.Body` |
| `<Op>200JSONResponse{...}` returned as the response object | `New<Op>ResponseData200(&body)`, headers with `.WithTypedHeaders(...)` ([response data](../server.md#response-data)) |
| `NewStrictHandler(ssi, middlewares)` | nothing; the adapter is the strict handler |
| `Handler(si)`, `HandlerFromMux(si, r)`, `HandlerWithOptions(si, opts)` | `NewRouter(svc, opts...)`; `WithRouter(r)` registers on an existing router |
| `ServerInterfaceWrapper` | `HTTPAdapter`, `NewHTTPAdapter(svc, opts...)` for hand-written routing |
| `ChiServerOptions.Middlewares`, `MiddlewareFunc` | `WithMiddleware(func(http.Handler) http.Handler)` |
| `ErrorHandlerFunc`, `RequestErrorHandlerFunc` | `WithErrorHandler(ErrorHandlerFunc(...))`, given a `*HandlerError` with the kind and the operation ([errors](../server.md#errors)) |
| `OapiRequestValidator` middleware from `nethttp-middleware` over `kin-openapi` | `server.validation.request: true`; plain generated code, nothing to import ([validation](../validation.md)) |
| the security requirements checked by the middleware | none generated; check them in a middleware of your own |
| the status written by hand in the handler | the status of the response data, or of the error type returned ([HTTP adapter](../server.md#http-adapter)) |

The [scaffolds](../server.md#scaffolds) write a service stub, a middleware file and a `main`
once, which oapi-codegen leaves to the project.

### Client

| oapi-codegen | mockzilla-codegen |
|---|---|
| `NewClient(server, WithHTTPClient(c), WithRequestEditorFn(fn))` | `NewClient(baseURL, WithHTTPClient(c), WithRequestEditor(fn))`; the type is named by `client.name` |
| `Client.<Op>(ctx, params, body, reqEditors...)` returning `*http.Response` | `<Op>(ctx, opts, editors...)` returns the success body, and an error for any other status ([methods](../client.md#methods)); `<Op>Request(ctx, opts, editors...)` builds the request without sending it |
| `ClientWithResponses`, `<Op>WithResponse` returning `<Op>Response` with `JSON200`, `Body`, `HTTPResponse` | `client.with-response: true`; the same method and field names on one client type ([envelopes](../client.md#envelopes)) |
| `Parse<Op>Response(rsp)` | none |
| `<Op>Params` and a body argument | one `<Op>RequestOptions` struct: `Query`, `Headers`, `PathParams`, `Cookies`, `Body` ([request options](../client.md#request-options)) |
| `reqEditors` per call | `editors` per call, after those of `WithRequestEditor` on the client |
| `<Op>JSONRequestBody` | `<Op>RequestBody`, or the referenced type; `JSON` appears only with several media types |
| `ClientInterface` | `<Name>Interface`, checked at compile time |

A 4xx or 5xx is an error: the type of `models.error-mapping` when the spec documents it, else
`runtime.APIError` with the status and the body.

### Types

| Schema | oapi-codegen | mockzilla-codegen |
|---|---|---|
| `number` without a format, or with an unknown one | `float32` | `float64` |
| `string` with format `uuid` | `uuid.UUID` | `string`, checked by validation; `uuid.UUID` with `models.format-types` |
| `string` with format `email` | `runtime.Email` that fails JSON encoding and decoding on a bad address | `runtime.Email` that decodes any string; `Validate` checks it |

Beyond the type mapping:

- `openapi_types.Date`, `File`, `Email` and `UUID` are `runtime.Date`, `runtime.File`,
  `runtime.Email` and a validated `string`. `models.format-types` keeps `uuid.UUID` for every
  `format: uuid` ([your own type for a format](../types.md#your-own-type-for-a-format)), and
  `x-go-type` does it for one schema.
- A union is a struct with one field per variant, set or nil, instead of a raw `union` with
  `As<Variant>`, `From<Variant>` and `Merge<Variant>` ([unions](../types.md#unions)).
- Enum constants are `<Type><Value>` for every enum, not only on conflict, unless
  `naming.enum-prefix: false`.
- `AdditionalProperties` stays a map field with `Get` and `Set`, written next to the properties
  ([additionalProperties](../types.md#additionalproperties)).

