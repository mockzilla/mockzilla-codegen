# Migrating from oapi-codegen

For projects on `github.com/oapi-codegen/oapi-codegen/v2`, or the older `deepmap/oapi-codegen`
import path. The fork `goapi-gen` has [its own section](#goapi-gen) at the end. The config pair
in [examples/migration/oapi-codegen](../../examples/migration/oapi-codegen) is the one this guide
walks through.

## The command

| oapi-codegen | mockzilla-codegen |
|---|---|
| `oapi-codegen -config cfg.yaml api.yaml` | `mockzilla-codegen generate -c codegen.yaml`, the spec path from `spec.path` or as the last argument |
| `//go:generate go tool oapi-codegen -config cfg.yaml ../api.yaml` | `//go:generate go tool mockzilla-codegen generate -c ../codegen.yaml` |
| `oapi-codegen -version` | `mockzilla-codegen version` |
| the command line flags | none; everything is in the config |

The output is written on every run, so remove the old generated file first, or point
`output.file` at it and let the run replace it.

## Config

Unknown keys are errors, so start from the table and add what the first run asks for. A key
without a row here has no equivalent; the [notes](#what-has-no-key) below say what to do instead.

| oapi-codegen | mockzilla-codegen |
|---|---|
| `package` | `package` |
| `output` | `output.file` |
| `generate.models` | always on |
| `generate.chi-server`, `echo-server`, `echo5-server`, `gin-server`, `gorilla-server`, `iris-server`, `std-http-server`, `fiber-v3-server` | `server.framework: chi`, `echo`, `echo-v5`, `gin`, `gorilla-mux`, `iris`, `std-http`, `fiber` |
| `generate.fiber-server` (Fiber v2) | none; `fiber` is Fiber v3 |
| `generate.strict-server` | always: the [service interface](../server.md#service-interface) is the only server shape |
| `generate.client` | `client:` |
| `output-options.client-type-name` | `client.name` |
| `output-options.skip-fmt` | `output.format: false` |
| `output-options.skip-prune` | `spec.prune: false` |
| `output-options.include-tags`, `exclude-tags` | `spec.filter.include.tags`, `spec.filter.exclude.tags` |
| `output-options.include-operation-ids`, `exclude-operation-ids` | `spec.filter.include.operation-ids`, `spec.filter.exclude.operation-ids` |
| `output-options.exclude-schemas` | `spec.filter.exclude.schema-properties` drops properties; an [overlay](../../README.md#configuration) removes a whole schema |
| `output-options.overlay.path` | `spec.overlays: [path]`, several in order |
| `output-options.additional-initialisms` | `naming.initialisms`, in effect without a `name-normalizer` |
| `output-options.yaml-tags`, `struct-tags` | `models.extra-tags: [yaml]`; each tag repeats the JSON name |
| `output-options.skip-enum-validate` | `models.validation.skip`, for every `Validate` method |
| `output-options.user-templates` | `templates` for the [blocks that may be replaced](../templates.md#blocks), else [`extra-files`](../templates.md#extra-files) |
| `output-options.prefer-skip-optional-pointer` | none globally; `x-go-type-skip-optional-pointer` per field |
| `output-options.prefer-skip-optional-pointer-on-container-types` | the default: a slice or map never gets a pointer |
| `output-options.streaming-content-types` | `client.streaming` reads `text/event-stream` and line-delimited JSON; the list is fixed |
| `output-options.resolve-type-name-collisions` | always ([naming](../naming.md)) |
| `output-options.generate-types-for-anonymous-schemas` | always: every inline object is a named type |
| `compatibility.always-prefix-enum-values` | `naming.enum-prefix`, on by default |
| `compatibility.allow-unexported-struct-field-names` | `x-go-name-exact` per field |
| `compatibility.apply-chi-middleware-first-to-last`, `apply-gorilla-middleware-first-to-last` | the default: `WithMiddleware` wraps outermost first |
| `compatibility.disable-flatten-additional-properties` | none: an object without properties is a map |
| `compatibility.disable-required-readonly-as-pointer` | the default: a required `readOnly` field is a plain value with `omitempty` |
| `additional-imports` | [`imports`](../templates.md#imports), same `package` and `alias`; no `.` alias |
| `import-mapping` | none, see below |

## What has no key

- `generate.embedded-spec` and `GetSwagger()`: embed the spec yourself with `//go:embed`.
- `generate.server-urls`: the client takes a base URL string.
- `output-options.name-normalizer`: the [naming rules](../naming.md) are fixed. They are closest
  to `ToCamelCaseWithInitialisms`, so a project on `ToCamelCase` sees `Id` become `ID`.
- `output-options.response-type-suffix`, `content-types`: type names follow fixed rules,
  `<Op>Response<Status>` and `<Op>JSONRequestBody` ([names](../naming.md#names-for-types-without-a-name)).
- `output-options.nullable-type`: a nullable field is a pointer ([pointers](../types.md#pointers)),
  there is no `nullable.Nullable[T]`.
- `output-options.type-mapping`, `disable-type-aliases-for-type`: `x-go-type` on a schema, and
  `models.int-type` for integers without a format.
- `output-options.client-response-bytes-function`, `skip-client-response-content-type`,
  `skip-response-body-getters`: an [envelope](../client.md#envelopes) always has `Body` and
  `HTTPResponse`, and a field per documented body.
- `output-options.lenient-union-accessors`, `skip-enum-via-oneof`, `prefer-skip-optional-pointer-with-omitzero`:
  unions and enums have one shape ([unions](../types.md#unions)); `omitzero` is added where a
  struct has `omitempty`.
- `compatibility.schema-merging-behavior`, `old-merge-schemas`, `old-allof-sibling-merging`,
  `old-enum-conflicts`, `old-aliasing`: an `allOf` is merged into one type, with its sibling
  properties ([allOf](../types.md#allof)), and enum constants are prefixed. The old behaviours
  cannot be brought back.
- `compatibility.headers-implicitly-required`: response headers follow their `required` flag.
- `compatibility.sort-handler-registrations`: routes are registered in spec order.
- `compatibility.enable-auth-scopes-on-context`, `circular-reference-limit`,
  `preserve-original-operation-id-casing-in-embedded-spec`: nothing to replace.
- `import-mapping`: a `$ref` into another file is resolved and its types generated with the rest.
  To keep several packages, run once with `output.files` and `output.packages` sending parts to
  their folders ([output files](../../README.md#output-files)); the imports between them are
  written for you.

## Extensions

Every extension oapi-codegen documents keeps its meaning, apart from these
([extensions](../extensions.md)):

| oapi-codegen | mockzilla-codegen |
|---|---|
| `x-oapi-codegen-extra-tags` | `x-go-extra-tags`; the old name is ignored |
| `x-oapi-codegen-only-honour-go-name` | `x-go-name-exact`; the old name is ignored |
| `x-enum-varnames`, `x-enumNames` | `x-enum-names` |
| `x-omitzero` | none; `omitzero` is written next to `omitempty` where a struct needs it |
| `x-order` | none; fields keep the order of the spec |
| `x-oapi-codegen-enum-merge` | none |
| `x-go-type-name` on a component | declares the type under the new name only, no alias under the component name |

## Generated code

### Server

| oapi-codegen | mockzilla-codegen |
|---|---|
| `ServerInterface`, methods `(w http.ResponseWriter, r *http.Request, params P)` | `ServiceInterface`, methods `(ctx, *<Op>ServiceRequestOptions) (*<Op>ResponseData, error)` |
| `StrictServerInterface`, methods `(ctx, <Op>RequestObject) (<Op>ResponseObject, error)` | the same interface; `<Op>RequestObject.Params.Limit` is `opts.Query.Limit`, `.Body` is `opts.Body` |
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

The client turns a 4xx or 5xx into an error: the type of `models.error-mapping` when the spec
documents it, else `runtime.APIError` with the status and the body.

### Types

| Schema | oapi-codegen | mockzilla-codegen |
|---|---|---|
| `number` without a format, or with an unknown one | `float32` | `float64` |
| `string` with format `uuid` | `uuid.UUID` | `string`, checked by validation |
| `string` with format `email` | `runtime.Email` that fails JSON encoding and decoding on a bad address | `runtime.Email` that decodes any string; `Validate` checks it |

Beyond the type mapping:

- `openapi_types.Date`, `File`, `Email` and `UUID` are `runtime.Date`, `runtime.File`,
  `runtime.Email` and a validated `string`. `x-go-type: uuid.UUID` with an import keeps the
  package type.
- A union is a struct with one field per variant, set or nil, instead of a raw `union` with
  `As<Variant>`, `From<Variant>` and `Merge<Variant>` ([unions](../types.md#unions)).
- Enum constants are `<Type><Value>` for every enum, not only on conflict, unless
  `naming.enum-prefix: false`.
- `AdditionalProperties` stays a map field with `Get` and `Set`, written next to the properties
  ([additionalProperties](../types.md#additionalproperties)).
- Every named type gets `Validate() error` ([validation](../validation.md)).

## goapi-gen

`goapi-gen` is a hard fork of `oapi-codegen` v1 for chi, with a flat config file:

| goapi-gen | mockzilla-codegen |
|---|---|
| `output` | `output.file` |
| `package` | `package` |
| `generate: [types, server]` | models are always on; `server: {framework: chi}` |
| `generate: [spec]`, `skip-fmt`, `skip-prune` | none, `output.format: false`, `spec.prune: false` |
| `include-tags`, `exclude-tags` | `spec.filter.include.tags`, `spec.filter.exclude.tags` |
| `exclude-schemas` | see `output-options.exclude-schemas` above |
| `templates` | `templates`, per block, or `extra-files` |
| `import-mapping` | none, see above |
| `alias` | none; a component that is only a `$ref` is always an alias |
| `initialisms` | `naming.initialisms`, added to the built-in set |

Its extensions:

| goapi-gen | mockzilla-codegen |
|---|---|
| `x-go-type` with `type`, `import`, `alias` | `x-go-type` with the type, `x-go-type-import` with `{path, name}` |
| `x-go-type-external` | the same two |
| `x-go-extra-tags` | the same |
| `x-go-optional-value` | `x-go-type-skip-optional-pointer` |
| `x-go-omitempty` | `x-omitempty` |
| `x-go-string` | none |
| `x-go-middlewares` | none; `WithMiddleware` wraps every route, and the `server.router-extra` template block adds routes of your own |

The generated server is the strict shape above, not goapi-gen's `ServerInterface` with the
response writer, so every handler moves to the service interface.
