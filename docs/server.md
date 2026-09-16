# Server

A `server` block in the config generates the service contract, an interface with one method per
operation, a request options type each method receives and a response data type each returns, and
the HTTP side: an adapter that decodes requests and calls the service, the error types, a router
for the framework the block names, and starter files on request.

```yaml
server:
  framework: chi   # or std-http
  name: Pets       # the interface is PetsInterface; defaults to Service
```

## Service interface

```go
type PetsInterface interface {
	// List pets
	ListPets(ctx context.Context, opts *ListPetsServiceRequestOptions) (*ListPetsResponseData, error)
	CreatePet(ctx context.Context, opts *CreatePetServiceRequestOptions) (*CreatePetResponseData, error)
}
```

Every operation has the same shape, even one without parameters or body, so middleware and plugins
treat them alike. The method's comment is the operation's summary and description.

## Request options

```go
type ListPetsServiceRequestOptions struct {
	PathParams *ListPetsPathParams
	Query      *ListPetsQuery
	Headers    *ListPetsHeaders
	Cookies    *ListPetsCookies
	Body       *Pet
	RawRequest *http.Request
}

func (o *ListPetsServiceRequestOptions) Validate() error
```

- One field per parameter location the operation uses, holding the struct of its parameters.
- `Body` holds the request body. An operation with several media types gets one field per media
  type, named after it: `BodyJSON`, `BodyForm`, `BodyMultipart`, `BodyText`. Two media types that
  would share a name are told apart by their type: `BodyXML` for `application/xml`, `BodyTextXML`
  for `text/xml`.
- A body without a schema is `any` for JSON, a `string` for `text/*`, and `[]byte` otherwise.
- `RawRequest` is the request as it came in.
- `Validate` calls the `Validate` of each parameter struct and body that has one, with the paths
  `path`, `query`, `header`, `cookie` and `body`.

## Response data

```go
type ListPetsResponseData struct {
	Status  int
	Headers http.Header
	Body    any
}

func NewListPetsResponseData200(body ListPetsResponse200) *ListPetsResponseData
func NewListPetsResponseDataDefault(status int, body *Problem) *ListPetsResponseData
func (r *ListPetsResponseData) WithStatus(code int) *ListPetsResponseData
func (r *ListPetsResponseData) WithHeaders(h http.Header) *ListPetsResponseData
func (r *ListPetsResponseData) WithTypedHeaders200(h ListPetsResponse200Headers) *ListPetsResponseData
func (r *ListPetsResponseData) Payload() any
func (r *ListPetsResponseData) ContentType() string
```

- One constructor per response the spec documents, named after the status when there are several.
  A range such as `4XX` and `default` take the status as their first argument. A response without
  a body takes no body.
- The body is the JSON media type of the response, else its first one; the content type is
  remembered and written with the response.
- A response that declares headers gets a struct for them, `ListPetsResponse200Headers`, and a
  `WithTypedHeaders` method that adds them; the method carries the status when several responses
  declare headers. A response under `components.responses` gets one struct for every operation
  that uses it, named after the component: `UnauthorizedHeaders`.

```go
func (s *Service) ListPets(ctx context.Context, opts *ListPetsServiceRequestOptions) (*ListPetsResponseData, error) {
	pets, next, err := s.store.List(ctx, opts.Query)
	if err != nil {
		return nil, err
	}
	return NewListPetsResponseData200(pets).
		WithTypedHeaders200(ListPetsResponse200Headers{XTotalCount: len(pets), XNext: next}), nil
}
```

## HTTP adapter

`HTTPAdapter` turns requests into calls of the service, one `http.HandlerFunc` method per
operation. Each handler reads the parameters of every location with the runtime codecs, decodes
the body by the request's `Content-Type`, validates the options when the config asks for it, calls
the service and writes what it returns.

```go
adapter := NewHTTPAdapter(svc, opts...)
mux.HandleFunc("GET /pets", adapter.ListPets)
```

- A body arrives in a media type the operation documents: JSON (`application/json` and `+json`)
  through the JSON decoder, `application/x-www-form-urlencoded` through `DecodeForm`,
  `multipart/form-data` into a struct with `DecodeMultipart`, and any other media type into a
  string or into bytes, whichever its field is. A documented media type that does not fit its
  type, such as XML into a struct, is accepted and left to `RawRequest`.
- Media types are matched without their parameters and in lower case. A wildcard such as `*/*`
  takes every media type the operation does not name, as JSON unless its field is a string or
  bytes. Without a wildcard, a media type the operation does not document is answered with 415.
- A required body that is missing is a 400; a missing optional body leaves its field nil.
- A service that returns an error type of the spec (see `models.error-mapping`), as a value, a
  pointer or wrapped, is answered with the status of the first response that carries the type and
  the error as the body. Any other error is a 500 whose message does not reach the client.
- A service that returns nil for both values is a 500 with `ErrNoResponse`.
- The response is written with its status, headers and content type; a nil body sends the status
  alone.

Options are set with `ServerOption` functions on the adapter and on the router alike:

| Option | Sets |
|---|---|
| `WithMiddleware(mw...)` | `func(http.Handler) http.Handler` wrappers, outermost first |
| `WithErrorHandler(h)` | what writes failed requests, `DefaultErrorHandler{}` by default |
| `WithJSONDecoder(fn)` | what reads JSON bodies, `runtime.DecodeJSON` by default |
| `WithMultipartMaxMemory(n)` | memory for multipart forms, `server.multipart-max-memory` by default |
| `WithRouter(r)` | the router the routes go on, one of the framework's |

## Validation

```yaml
server:
  validation:
    request: true    # opts.Validate() before the service is called; 400 on failure
    response: true   # the body's ValidateResponse or Validate before it is written; 500 on failure
```

