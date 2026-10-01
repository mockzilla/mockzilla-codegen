# Server

A `server` block in the config generates the service contract, an interface with one method per
operation, a request options type each method receives and a response data type each returns, and
the HTTP side: an adapter that decodes requests and calls the service, the error types, a router
for the framework the block names, and starter files on request.

```yaml
server:
  framework: chi   # or one of the routers below
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

`HTTPAdapter` turns requests into calls of the service, one handler method per operation, in
the shape the framework takes: a `func(echo.Context) error` for echo and a `func(khttp.Context)
error` for kratos, an `http.HandlerFunc` for every other framework. Each handler reads the parameters of every location with the
runtime codecs, decodes the body by the request's `Content-Type`, validates the options when the
config asks for it, calls the service and writes what it returns.

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
| `beego` | github.com/beego/beego/v2 | `*web.ControllerRegister` | `*web.ControllerRegister` |
| `chi` | github.com/go-chi/chi/v5 | `chi.Router` | `chi.Router` |
| `echo` | github.com/labstack/echo/v4 | `*echo.Echo` | `*echo.Echo` |
| `echo-v5` | github.com/labstack/echo/v5 | `*echo.Echo` | `*echo.Echo` |
| `fasthttp` | github.com/fasthttp/router over github.com/valyala/fasthttp | `*router.Router` | `*router.Router` |
| `fiber` | github.com/gofiber/fiber/v3 | `*fiber.App` | `*fiber.App` |
| `gin` | github.com/gin-gonic/gin | `*gin.Engine` | `*gin.Engine` |
| `go-zero` | github.com/zeromicro/go-zero/rest | `httpx.Router` | `httpx.Router` |
| `goframe` | github.com/gogf/gf/v2 | `*ghttp.Server` | `*ghttp.Server` |
| `gorilla-mux` | github.com/gorilla/mux | `*mux.Router` | `*mux.Router` |
| `hertz` | github.com/cloudwego/hertz | `*server.Hertz` | `*server.Hertz` |
| `iris` | github.com/kataras/iris/v12 | `*iris.Application` | `*iris.Application` |
| `kratos` | github.com/go-kratos/kratos/v2/transport/http | `*http.Server` of kratos | `*http.Server` of kratos |
| `std-http` | `http.ServeMux` with the patterns of Go 1.22 | `http.Handler` | `*http.ServeMux` |

```go
router := NewRouter(svc, WithMiddleware(RequestIDMiddleware, RecoverMiddleware))
http.ListenAndServe(":8080", router)
```

On a new router the middleware wraps everything, unknown paths included, so a CORS preflight is
answered. With `WithRouter(existing)` the routes are registered on the given router and the
middleware wraps those routes only: in a group of a chi router, around each handler on a
`ServeMux`, as the middleware of each route on an Echo, around each handler on the others.

Most routers take the handlers as `http.HandlerFunc`s, straight or through a small function of
the generated router that serves one from the framework's context and puts the path parameters
on the request, where `r.PathValue` reads them. A new router of those answers a request no route
takes with the standard library's 404 through the middleware, so a logging or CORS middleware
sees it; a router given with `WithRouter` keeps its own answer. Echo and kratos take handlers of
their own shape instead, see below.

An operation the router cannot serve is left out with a `route-dropped` warning: a method the
router does not take, a path it rejects, or a route it cannot hold next to an earlier one. Every
router takes `GET`, `PUT`, `POST`, `DELETE`, `OPTIONS`, `HEAD`, `PATCH` and `TRACE`; std-http
takes any other method too, such as `QUERY` and the `additionalOperations` of OpenAPI 3.2. Every
router rejects a path without a leading slash, an unclosed brace, a parameter without a name or
named twice, and a `*` that is not a segment of its own at the end; the sections below name what
each rejects on top, and how each writes its routes.

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

### echo

Routes write parameters as echo does, `/pets/{id}` as `/pets/:id`, and the handlers read them
with `c.Param`. A parameter runs to the end of its segment on echo, so a literal prefix is fine,
`/pets/v{id}`, while a suffix, `{id}.json`, or two parameters in one segment are rejected. A
literal colon is escaped, `/pets:search` as `/pets\:search`, since echo reads a colon as the
start of a parameter. A trailing `*` stays and takes the rest of the path. Echo also rejects a
path without a leading slash, an unclosed brace, a parameter without a name or named twice and `*`
not last. It replaces an earlier route with a later one of the same method and shape without a
word, so the generator drops the later one, as for chi.

The handlers have echo's shape, `func (a *HTTPAdapter) ListPets(c echo.Context) error`, and
write their response through `c.Response()`, so echo's own middleware sees the status. They
return nil: the error handler writes every failed request, so the responses of an operation are
the same under every framework. `WithMiddleware` still takes `func(http.Handler) http.Handler`,
wrapped with `echo.WrapMiddleware`; echo middleware goes on the returned `*echo.Echo` with `Use`,
or on the Echo `WithRouter` gives. On a new Echo with middleware, an error a handler returns, such
as echo's own for an unknown path, is given to echo's error handler inside the middleware, so a
logging or timeout middleware sees the 404 it writes.

### echo-v5

As echo, with the routes and handlers of echo v5: `func (a *HTTPAdapter) ListPets(c *echo.Context)
error`, and an error a handler returns given to `c.Echo().HTTPErrorHandler` inside the
middleware. Echo v5 keeps a route of each shape too, and replaces an earlier one without a word,
so the generator drops the later one.

### gin

Routes write parameters as gin does, `/pets/{id}` as `/pets/:id`, and a trailing `*` as `/*rest`,
the catch-all gin asks a name for. A parameter runs to the end of its segment, so a literal prefix
is fine, `/pets/v{id}`, while a suffix or two parameters in one segment are rejected, and so is a
literal colon or star, which gin reads as the start of a parameter and cannot escape. Gin panics
on a route that names the parameter at some position otherwise than an earlier route, `/pets/{id}`
next to `/pets/{petId}/photos`, and on anything next to a catch-all, `/files/*` next to `/files/x`;
the generator drops such routes by the same rules. A new Engine comes from `gin.New()`, without
gin's logger and recovery; gin middleware goes on an Engine before `WithRouter` gives it, since
gin applies `Use` to the routes registered after it. A wrong method is a 404 on gin.

### gorilla-mux

Routes keep the spec's `{name}` parameters, with a trailing `*` as `{rest:.*}`, and the handlers
read them with `mux.Vars`. A name that holds a colon is rejected, since mux reads what follows as
a pattern. Mux takes the first route that matches, so the generator drops a route that repeats
the shape of an earlier one and registers literals before parameters and parameters before the
wildcard at each position, whatever the order of the spec. A wrong method is a 405 without a
body.

### fiber

Routes write parameters as fiber does, `/pets/{id}` as `/pets/:id`, with a name that is an
identifier, `{pet-id}` as `:pet_id`, since fiber ends a name at any other character; a literal
colon, star, plus or question mark is escaped with a backslash. A parameter runs to the end of its
segment, so a prefix is fine and a suffix or two parameters in one segment are rejected. Fiber
does not tell `/pets` from `/pets/` and takes the first route that matches, so the generator
drops a route that matches the same requests as an earlier one and registers literals before
parameters and parameters before the wildcard. The handlers are served through fasthttp's
adaptor: the strings of the request live for the request alone, so a service that keeps a
parameter or header beyond it copies the string first. On a new App a request no route takes,
by path or by method, is a 404 through the middleware. The main scaffold serves with
`app.Listen` and stops with `app.ShutdownWithTimeout`.

### fasthttp

Routes keep the spec's `{name}` parameters, with a trailing `*` as `{rest:*}`, on a router of
github.com/fasthttp/router, and the handlers are served through fasthttp's adaptor, so the strings
of the request live for the request alone, as for fiber. Two parameters with nothing between them
and a name that holds a colon are rejected. The router panics on a route that matches the same
requests as an earlier one, which the trailing slash does not tell apart, on the parent of an
earlier catch-all, `/files` after `/files/*`, and on a route that differs from an earlier one in a
single segment where both have a parameter after the same literal, `/pets/{id}.json` next to
`/pets/{id}.xml`; the generator drops such routes by the same rules. A new router leaves OPTIONS
requests to the routes and the middleware, and answers a wrong method with a 405 and the Allow
header through the middleware. The main scaffold serves with a `fasthttp.Server`.

### hertz

Routes write parameters as hertz does, as gin: `/pets/{id}` as `/pets/:id`, a trailing `*` as
`/*rest`, a prefix allowed and a suffix, two parameters in one segment or a literal colon or star
rejected. Hertz panics on a route that repeats the shape of an earlier one, so the generator drops
it. The handlers are served from hertz's request context: the request is read into an
`http.Request` and the response written into hertz's, so `ut.PerformRequest` drives the router in
tests. A wrong method is a 404 on a new server. The main scaffold builds the server with
`server.New` and `WithHostPorts`, serves with `h.Run` and stops with `h.Shutdown`.

### beego

Routes write parameters as beego does, `/pets/{id}` as `/pets/:id`, with a name that is an
identifier since beego ends a name at any other character, and a trailing `*` stays. A parameter
fills its segment, so a prefix or a suffix is rejected, and so is a literal colon, star or
question mark, which beego reads as the start of a parameter. Beego holds one route of a shape
and takes literals before parameters on its own; the generator drops a route that repeats the
shape of an earlier one. Beego reads a form body before the handler, so the generated router
puts the form back into the body. A new `ControllerRegister` answers a request no route takes,
by path or by method, with a 404 through the middleware, on a route for `/*`.

### goframe

Routes are GoFrame patterns such as `GET:/pets/{id}`, with each parameter as a field whose name is
an identifier, a prefix or a suffix allowed, `/pets/{id}.json`, and a trailing `*` as `/*rest`.
A trailing slash is dropped, since GoFrame drops it, and a literal colon or at sign is rejected,
which GoFrame reads as the start of a parameter or of a domain. GoFrame exits the process on a
route registered twice, so the generator drops a route that matches the same requests as an
earlier one. The routes are bound when the server starts, so a test starts it on a free port, as
the examples do. The handlers write past GoFrame's buffer, so a status without a body stays
without one. A new server is named after a fresh id, since GoFrame keeps its servers by name;
`WithRouter(ghttp.GetServer())` uses the default one, which the config file sets up, as the main
scaffold does before it serves with `s.Start` and stops with `s.Shutdown`.

### go-zero

Routes write parameters as go-zero does, `/pets/{id}` as `/pets/:id`, on the router of the rest
package, which `rest.WithRouter` gives a rest server; the handlers read them with `pathvar.Vars`.
A parameter fills its segment, so a prefix or a suffix is rejected, and so are a wildcard, which
go-zero has none of, a path that is not clean and a literal segment that begins with a colon. A
trailing slash is dropped, since go-zero does not tell it apart, and a route that then matches the
same requests as an earlier one is dropped, which go-zero refuses as a duplicate. `NewRouter`
returns `httpx.Router`; a new one is served through the middleware as a whole, and a wrong method
is a 405 with the Allow header.

### iris

Routes keep the spec's `{name}` parameters, with a name that is an identifier since iris takes no
other, and a trailing `*` as `{rest:path}`. A parameter fills its segment, so a prefix or a suffix
is rejected. Iris holds one route of a shape and takes literals before parameters on its own; the
generator drops a route that repeats the shape of an earlier one. A new Application is built by
`NewRouter`, ready to serve, keeps the status a handler writes as it is and answers a request no
route takes with an empty 404, through the middleware, which wraps the router as a whole. An
Application given with `WithRouter` is left for its owner to build, with `app.Build` or
`app.Listen`.

### kratos

Routes keep the spec's `{name}` parameters, with a trailing `*` as `{rest:.*}`, on the HTTP
transport of kratos, which routes with gorilla/mux; the handlers have kratos's shape,
`func (a *HTTPAdapter) ListPets(c khttp.Context) error`, and read the parameters with
`c.Vars()`. A path that is not clean, such as one with a trailing slash, is rejected, since kratos
cleans it without a word, and so is a name that holds a colon. As for mux, the generator drops a
route that repeats the shape of an earlier one and registers literals first. The middleware wraps
a new server as its filters, and each route as filters of its own on a server `WithRouter` gives;
kratos middleware goes on the server with `Use`. A new server listens on kratos's default address
and times requests out after its default second: `WithRouter(khttp.NewServer(khttp.Address(...),
khttp.Timeout(...)))` sets both.

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
in a folder of its own, since it is `package main`, and imports the others by module path. It
serves with an `http.Server`, except for fiber, fasthttp, hertz and goframe, whose servers are
served in their own way, see their sections.

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
