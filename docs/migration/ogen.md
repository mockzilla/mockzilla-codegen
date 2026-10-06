# Migrating from ogen

For projects on `github.com/ogen-go/ogen`. ogen and mockzilla-codegen read the same specs and both
generate a typed server and client with validation, but the generated code has a different shape:
ogen wraps optional values, mockzilla-codegen uses pointers; ogen returns response sum types,
mockzilla-codegen response data. The config pair in
[examples/migration/ogen](../../examples/migration/ogen) is the one this guide walks through.

## The command

ogen is driven by flags, with an optional config file. The output flags have a counterpart, and
the config file is optional here too. The rest moves into `codegen.yaml`.

| ogen | mockzilla-codegen |
|---|---|
| `ogen --target ./api --package api --clean api.yaml` | `mockzilla-codegen generate -o ./api/gen.go -package api api.yaml`, or `output.file: ./api/gen.go` and `package: api` in `codegen.yaml` |
| `--config ogen.yaml` | `-c codegen.yaml`; the spec path from `spec.path` or as the last argument |
| `--clean` | none needed: every file the config names is written on every run. Delete the `oas_*_gen.go` files once |
| `--initialisms`, `--initialisms-extra` | `naming.initialisms`, added to the built-in set; the set cannot be replaced |
| `--strict` | `-strict` exits 1 on a warning too; an error exits 1 without it. Files are written either way |
| `--no-client`, `--no-server` | `-no-client`, `-no-server`, over what the config says |
| `--debug.*`, `--cpuprofile`, `--memprofile` | none; `-v` prints info diagnostics and the files written |

ogen writes one file per concern, `oas_client_gen.go`, `oas_server_gen.go`, `oas_schemas_gen.go`
and so on. mockzilla-codegen writes one file, or the files `output.files` names
([output files](../../README.md#output-files)).

## Config

| ogen | mockzilla-codegen |
|---|---|
| `parser.infer_types` | always: a schema without `type` is read from its keywords ([type mapping](../types.md#type-mapping)) |
| `parser.allow_remote`, `depth_limit`, `authentication_schemes`, `allow_cross_type_constraints`, `disallow_duplicate_method_paths` | none |
| `generator.features.enable: [paths/client]` | `client:` |
| `generator.features.enable: [paths/server]` | `server:` with a `framework` |
| `generator.features.enable: [webhooks/client, webhooks/server]` | always: a webhook gets its types and a service method, no route and no client method |
| `generator.features.enable: [client/editors]` | always: `WithRequestEditor` on the client, and `editors` per call |
| `generator.features.enable: [client/request/validation]` | `Validate` on every request options struct, called by you |
| `generator.features.enable: [client/request/options]`, `client/security/reentrant` | none |
| `generator.features.enable: [server/response/validation]` | `server.validation.response: true` |
| `generator.features.enable: [ogen/unimplemented]` | `server.scaffold.service`: a stub per operation returning `ErrNotImplemented`, written once |
| `generator.features.enable: [ogen/otel]` | none; wrap the `http.Client` you pass, and add a middleware on the server |
| `generator.features.disable_all` | leave `server` and `client` out: models only |
| `generator.filters.path_regex` | `spec.filter.include.paths`, the exact paths; `tags` and `operation-ids` filter too |
| `generator.filters.methods` | none |
| `generator.convenient_errors` | `models.error-mapping` on the error type, see below |
| `generator.ignore_not_implemented` | none needed: what cannot be generated is reported as a diagnostic and left out, and the run goes on |
| `generator.content_type_aliases`, `wildcard_content_type_default` | none; a wildcard media type is JSON unless the field is a string or bytes ([HTTP adapter](../server.md#http-adapter)) |
| `generator.initialisms` | `naming.initialisms`; `inherit` is implied |
| `expand` | none; `-dry-run` prints what a run would write |

## Extensions

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

## Generated code

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
| `ogenerrors.DecodeParamsError` and friends | `*HandlerError` with a kind; its `Body` is the mapped error type the operation documents for 400, which the default handler writes ([errors](../server.md#errors)) |

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
| `OptString`, `OptInt`, `OptPet` | `*string`, `*int`, `*Pet`; `.Get()` becomes a nil check, `.Set = true` becomes `new(v)` ([pointers](../types.md#pointers)) |
| `NilString`, `OptNilString` | `*string` for both; `null` and absent are one state |
| `[]T` for an optional array | the same |
| sum type `ID{Type IDType, String string, Int int}` with `NewStringID` | a union struct with a field per variant: `ID{String: &s}` ([unions](../types.md#unions)) |
| `<Schema>Sum` for an inline `oneOf` | a union type named after where it sits ([names](../naming.md#names-for-types-without-a-name)) |
| `uuid.UUID`, `url.URL`, `net.IP`, `time.Duration` for the formats | `string`, checked by `Validate`; `models.format-types` keeps a package type for a format, `x-go-type` for one schema ([your own type for a format](../types.md#your-own-type-for-a-format)) |
| `time.Time` for `date` | `runtime.Date` |
| `jx` encoders, `Encode`/`Decode` methods | `encoding/json`, `MarshalJSON` only where the shape needs it |
| `Validate() error` | the same, plain code over the runtime helpers ([validation](../validation.md)) |
| `Pet` with `IsSet` helpers | none |

`allOf` is merged into one struct in both. An enum is a named string type with constants in both,
prefixed with the type name here (`StatusSold`), and with a `Validate` instead of ogen's
`MarshalText` check.