`server.validation.response` also turns on `models.validation.response`, so the response types
get their `ValidateResponse` methods.

## Errors

The handlers pass a `*HandlerError` to the error handler when a request cannot be served. The
generated package aliases the runtime's types, so a project names them without importing the
runtime.

```go
type HandlerError struct {
	Kind          ErrorKind // ErrorParse, ErrorDecode, ErrorValidation, ErrorService, ErrorResponse
	OperationID   string
	ParamName     string
	ParamLocation string
	Status        int       // 0 for the status the kind implies
	Err           error
}

type ErrorHandler interface {
	HandleError(w http.ResponseWriter, r *http.Request, status int, err error)
}
```

| Kind | When | Status |
|---|---|---|
| `ErrorParse` | a parameter cannot be read | 400 |
| `ErrorDecode` | the body cannot be read, or has a media type the operation does not take | 400, 415 |
| `ErrorValidation` | the request fails the checks of the spec | 400 |
| `ErrorService` | the service returned an error, or no response | 500 |
| `ErrorResponse` | the response fails the checks of the spec | 500 |

`DefaultErrorHandler` writes `{"error": "..."}` when the request accepts JSON, and plain text
otherwise. An error type of the spec is written as its own JSON. `ErrorHandlerFunc` turns a
function into an `ErrorHandler`:

```go
NewRouter(svc, WithErrorHandler(ErrorHandlerFunc(func(w http.ResponseWriter, r *http.Request, status int, err error) {
	var herr *HandlerError
	if errors.As(err, &herr) {
		slog.WarnContext(r.Context(), "request failed", "operation", herr.OperationID, "kind", herr.Kind)
	}
	http.Error(w, http.StatusText(status), status)
})))
```

## Router

`NewRouter` registers every operation on a router of the framework `server.framework` names:

| Framework | Router | `NewRouter` returns | `WithRouter` takes |
|---|---|---|---|
| `chi` | github.com/go-chi/chi/v5 | `chi.Router` | `chi.Router` |
| `std-http` | `http.ServeMux` with the patterns of Go 1.22 | `http.Handler` | `*http.ServeMux` |

```go
router := NewRouter(svc, WithMiddleware(RequestIDMiddleware, RecoverMiddleware))
http.ListenAndServe(":8080", router)
```

On a new router the middleware wraps everything, unknown paths included, so a CORS preflight is
answered. With `WithRouter(existing)` the routes are registered on the given router and the
middleware wraps those routes only: in a group of a chi router, around each handler on a
`ServeMux`.

An operation the router cannot serve is left out with a `route-dropped` warning: a method the
router does not take, a path it rejects, or a route it cannot hold next to an earlier one.

### chi

Routes keep the spec's `{name}` parameters and a trailing `*`. chi rejects a path without a
leading slash, an unclosed brace, a parameter named twice and `*` not last. A route that repeats
an earlier one, or whose path parameters are named otherwise than an earlier route of the same
shape, is dropped, since chi keys parameters by position.

### std-http

Routes are `ServeMux` patterns such as `GET /pets/{id}`, and the handlers read parameters with
`r.PathValue`. A parameter name that is no Go identifier is written with underscores, `{pet-id}`
as `{pet_id}`. A trailing `*` becomes `{rest...}`, which takes the rest of the path, and a trailing
slash becomes `{$}`, so `/pets/` matches itself alone. `ServeMux` rejects a path without a leading
slash, one that is not clean (`/a//b`, `/a/../b`), a parameter that does not fill its segment
(`{id}.json`) and two wildcards of one name. It panics on a route that matches the same requests as
an earlier one, or overlaps with it while neither is more specific, `/a/{x}` next to `/{y}/b`; the
generator drops such routes by the same rules, so `NewRouter` never panics. A `GET` route answers
`HEAD` requests too.

## Scaffolds

```yaml
server:
  scaffold:
    service: ./service.go           # a Service struct with a stub per operation
    middleware: ./middleware.go     # request id, recovery, logging, CORS, timeout
    main: ./cmd/server/main.go      # a program that serves with graceful shutdown
    port: 8080
    timeout: 30s
    overwrite: false                # write them again when they exist
```

Scaffolds are written once and never overwritten unless `overwrite` is set, so they are the place
for the project's own code. The struct of the service scaffold is named after `server.name`
(`Service` by default) and its stubs return `ErrNotImplemented`. `main` needs `service`; it lives
in a folder of its own, since it is `package main`, and imports the others by module path.

## Runtime codecs

The runtime package holds what the generated HTTP code and clients use, standard library only:

- Parameters: `DecodePath`, `DecodeQuery`, `DecodeHeader`, `DecodeCookie` and their `Encode`
  counterparts handle every style of the spec (`simple`, `label`, `matrix`, `form`,
  `spaceDelimited`, `pipeDelimited`, `deepObject`), exploded or not, for values, lists and objects,
  and parameters with JSON content.
- Bodies: `DecodeJSON`, `DecodeForm` (bracketed keys nest: `address[city]=Berlin`,
  `items[0]=a`), `DecodeMultipart` (files as `runtime.File`, JSON parts into structs),
  `DecodeText`, `DecodeBytes`. A required body that is empty gives `ErrBodyEmpty`; an empty
  optional one is left alone.
- Responses: `Write` sends a status, headers and a body: JSON for most values, text and bytes as
  they are, a `File` streamed.
- Clients: `RequestBuilder`, `EncodeForm`, `EncodeMultipart`, `Send`, `Decode`, `DecodeSuccess`,
  `DecodeHeaders` and `APIError`, see [client](client.md#runtime).
