# Server

A `server` block in the config generates:

- the service contract: an interface with one method per operation
- a request options type that each method receives
- a response data type that each method returns
- an adapter that decodes requests and calls the service
- the error types
- a router for the framework the block names
- starter files, on request

```yaml
server:
  framework: chi   # or one of the routers below
  name: Pets       # the interface is PetsInterface; defaults to Service
```

## Service interface

```go
type PetsInterface interface {
	// ListPets handles GET /pets.
	// List pets
	ListPets(ctx context.Context, opts *ListPetsServiceRequestOptions) (*ListPetsResponseData, error)
	// CreatePet handles POST /pets.
	CreatePet(ctx context.Context, opts *CreatePetServiceRequestOptions) (*CreatePetResponseData, error)
}
```

Every operation has the same shape, even one without parameters or body. Middleware and wrappers
can treat all operations alike.

The method's comment names the HTTP method and the path, then the operation's summary and
description. `models.descriptions: false` leaves out the summary and the description.

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

| Field | Holds |
|---|---|
| `PathParams`, `Query`, `Headers`, `Cookies` | the parameters of one location, one struct per location the operation uses |
| named after the parameter, such as `Filter` | a `querystring` parameter, see below |
| `Body` | the request body |
| `RawRequest` | the request as it came in |

`Validate` calls the `Validate` of each parameter struct and of the body, when they have one. Its
errors get the paths `path`, `query`, `header`, `cookie`, `querystring` and `body`.

### Bodies

An operation with several body media types gets one field per media type, named after it:
`BodyJSON`, `BodyForm`, `BodyMultipart`, `BodyText`. When two media types would get the same name,
the full type tells them apart: `BodyXML` for `application/xml`, `BodyTextXML` for `text/xml`.

A body without a schema is:

| Media type | Go type |
|---|---|
| JSON | `any` |
| `text/*` | `string` |
| anything else | `[]byte` |

### querystring

A `querystring` parameter (OpenAPI 3.2) is the whole query in one media type. Its field is named
after it and typed by its schema: `Filter *SearchFilter`.

| Media type | The query is read as |
|---|---|
| a form | form values |
| JSON | percent-encoded JSON text |
| any other | no field |

A second `querystring` parameter, or one next to query parameters, gets no field either. OpenAPI
does not allow them. Generation warns (`querystring-unsupported`).

### Nested values

OpenAPI leaves values nested deeper than one level to the implementation. These work:

| Style | Example |
|---|---|
| `deepObject`, any value at any depth | `filter[size][x]=1`, `filter[tags][0]=a`, `expand[0]=a` |
| exploded `form` query object or `cookie` object, a list of values inside | `reference=r&status=a&status=b` |

Any other nesting, such as a list of lists or an object inside a `simple` object, gets no field.

OpenAPI defines `deepObject` for an object of scalars only. Lists, nesting and single values follow
[the bracket form](client.md#deepobject-past-an-object) of the client. The server also reads `[]`
and a repeated bare name: `expand[]=a`, `expand=a&expand=b`.

### Parameters without a field

Some parameters cannot be read, so they get no field. Generation warns about each one.

| Parameter | Warning |
|---|---|
| a query, header or cookie parameter whose union has an object or array variant, or a list or map of such unions. No style writes it as text. | `param-unsupported` |
| a parameter or response header in a style the OpenAPI style table does not define for its location or value. See below. | `param-unsupported` |
| a form-style cookie that holds a list or object with `explode: true`. OpenAPI says it writes the wrong delimiter for cookies. `style: cookie` writes it. | `param-unsupported` |
| nesting the table above does not list | `param-unsupported` |
| a parameter without `in`, or with an `in` other than `path`, `query`, `header`, `cookie` and `querystring` | `param-in` |
| a path parameter whose `{name}` is not in the path. Nothing could fill it. | `path-param-unused` |
| a parameter or response header named `""` | `name-empty` |

Styles the table does not define:

- `matrix` in a query
- any style but `simple` on a header
- `pipeDelimited` with `explode: true`
- a style OpenAPI does not name

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

### Constructors

Each response the spec documents gets one constructor.

- When the operation has several responses, each constructor is named after its status.
- A range such as `4XX`, and `default`, take the status as their first argument.
- A response without a body takes no body.
- A response under a key that is not a status, a range or `default`, such as `"401 "`, is left
  out. Generation warns (`invalid-status`).

### Body and content type

The body is the JSON media type of the response. If there is none, it is the first media type. The
content type is remembered and written with the response.

A range is written as a type inside it:

| Range | Written as |
|---|---|
| `text/*` | `text/plain` |
| `multipart/*` | `multipart/form-data` |

A `runtime.File` is written in its own type. Any other range, such as `*/*`, sets no content type.
The Go type of the body picks it.

### Streams

A response in a sequential media type takes its frames as an `iter.Seq` of the frame type.
Sequential media types are `text/event-stream` and line-delimited JSON types such as
`application/x-ndjson`.

When the response also has a body that is read whole, the frames constructor has the suffix
`Stream`:

```go
func NewChatResponseData200(body *Reply) *ChatResponseData
func NewChatResponseData200Stream(frames iter.Seq[Chunk]) *ChatResponseData

return NewChatResponseData200Stream(func(yield func(Chunk) bool) {
	for _, word := range words {
		if !yield(Chunk{Text: word}) {
			return // the client is gone
		}
	}
}), nil
```

### Response headers

A response that declares headers gets a struct for them, `ListPetsResponse200Headers`, and a
`WithTypedHeaders` method that adds them. When several responses declare headers, the method
carries the status, as in `WithTypedHeaders200`.

A response under `components.responses` gets one struct for every operation that uses it, named
after the component: `UnauthorizedHeaders`.

Two header settings add a tag to the field, so the runtime writes and reads the header that way:

| Header | Tag |
|---|---|
| `explode: true` | `header:"explode"` |
| JSON content | `header:"json"` |

A service sets the headers like this:

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

`HTTPAdapter` turns requests into calls of the service. It has one handler method per operation.

```go
adapter := NewHTTPAdapter(svc, opts...)
mux.HandleFunc("GET /pets", adapter.ListPets)
```

A handler has the shape the framework takes:

| Framework | Handler |
|---|---|
| echo | `func(echo.Context) error` |
| kratos | `func(khttp.Context) error` |
| every other framework | `http.HandlerFunc` |

Each handler:

- reads the parameters of every location with the runtime codecs
- decodes the body by the request's `Content-Type`
- validates the options when the config asks for it
- calls the service
- writes what the service returns

### Parameter defaults

A query, header, cookie or querystring parameter that is not in the request takes the `default` of
its schema.

- The field stays a pointer, so the client still sends only what is set.
- Its getter, such as `GetLimit()`, returns the value or the default, see
  [defaults](types.md#defaults).
- A required parameter is never filled.
- A default that does not fit its schema is left out. Generation warns (`default-ignored`).

A property of a JSON, form or multipart body gets its default the same way, see
[request bodies](#request-bodies).

### Operation name

A handler first puts the name of its operation, `ListPets`, on the request's context. The service,
a wrapper of it and the error handler read it with `runtime.OperationID(ctx)`.

The router's middleware runs before the handler, so it does not see the name.

### Reading the body

A body arrives in a media type the operation documents:

| Media type | Read with |
|---|---|
| JSON (`application/json` and `+json`) | the JSON decoder |
| `application/x-www-form-urlencoded` | `DecodeForm` |
| `multipart/form-data`, into a struct or a union | `DecodeMultipart` |
| any other media type | a `runtime.File`, a string or bytes, whichever its field is |
| a text media type, for a number, a boolean, a time or an `any` | `DecodeTextValue`, from its text, the way the server writes them |

- A `runtime.File` (`format: binary`) streams the body. The service reads it once, before it
  returns.
- A documented media type that does not fit its type, such as XML into a struct, is accepted and
  left to `RawRequest`. The generator warns (`server-body-unread`).
- Content under an empty key names no media type, so it is left out. Generation warns
  (`media-type-empty`).
- A required body that is missing is a 400. A missing optional body leaves its field nil.

### Form bodies

A form body follows its `encoding` object as the client writes it, see
[the client](client.md#the-encoding-object).

- A property with a style is read as a query parameter of that style. `tags=a,b%2Cc` under
  `style: form` is `a` and `b,c`.
- A property without one is read from JSON or from brackets, whichever came:
  `address={"city":"Rome"}` and `address[city]=Rome` give the same value.
- A property declared JSON is read as JSON. Text that is not JSON is a 400.
- A list of a multipart form is read from one part per item, or from one part that holds the whole
  list as JSON.
- A part sent as a file, as browsers send a Blob, is read as the text of a property that holds no
  file.

### Matching media types

Media types are matched without their parameters and in lower case.

A key with a `*` takes what the operation does not name:

| Key | Takes |
|---|---|
| `application/*+json` | any JSON media type: `application/json` and every `+json` one |
| a range of one type, such as `text/*` | the media types of its type, read as its member: `text/*` as `text/plain`, `multipart/*` as `multipart/form-data` |
| `*/*`, or another key with a `*` | every media type left, as JSON unless its field is a file, a string or bytes |

A media type nothing takes is answered with 415.

### Errors from the service

A service can return an error type of the spec (see `models.error-mapping`), as a value, a pointer
or wrapped. The adapter answers with:

- the status of the first response that carries the type
- the media type of that response, as JSON under a range
- the error as the body

The adapter sets the `Content-Type` before it calls the error handler.

Other results of the service:

| The service returns | Answer |
|---|---|
| an error that wraps `context.DeadlineExceeded`, such as from a service that gave up when the request's context ran out | 503 |
| any other error | 500, and the message does not reach the client |
| nil for both values | 500 with `ErrNoResponse` |

### Writing the response

The response is written with its status, headers and content type. The body is written by its
media type:

| Media type | Body written as |
|---|---|
| JSON | JSON, a string too |
| a form | url-encoded values |
| `multipart/form-data` | a form of the struct |
| a sequential media type | one frame per value, flushed as it goes. Each line of a frame is a `data:` line under `text/event-stream` |
| `text/*`, for a number or a boolean | its text |
| every other media type, for a string, bytes or a `runtime.File` | as they are |

A nil body sends the status alone.

A body the server has no encoder for, such as a struct under `application/xml`, is a 500 of kind
`ErrorResponse`. The generator warns (`server-body-unwritable`). To send XML or YAML, encode it
yourself and set `Body` to a string, bytes or a `runtime.File`.

### Options

Options are set with `ServerOption` functions on the adapter and on the router alike:

| Option | Sets |
|---|---|
| `WithMiddleware(mw...)` | `func(http.Handler) http.Handler` wrappers, outermost first |
| `WithErrorHandler(h)` | what writes failed requests, `DefaultErrorHandler{}` by default |
| `WithJSONDecoder(fn)` | what reads JSON bodies, `runtime.DecodeJSON` by default |
| `WithPresence(p)` | what checks the keys of request bodies and fills their defaults, see [request bodies](#request-bodies) |
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

### Request bodies

A decoded struct cannot tell a missing key from an empty value, or `null` from a missing key. So
the adapter reads a JSON, form or multipart body once more before it decodes it.

With `validation.request`, it answers these bodies of `Pet` with a 400 of kind `ErrorValidation`:

| Body | Error |
|---|---|
| `{"owner":{}}` | `body.name: is required; body.owner.id: is required` |
| `{"name":null,"owner":{"id":1}}` | `body.name: must not be null` |
| `{"name":"Rex","owner":{"id":1},"tags":["a",null]}` | `body.tags[1]: must not be null` |
| `null` | `body: must not be null` |
| `{"name":"Rex","owner":{"id":1},"x":1}` | `body.x: is not allowed` |

The checks:

- A required key must be there. An empty value passes. A `readOnly` property is not required in
  a request.
- `null` fails where the schema does not allow it: in a property, a list item, a map value or the
  whole body. A schema without a `type`, such as `{}`, allows it.
- A key that is not a property fails when the object has `additionalProperties: false`.
- A form has no `null`. Its fields are checked for presence and unknown names, and a file part
  counts as its field. A field that holds a JSON object or array is checked as JSON.
- Unions are not looked into. Decoding picks their variant by its required keys.

#### Defaults

A missing optional property with a `default` gets it. This happens with or without
`validation.request`: in the body, in nested objects and in list items that are sent.

- An object that is not sent is not built for its defaults.
- A sent `null` stays `null`.
- The field stays a pointer.
- A required or `readOnly` property gets no default.
- A default that does not fit its schema is left out. Generation warns (`default-ignored`).

#### The presence table

The adapter keeps what it checks in one table, `bodyPresence`. It checks a body before the JSON
decoder or `DecodeForm` reads it.

The table holds facts about keys only:

- which keys are required
- which may be null
- which are unknown
- which get a default

Value rules, such as `minLength` or `pattern`, stay in `Validate`.

```go
var bodyPresence = runtime.Presence{
	IsChecked: true,
	Objects: []runtime.Object{
		{Name: "Owner", Props: []runtime.Prop{
			{Key: "city", Default: `"Berlin"`},
			{Key: "id", IsRequired: true},
		}},
		{Name: "Pet", IsClosed: true, Props: []runtime.Prop{
			{Key: "age", Default: "1"},
			{Key: "name", IsRequired: true},
			{Key: "owner", IsRequired: true, Object: "Owner"},
			{Key: "tag", IsNullable: true},
			{Key: "tags", Items: &runtime.Prop{}},
		}},
	},
}
```

Objects are sorted by name and properties by key. The runtime finds them by binary search. The
table is a slice, not a map, so it is plain data with no code that runs at start. A spec with
thousands of body properties compiles about as fast as without the table.

Without `validation.request`, the table holds only the objects that lead to a default. With no
default either, nothing is generated. `examples/bodies/checked` and `examples/bodies/defaults` show
both.

#### WithPresence

`WithPresence` replaces the table with any `runtime.PresenceChecker`.
`WithPresence(runtime.Presence{})` turns the checks and the defaults off. The option exists only
when the server has a table.

A wrapper can log or skip what the table finds:

```go
type presence struct{ next runtime.PresenceChecker }

func (p presence) JSON(body io.Reader, prop runtime.Prop) (io.Reader, error) {
	out, err := p.next.JSON(body, prop)
	if err != nil {
		slog.Info("body rejected", "err", err)
	}
	return out, err
}
```

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
	Body          error     // the error type of the spec that answers a request turned away
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
| `ErrorService` | the service returned an error, or no response; on fiber and fasthttp also a panic, wrapping `ErrPanic` | 500, or 503 for `context.DeadlineExceeded` |
| `ErrorResponse` | the response fails the checks of the spec, or has no encoder for its media type | 500 |

### DefaultErrorHandler

`DefaultErrorHandler` writes `{"error": "..."}` when the request accepts JSON, and plain text
otherwise.

- An error type of the spec is written as its own JSON, under the JSON media type its response
  documents, such as `application/problem+json`.
- An error that wraps `ErrResponseCut` happened once the status was out, such as a client that
  left a stream. It gets nothing more.

### Error types of the spec

When the handler turns a request away, with a kind of `ErrorParse`, `ErrorDecode` or
`ErrorValidation`, the answer is the error type its operation documents for the status. The adapter
looks for it:

1. under the status code
2. else under its range, such as `4XX`
3. else under `default`

The type must be in `models.error-mapping` and have a constructor. The adapter builds it with the
message, puts it in `Body` and sets the media type of its response. `DefaultErrorHandler` writes it:

```
PUT /pets/1 {"name": ""}

400 application/problem+json
{"detail":"invalid request: body.name: must be at least 1 characters long"}
```

- The other fields of the type stay empty. The generator warns when the type requires one
  (`error-mapping`).
- The generator warns about a documented type without a constructor too, such as one whose message
  is inside a union. Those requests get `{"error": "..."}`.
- A 415 takes only a type documented under 415, `4XX` or `default`.
- A handler set with `WithErrorHandler` gets `Body` and writes what it likes.

### Your own error handler

`ErrorHandlerFunc` turns a function into an `ErrorHandler`:

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
| `std-http` | `http.ServeMux` with method and wildcard patterns | `http.Handler` | `*http.ServeMux` |

A router that is an `http.Handler` serves as one. That is every router but fiber, fasthttp, hertz
and goframe:

```go
router := NewRouter(svc, WithMiddleware(RequestIDMiddleware, RecoverMiddleware))
http.ListenAndServe(":8080", router)
```

The other four serve in their own way, see their sections and the main scaffold.

### Middleware

On a new router, the middleware wraps everything, unknown paths included. So a CORS preflight is
answered.

With `WithRouter(existing)`, the routes are registered on the given router, and the middleware
wraps those routes only:

| Router | The middleware wraps |
|---|---|
| chi | the routes, in a group |
| `ServeMux` | each handler |
| Echo | each route, as its middleware |
| the others | each handler |

A router can make a redirect on its own, for a trailing slash or a path it cleans. On gin,
gorilla-mux, kratos, iris, fasthttp and hertz, that redirect is answered before the middleware.

### Handlers

Most routers take the handlers as `http.HandlerFunc`s: some directly, the others through a small
function of the generated router. That function serves the handler from the framework's context
and puts the path parameters on the request, where `r.PathValue` reads them.

A new router of those answers a request no route takes with the standard library's 404, through
the middleware. So a logging or CORS middleware sees it. A router given with `WithRouter` keeps its
own answer.

Echo and kratos take handlers of their own shape instead, see below.

### Path values

On every router, a path value reaches the service unescaped once:

| Request path | Value |
|---|---|
| `/pets/john%40example.com` | `john@example.com` |
| `/pets/100%25` | `100%` |

A `+` stays a `+`.

An escaped slash, `a%2Fb`, is part of the value, `a/b`, on chi, echo, echo-v5, fasthttp, fiber,
goframe and std-http. The other routers match the path with it unescaped, so the request finds no
route and is a 404.

### Dropped routes

An operation the router cannot serve is left out with a `route-dropped` warning. This happens
when:

- the router does not take the method
- the router rejects the path
- the router cannot hold the route next to an earlier one

#### Methods

| Method | Routers that take it |
|---|---|
| `GET`, `PUT`, `POST`, `DELETE`, `OPTIONS`, `HEAD`, `PATCH` | every router |
| `TRACE` | all but go-zero |
| any other method, such as `QUERY` and the `additionalOperations` of OpenAPI 3.2 | std-http |

#### Paths

Every router rejects:

- a path without a leading slash
- an unclosed brace
- a parameter without a name, or named twice

Every router but std-http also rejects a `*` that is not last.

#### Parameter names

A parameter name a router cannot hold is written for it:

- with underscores for a colon or a star, which routers read as a pattern
- as an identifier, where the router takes no other

Two names it would write the same, `{pet-id}` and `{pet_id}`, are rejected.

### Differences between routers

The table shows what each router holds and how it answers. The sections below name the rest. The
`paths` example of each router runs these shapes against the real router.

| Framework | `v{id}` | `{id}.json` | `{name}.{ext}` | a literal `:` | `/a` and `/a/` | unknown path | wrong method | `HEAD` of a `GET` route | `%2F` in a value |
|---|---|---|---|---|---|---|---|---|---|
| `beego` | no | no | no | no | one route | 404 | 404 | 404 | 404 |
| `chi` | yes | yes | yes | yes | two routes | 404 | 405 | 405 | yes |
| `echo`, `echo-v5` | yes | no | no | not first in a segment | two routes | 404, JSON | 405, JSON | 405 | yes |
| `fasthttp` | yes | yes | yes | yes | one route, `/a/` redirects | 404 | 405 | 405 | yes |
| `fiber` | yes | no | no | yes | one route | 404 | 405 | 200 | yes |
| `gin` | yes | no | no | yes | two routes | 404 | 404 | 404 | 404 |
| `go-zero` | no | no | no | not first in a segment | one route | 404 | 405 | 405 | 404 |
| `goframe` | yes | yes | yes | no | one route | 404 | 404 | 404 | yes |
| `gorilla-mux` | yes | yes | yes | yes | two routes | 404 | 405 | 405 | 404 |
| `hertz` | yes | no | no | no | two routes | 404 | 404 | 404 | 404 |
| `iris` | no | no | no | not first in a segment | one route, `/a/` redirects | 404, empty | 404, empty | 404 | 404 |
| `kratos` | yes | yes | yes | yes | `/a` alone, `/a/` redirects | 404 | 405 | 405 | 404 |
| `std-http` | no | no | no | yes | two routes | 404 | 405 | 200 | yes |

Fiber, fasthttp and hertz do not end a request's context when the client goes away.

### chi

Routes keep the spec's `{name}` parameters and a trailing `*`.

- A colon in a name becomes an underscore, `{lat:lng}` as `{lat_lng}`, since chi reads what follows
  a colon as a pattern.
- chi rejects a path without a leading slash, an unclosed brace, a parameter named twice and a `*`
  that is not last.
- A route is dropped when it repeats an earlier one, or when its path parameters are named
  otherwise than an earlier route of the same shape. chi keys parameters by position.
- chi matches the escaped path when a request has one, so the handlers unescape each value with
  `runtime.UnescapePath`.

### std-http

Routes are `ServeMux` patterns such as `GET /pets/{id}`. The handlers read parameters with
`r.PathValue`.

| Spec | Pattern |
|---|---|
| a parameter name that is not a Go identifier, `{pet-id}` | underscores: `{pet_id}` |
| a `*` as the last segment | `{rest...}`, which takes the rest of the path |
| a `*` anywhere else | a literal star |
| a trailing slash | `{$}`, so `/pets/` matches itself alone |

The name in `{rest...}` gets an underscore, `{rest_...}`, for as long as a parameter of the path
has it.

`ServeMux` redirects a request for either kind of path without its last slash, such as `/files` for
`/files/*`. The redirect goes to the path with the slash and is a 307. It does not happen when a
route that does not end in `*` matches the request.

`ServeMux` rejects:

- a path without a leading slash
- a path that is not clean (`/a//b`, `/a/../b`)
- a parameter that does not fill its segment (`{id}.json`)
- two wildcards of one name

It panics on a route that matches the same requests as an earlier one, or overlaps with it while
neither is more specific, such as `/a/{x}` next to `/{y}/b`. The generator registers the routes on
a `ServeMux` of its own first and drops each one that panics there, so `NewRouter` never panics.

A `GET` route answers `HEAD` requests too.

### echo

Routes write parameters as echo does, `/pets/{id}` as `/pets/:id`. The handlers read them with
`c.Param`, unescaped with `runtime.UnescapePath`, since echo matches the escaped path when a
request has one.

A parameter runs to the end of its segment on echo:

| Spec path | Result |
|---|---|
| a literal prefix, `/pets/v{id}` | accepted |
| a suffix, `{id}.json`, or two parameters in one segment | rejected |
| a literal colon, `/pets:search` | escaped as `/pets\:search`, since echo reads a colon as the start of a parameter |
| a segment that begins with a colon, `/pets/:search` | rejected |
| a literal star, which echo reads as a wildcard | rejected |
| a trailing `*` | stays, and takes the rest of the path |

Echo also rejects a path without a leading slash, an unclosed brace, a parameter without a name or
named twice and `*` not last.

Echo replaces an earlier route with a later one of the same method and shape without a word. The
generator drops the later one, as for chi.

The handlers have echo's shape, `func (a *HTTPAdapter) ListPets(c echo.Context) error`.

- They write their response through `c.Response()`, so echo's own middleware sees the status.
- They return nil. The error handler writes every failed request, so the responses of an operation
  are the same under every framework.

Middleware:

- `WithMiddleware` still takes `func(http.Handler) http.Handler`, wrapped with
  `echo.WrapMiddleware`.
- Echo middleware goes on the returned `*echo.Echo` with `Use`, or on the Echo `WithRouter` gives.
- On a new Echo with middleware, an error a handler returns, such as echo's own for an unknown
  path, is given to echo's error handler inside the middleware. So a logging or timeout middleware
  sees the 404 it writes.

### echo-v5

As echo, with the routes and handlers of echo v5: `func (a *HTTPAdapter) ListPets(c *echo.Context)
error`, and an error a handler returns given to `c.Echo().HTTPErrorHandler` inside the
middleware. Echo v5 keeps a route of each shape too, and replaces an earlier one without a word,
so the generator drops the later one.

### gin

Routes write parameters as gin does:

| Spec path | Gin route |
|---|---|
| `/pets/{id}` | `/pets/:id` |
| a colon in a name | an underscore |
| a trailing `*` | `/*rest`, the catch-all gin asks a name for |
| a literal prefix, `/pets/v{id}` | accepted, since a parameter runs to the end of its segment |
| a suffix, or two parameters in one segment | rejected |
| a literal colon, `/pets:search` | escaped as `/pets\:search` |
| a literal star | rejected |
| a path that is not clean | rejected, since gin cleans it before it matches |

- Gin holds one name at a position for every route that shares it. A route that names the parameter
  there otherwise, `/pets/{petId}/photos` next to `/pets/{id}`, takes the earlier name in its
  pattern. The generated `handle` puts each value on the request under the spec's name.
- Gin panics on anything next to a catch-all, `/files/*` next to `/files/x`, and on a route that
  then matches the same requests as an earlier one. The generator drops such routes.
- `TRACE`, which gin has no function for, is registered with `e.Handle`.
- A new Engine comes from `gin.New()`, without gin's logger and recovery.
- Gin middleware goes on an Engine before `WithRouter` gives it, since gin applies `Use` to the
  routes registered after it.
- A wrong method is a 404 on gin.

### gorilla-mux

Routes keep the spec's `{name}` parameters. The handlers read them with `mux.Vars`.

- A colon in a name becomes an underscore.
- A trailing `*` becomes `{rest:.*}`.

Mux takes the first route that matches. So the generator drops a route that repeats the shape of an
earlier one, and registers the routes in the order mux needs, whatever the order of the spec. At
each position the order is:

1. a literal
2. a parameter next to a literal, the longer literal first
3. a parameter alone
4. the wildcard

So `/v1/{name}:getIamPolicy` comes before `/v1/{name}`, which would match its requests too.

A wrong method is a 405 without a body.

### fiber

Routes write parameters as fiber does, `/pets/{id}` as `/pets/:id`.

- A name is an identifier, `{pet-id}` as `:pet_id`, since fiber ends a name at any other
  character.
- A literal colon, star, plus or question mark is escaped with a backslash.
- A parameter runs to the end of its segment, so a prefix is fine and a suffix or two parameters in
  one segment are rejected.

Fiber does not tell `/pets` from `/pets/`, nor `/Pets` from `/pets`, and it takes the first route
that matches. So the generator drops a route that matches the same requests as an earlier one, and
registers literals before parameters and parameters before the wildcard.

How the handlers run:

- They are served through fasthttp's adaptor, from a copy of the request whose strings are its own.
  Fiber reuses the memory of a request for the next one on its connection.
- Fiber matches the escaped path, so the values are unescaped, unless the App sets `UnescapePath`.
- A panic in the service would end the process, so the router recovers and answers it with a 500
  through the error handler.

On a new App, a request no route takes is a 404, and a wrong method a 405, through the middleware.
Both are answered from the App's error handler, so a route added to the App after `NewRouter` is
served too.

`WithConfig(fiber.Config{...})` sets the config of the new App. The main scaffold makes the App
that way, with its read timeout, serves with `app.Listen` and stops with `app.ShutdownWithTimeout`.

### fasthttp

Routes keep the spec's `{name}` parameters, on a router of github.com/fasthttp/router.

- A colon in a name becomes an underscore.
- A trailing `*` becomes `{rest:*}`.
- Two parameters with nothing between them are rejected.

The router reads what follows a parameter in its segment as part of a regular expression, so the
generator quotes it: `/files/{name}.{ext}` as `/files/{name}\.{ext}`, `/products({id})` as
`/products({id}\)`.

The router panics on:

- a route that matches the same requests as an earlier one, which the trailing slash does not tell
  apart
- the parent of an earlier catch-all, `/files` after `/files/*`
- a route that differs from an earlier one in a single segment where both have a parameter after
  the same literal, `/pets/{id}.json` next to `/pets/{id}.xml`

The generator drops such routes by the same rules.

How the handlers run:

- They are served through fasthttp's adaptor as for fiber: from a copy of the request, with the
  values unescaped, and a panic answered with a 500.
- A new router leaves OPTIONS requests to the routes and the middleware.
- A new router answers a wrong method with a 405 and the Allow header, through the middleware.

The main scaffold serves with a `fasthttp.Server`.

### hertz

Routes write parameters as hertz does, in gin's syntax: `/pets/{id}` as `/pets/:id`.

- A colon in a name becomes an underscore.
- A trailing `*` becomes `/*rest`.
- A prefix is allowed.
- A suffix, two parameters in one segment, or a literal colon or star are rejected.

Hertz panics on a route that repeats the shape of an earlier one, so the generator drops it.
`TRACE`, which hertz has no function for, is registered with `h.Handle`.

How the handlers run:

- They are served from hertz's request context. The request is read into an `http.Request` and the
  response is written into hertz's, so `ut.PerformRequest` drives the router in tests.
- The writer is not an `http.Flusher`, and the body is sent when the handler returns.
- A wrong method is a 404 on a new server.

`WithConfig(server.WithHostPorts(...), ...)` sets the options of the new server. The main scaffold
makes it that way, serves with `h.Run` and stops with `h.Shutdown`.

### beego

Routes write parameters as beego does, `/pets/{id}` as `/pets/:id`.

- A name is an identifier, since beego ends a name at any other character.
- A trailing `*` stays.
- A parameter fills its segment, so a prefix or a suffix is rejected.
- A literal colon, star or question mark is rejected, since beego reads it as the start of a
  parameter.

Beego holds one route of a shape and takes literals before parameters on its own. It does not tell
`/pets` from `/pets/`. The generator drops a route that matches the same requests as an earlier
one.

How the router behaves:

- Beego reads the form body of a POST, PUT or PATCH before the handler, so the generated router
  puts the form back into the body.
- A new `ControllerRegister` answers a request no route takes, by path or by method, with a 404
  through the middleware, on a route for `/*`.
- Beego routes `/all.html` to the route of `/all`.

### goframe

Routes are GoFrame patterns such as `GET:/pets/{id}`.

- Each parameter is a field whose name is an identifier.
- A prefix or a suffix is allowed, `/pets/{id}.json`.
- A trailing `*` becomes `/*rest`.
- A trailing slash is dropped, since GoFrame drops it.

These are rejected:

- a literal colon or at sign, which GoFrame reads as the start of a parameter or of a domain
- a literal with `$`, `^`, `(`, `)`, `|`, `[`, `]`, `?` or a backslash, which GoFrame leaves as
  they are in the regular expression it makes of a path

GoFrame exits the process on a route registered twice, so the generator drops a route that matches
the same requests as an earlier one.

How the router behaves:

- GoFrame picks the route of an OPTIONS request by its `Access-Control-Request-Method` header, and
  of any request by an `X-Url-Path` header. The generated router answers such a request with a 404
  past the middleware, so a CORS preflight is still answered and no other operation runs.
- GoFrame unescapes a value twice, so the router reads the values from the escaped path and
  unescapes them once.
- A route for `/store/*` also takes `/store`.
- The routes are bound when the server starts, so a test starts it on a free port, as the examples
  do.
- The handlers write past GoFrame's buffer, so a status without a body stays without one.

A new server is named after a fresh id, since GoFrame keeps its servers by name.
`WithRouter(ghttp.GetServer())` uses the default one, which the config file sets up. The main
scaffold does that before it serves with `s.Start` and stops with `s.Shutdown`.

### go-zero

Routes write parameters as go-zero does, `/pets/{id}` as `/pets/:id`, on the router of the rest
package, which `rest.WithRouter` gives a rest server. The handlers read them with `pathvar.Vars`.

These are rejected:

- a parameter that does not fill its segment: a prefix or a suffix
- `TRACE`, which go-zero has no route for
- a wildcard, which go-zero has none of
- a path that is not clean
- a literal segment that begins with a colon

A trailing slash is dropped, since go-zero does not tell it apart. A route that then matches the
same requests as an earlier one is dropped too, which go-zero refuses as a duplicate.

`NewRouter` returns `httpx.Router`. A new one is served through the middleware as a whole, and a
wrong method is a 405 with the Allow header.

### iris

Routes keep the spec's `{name}` parameters.

- A name is an identifier, since iris takes no other.
- A trailing `*` becomes `{rest:path}`.
- A parameter fills its segment, so a prefix or a suffix is rejected.
- A literal segment that begins with a colon is rejected, since iris reads it as a parameter.
- A trailing slash is dropped, since iris redirects `/pets/` to `/pets`.

Iris holds one route of a shape and takes literals before parameters on its own. The generator
drops a route that matches the same requests as an earlier one.

A new Application is built by `NewRouter`, ready to serve.

- It keeps the status a handler writes as it is.
- It answers a request no route takes with an empty 404, through the middleware, which wraps the
  router as a whole.
- A route added to it after `NewRouter` is served once `app.RefreshRouter` has run.

An Application given with `WithRouter` is left for its owner to build, with `app.Build` or
`app.Listen`. It answers an unknown path with iris's own `Not Found`.

### kratos

Routes keep the spec's `{name}` parameters, on the HTTP transport of kratos, which routes with
gorilla/mux.

- A colon in a name becomes an underscore.
- A trailing `*` becomes `{rest:.*}`.
- The handlers have kratos's shape, `func (a *HTTPAdapter) ListPets(c khttp.Context) error`, and
  read the parameters with `c.Vars()`.
- A path that is not clean, such as one with a trailing slash, is rejected. Kratos cleans it
  without a word and redirects `/pets/` to `/pets`.

As for mux, the generator drops a route that repeats the shape of an earlier one and registers the
routes in the order mux needs.

Middleware:

- It wraps a new server as its filters.
- On a server `WithRouter` gives, it wraps each route as filters of its own.
- Kratos middleware goes on the server with `Use`.

A new server:

- listens on kratos's default address
- sets no timeout of its own, where kratos ends each request after a second
- answers a path no route takes with a 404 and a wrong method with a 405, where kratos would hand
  both to `http.DefaultServeMux`

`WithRouter(khttp.NewServer(khttp.Address(...), ...))` sets the address.

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

Scaffolds are written once. They are never overwritten unless `overwrite` is set, so they are the
place for the project's own code.

With `overwrite` set, `Generate` sets `File.IsOverwritten` on every scaffold and `Write` writes it
again.

- `service`: the struct is named after `server.name` (`Service` by default). Its stubs return
  `ErrNotImplemented`.
- `main`: it needs `service`. It lives in a folder of its own, since it is `package main`, and
  imports the others by module path.

`main` serves with an `http.Server`, except for fiber, fasthttp, hertz and goframe. Their servers
are served in their own way, see their sections.

### TimeoutMiddleware

`TimeoutMiddleware` sets a deadline on the request's context, `timeout` after the request came. It
serves the request on the same goroutine.

A service that watches its context and returns the context's error is answered with a 503. One
that does not runs to its end.

## Runtime codecs

The runtime package holds the codecs the generated HTTP code and clients use, standard library only.

### Parameter codecs

`DecodePath`, `DecodeQuery`, `DecodeHeader` and `DecodeCookie`, and the `Encode` functions the
client writes them with, handle:

- every style of the spec (`simple`, `label`, `matrix`, `form`, `spaceDelimited`,
  `pipeDelimited`, `deepObject`, `cookie`), exploded or not
- values, lists and objects
- parameters with JSON content

A value that does not decode gives `ErrParamValue`.

`DecodeQuery` reads the `Query` that `ParseQuery` makes of the raw query. A list is split at its
commas before its items are unescaped: `tags=a,b%2Cc` is `a` and `b,c`.

A `+` in a query value is a space. A value that does not unescape gives `ErrParamValue` too.

A `form` cookie is percent-encoded the same way, with a `+` read as a plus. A `cookie`-style one
goes as it is.

### Body codecs

- `DecodeJSON`, `DecodeText`, `DecodeBytes` and `DecodeFile`.
- `DecodeForm`: bracketed keys nest, `address[city]=Berlin`, `items[0]=a`. One value for a struct
  or map is read as JSON, else as a string. A property its `Encoding` gives a style is read as a
  query parameter of that style.
- `EncodeForm` writes what `DecodeForm` reads: each property in its style, else an object as JSON.
- `DecodeMultipart`: files as `runtime.File`, JSON parts into structs.
- A type with `UnmarshalForm` reads a form itself.
- A required body that is empty gives `ErrBodyEmpty`. An empty optional one is left alone.
- A form or multipart value that does not decode gives `ErrBodyValue` and names its field:
  `invalid body value: age: "x" is no int`.
- `Presence`, with its methods `JSON`, `Form` and `Multipart`, checks the keys of a body and fills
  its defaults before it is decoded.

### Other runtime packages

Five packages sit beside it:

- `pkg/runtime/httpserver` holds what a generated server needs beside the codecs: `HandlerError`
  and the error handlers, and `Write`, which sends a status, headers and a body. It writes JSON for
  most values, text and bytes as they are, and streams a `File`.
- `pkg/runtime/httpclient` holds the requests and responses of a client, see
  [client](client.md#runtime).
- `pkg/runtime/validation` holds the checks of `Validate` and the errors they return (see
  [validation](validation.md)).
- `pkg/runtime/mask` holds the masks of sensitive values.
- `pkg/runtime/mcptool` holds what generated MCP tools call.
