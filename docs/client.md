# Client

A `client` block in the config generates an HTTP client: a client type with one method per
operation, and a request options type each method takes. On request, it also generates an envelope
type per operation that carries the whole response.

```yaml
client:
  name: PetClient      # the client type; defaults to Client
  timeout: 5s          # how long a call may take, 0s for no limit; defaults to 3s
  with-response: true  # also generate <Op>WithResponse and the envelopes
  streaming: true      # also generate <Op>Stream for responses that come frame by frame
```

## Client

```go
type HTTPDoer = httpclient.Doer                                // Do(*http.Request) (*http.Response, error)
type RequestEditor func(ctx context.Context, req *http.Request) error
type PetClientOption func(*PetClient)

func NewPetClient(baseURL string, opts ...PetClientOption) (*PetClient, error)
func WithHTTPClient(d HTTPDoer) PetClientOption
func WithTimeout(d time.Duration) PetClientOption
func WithRequestEditor(fns ...RequestEditor) PetClientOption
func WithJSON(marshal func(v any) ([]byte, error), unmarshal func(data []byte, v any) error) PetClientOption
```

### Base URL

`NewPetClient` needs a base URL with a scheme and a host, such as `https://api.example.test/v1`.

- The path of every operation goes after the path of the base URL.
- The query of the base URL, such as `?key=abc`, comes first in the query of every request.
- The fragment of the base URL is not sent.

### HTTP client

The client sends with an `http.Client`. `WithHTTPClient` replaces it with anything that has the
`Do` method of `*http.Client`. Set up retries, tracing and transports there.

A nil `HTTPDoer` panics at once, as a nil editor given to `WithRequestEditor` does. The panic does
not wait for the first call.

### Timeout

A call gives up after `client.timeout`, whatever sends it. `WithTimeout` sets another limit, and 0
means none, as `timeout: 0s` does in the config.

| Method | The limit covers |
|---|---|
| plain method | the whole call, the body included |
| stream method | getting the response headers. The frames then come until the server ends the stream or the context is canceled |

The `Timeout` of an `http.Client` covers reading the body too, so it cuts a stream. Leave it unset
and use `WithTimeout`.

### Request editors

Request editors run on every request before it is sent, in the order they were added. An editor
that returns an error stops the request. Editors are the place for credentials.

A method takes editors of its own too, see [Methods](#methods).

### JSON library

The client writes and reads JSON with `encoding/json`. `WithJSON` swaps in another library that
has the same two functions:

```go
import "github.com/bytedance/sonic"

c, err := NewPetClient(baseURL, WithJSON(sonic.Marshal, sonic.Unmarshal))
```

| Uses the library | Stays on `encoding/json` |
|---|---|
| JSON request bodies | parameters with JSON content |
| JSON response bodies and stream frames | the `MarshalJSON` methods of generated types, which the library calls |

A nil function panics at once, as a nil `HTTPDoer` does.

### Operation ID

The context of a request holds the name of its operation, `ListPets`. Editors and the `HTTPDoer`
read it with `runtime.OperationID(ctx)` or `runtime.OperationID(req.Context())`, for example to tag
a metric or a trace span.

### Interface

`PetClientInterface` lists every method of the client but `<Op>Request`, so a test double can
stand in for it. The client satisfies it, which is checked at compile time.

A `client.interface-header` block writes lines before it, such as a `go:generate` line for a mock
([templates](templates.md#blocks)).

## Methods

Every operation has the same shape, even one without parameters or body. `opts` may be nil when
there is nothing to send. Webhooks get no method, since they come in.

```go
type PetClientInterface interface {
	// ListPets calls GET /pets.
	// List pets
	ListPets(ctx context.Context, opts *ListPetsRequestOptions, editors ...RequestEditor) (ListPetsResponse200, error)
	// CreatePet calls POST /pets.
	CreatePet(ctx context.Context, opts *CreatePetRequestOptions, editors ...RequestEditor) (*Pet, error)
	// DeletePet calls DELETE /pets/{id}.
	DeletePet(ctx context.Context, opts *DeletePetRequestOptions, editors ...RequestEditor) error
}

func (c *PetClient) ListPetsRequest(ctx context.Context, opts *ListPetsRequestOptions, editors ...RequestEditor) (*http.Request, error)
```

### Method comments

The comment of a method starts with the HTTP method and the path it calls, then the summary and the
description of the spec. `models.descriptions: false` leaves out the summary and the description.

### Editors per call

`editors` run on the request of that one call, after the editors of the client. They are for what
changes from call to call, such as a request ID:

```go
pets, err := c.ListPets(ctx, nil, func(_ context.Context, req *http.Request) error {
	req.Header.Set("X-Request-ID", id)
	return nil
})
```

### Success

The method returns the body of the lowest 2xx response the spec documents with a body the client
can decode. When that response has several media types, the JSON one is used, else the first.

| Response | The method gives |
|---|---|
| the lowest documented 2xx with a body the client can decode | the body |
| another documented 2xx, or one without a body | the zero value |
| a 2xx the spec does not list, such as 202 where it documents 201 and 204 | a `*httpclient.APIError` with the raw body and no error type. `default` never covers a 2xx |

An operation without such a response returns only an error, and every 2xx counts as success.

#### A body the client cannot decode

A 2xx body the client cannot decode, such as XML into a struct, is not returned. For a spec that
documents only `application/xml`, the method is `GetPet(ctx, opts) error`. Generation warns
(`client-body-unread`), and `<Op>WithResponse` holds the raw body.

### Errors

A response outside 2xx is a `*httpclient.APIError` with the status, the headers and the raw body.

When the spec documents an error type for the status (see `models.error-mapping`), the body is
decoded into it, and `errors.As` finds it through the `APIError`. A status documented without an
error type gets none, even when its range or `default` has one:

```go
pet, err := c.GetPet(ctx, &GetPetRequestOptions{PathParams: &GetPetPathParams{ID: 7}})
var problem *Problem
if errors.As(err, &problem) {
	log.Println(problem.Detail)
}
var apiErr *httpclient.APIError
if errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusNotFound {
	return nil
}
```

### Decoding by media type

A 2xx body is decoded by its media type:

| Response body | Result |
|---|---|
| a media type the method does not take, such as HTML where JSON is documented | `runtime.ErrContentType` |
| no `Content-Type` | decoded as the documented type, the JSON one when there are several |
| binary (`format: binary`) | a `runtime.File` that holds the body as it came, under the response's media type |
| `application/x-www-form-urlencoded` | read as a form, with the keys of `DecodeForm` |
| `multipart/form-data` | read into its struct the way the server reads a multipart request, every part held in memory |
| a wildcard media type (`*/*`, `application/*`), into a string, bytes or a `runtime.File` | the body as it came, whatever the response's media type, also JSON. The client sends these the same way |
| a wildcard media type, into anything else | read as JSON |
| a schema without a type (an `any`) and a body that is not JSON | as for an absent schema: the text under `text/*`, else the bytes |
| a text body into a number, a boolean or a time | read from its text |

Media types are compared without their parameters and in lower case, so
`application/json; charset=utf-8` is JSON. A request body goes under the media type as the spec
writes it.

### Accept

`<Op>` and `<Op>WithResponse` send `Accept` with every media type the responses of the operation
come in. The one `<Op>` returns is first, then the others in the order of the spec:

```
Accept: application/json, application/xml, application/problem+json
```

- Sequential media types are left out. The stream methods ask for theirs.
- An `Accept` the request already has, set by an editor, is kept.

### Building without sending

`<Op>Request` builds the request without sending it, with the editors of the client and of the call
applied. Use it to send through something else, to log, or to test what an operation sends.

It sets no `Accept`: the methods add it when they send.

## Request options

```go
type CreatePetRequestOptions struct {
	Query   *CreatePetQuery
	Headers *CreatePetHeaders
	Body    *Pet
}

func (o *CreatePetRequestOptions) Validate() error
```

Every method takes one options struct. It has a field for each parameter location the operation
uses, and `Body` for the request body. An operation with several body media types gets one body
field per media type, named as on the [server](server.md#request-options).

`Validate` checks the parameters and the body against the spec, with the same checks the server
runs. The client never calls it for you.

### Values you leave out

| You leave out | What happens |
|---|---|
| a whole group, such as `Query` | none of its parameters are sent |
| a required parameter | `runtime.ErrParamMissing`, before anything is sent |
| a path parameter | `runtime.ErrParamMissing`, even when the spec marks it optional |
| an optional parameter without a pointer (`x-go-type-skip-optional-pointer`) | it is not sent while it holds its zero value |

### Parameters

Each parameter is written in the style the spec gives it. Path values are escaped, so the
delimiters of a style survive.

These fail before anything is sent:

- A path value that writes as nothing, such as `""`: `runtime.ErrParamMissing`. `/pets/` would
  be a different path.
- A cookie value with a byte a cookie cannot hold, such as `;` or `"`: `runtime.ErrParamValue`.
  net/http would drop the byte and only log it.

An object leaves out a property that is nil, an empty list or map, and a zero value tagged
`omitempty`.

A list or object inside an object:

| Style | Example |
|---|---|
| `deepObject`, list inside | `filter[tags][0]=a&filter[tags][1]=b` |
| `deepObject`, object inside | `filter[size][x]=1` |
| `deepObject`, list of objects inside | `filter[items][0][price]=p1` |
| exploded `form` or `cookie`, list inside | `status=a&status=b` |
| any other style | cannot be written: `runtime.ErrParamValue`. Generation leaves such a parameter out with a warning. |

#### deepObject past an object

OpenAPI defines `deepObject` for an object of scalars only. Everything else is written in the
bracket form most form APIs read, Stripe's among them: a bracket per level, an index per list item.

| Value | Written |
|---|---|
| `{city: Rome}` | `address[city]=Rome` |
| `[a, b]` | `expand[0]=a&expand[1]=b` |
| `[{price: p1}]` | `items[0][price]=p1` |
| `{a: {b: [1]}}` | `filter[a][b][0]=1` |
| `https://x.test`, one value | `url=https%3A%2F%2Fx.test` |
| a union, `{gte: 1}` or `1` | `created[gte]=1` or `created=1` |

Brackets are shown as they read. On the wire they are `%5B` and `%5D`.

Generation notes each such parameter with `-v` (`deepobject-convention`). A server reads `[0]`,
`[]` and a repeated bare name, `expand=a&expand=b`, the same way.

### Query parameters

Query parameters go in the order of the spec. Values are percent-encoded the way RFC 6570 writes a
form-style query: every byte except letters, digits and `-._~` is escaped, and a space becomes
`%20`.

A separator stays as it is. The same byte inside a value is escaped:

```go
Tags: []string{"a", "b,c"} // tags=a,b%2Cc
```

With `allowReserved: true`, a value keeps the reserved characters a query can hold, and its own
`%XX` escapes: `ids=List(1,2)`. Two rules still apply:

- `[`, `]` and `#` are always escaped.
- `&`, `=` and `+` are sent as they are. If a value holds them as data, escape them yourself.

### Cookies

- A `form` cookie is percent-encoded like a query value, `allowReserved` included.
- A `cookie`-style cookie is sent as it is. If the value needs escaping, escape it yourself.

### A query inside the path

Some specs write part of the query into the path. A `?` in a spec path starts that query. It is
sent as written, before the query parameters.

| Spec path | Sent |
|---|---|
| `/rest?method=photos.search`, with `text` set | `/rest?method=photos.search&text=fox` |
| `/#Action=ListUsers` | `/`, plus the `Action` query parameter the spec declares |

- A `#` starts a fragment, which is never sent.
- A key written in the path that is also declared as a query parameter is sent twice.
- A path parameter fills its placeholder in the query too, escaped for a query.
- A placeholder that no path parameter fills, such as `{query}` in `/search?query={query}` when
  `query` is a query parameter, fails every call with `runtime.ErrParamMissing`. Generation warns
  (`path-param-missing`).

### querystring

A `querystring` parameter has a field of its own, see the
[server's request options](server.md#request-options). It goes after the query parameters.

| Media type | Sent as |
|---|---|
| a form | form values: `name=rex&tag=a&tag=b` |
| JSON | its JSON text |

Both escape every byte except letters, digits and `-._~`. A nil field sends nothing, unless the
parameter is required.

### Bodies

The body is written in its media type:

| Media type | Written as |
|---|---|
| `application/json`, `+json` | JSON |
| `application/x-www-form-urlencoded` | a form, see [the encoding object](#the-encoding-object) |
| `multipart/form-data` | a multipart form, see below |
| any, holding a `runtime.File` | the file, streamed |
| text or bytes | as they are |
| a number, boolean, time or `any` under a text type | its text, see `runtime.EncodeText` |
| `text/*`, `multipart/*` | as `text/plain`, `multipart/form-data` |
| any other wildcard | JSON, text or bytes, by the Go type of the field |

- With several body fields, the first one you set is sent.
- A required body with none set is `runtime.ErrBodyEmpty`.
- A body the client cannot write, such as XML into a struct, is `runtime.ErrContentType`.
  Generation warns (`client-body-unwritable`).

#### Multipart

A multipart form is written while it is sent, so its files stream too.

- A file without a name is named `blob`, as browsers name a Blob.
- The request has a `Content-Length` when every file knows its size. It is chunked when one does
  not, such as a `runtime.NewFileReader` of size -1.
- A list is one part per item. A list of objects is one JSON part per item.

#### The encoding object

The `encoding` object of a form body says how each property is written. Url-encoded and multipart
forms both follow it.

| The property's encoding has | It is written |
|---|---|
| nothing | text for a scalar, JSON for an object, a list one item at a time |
| `contentType` | in that type, see [Content types](#content-types) |
| `style`, `explode` or `allowReserved` | as a query parameter of that style, see [Styles](#styles) |

So without an encoding, an object is one JSON field, as OpenAPI says: `address={"city":"Rome"}`.

##### Content types

In a multipart form, each part goes in the declared type:

| Declared type | The part holds |
|---|---|
| `application/json` | the JSON of the value: `"p1"` for a string |
| any other | the text of a string, number or boolean |

- A list goes item by item, each in that type.
- A file part keeps the content type of its `runtime.File`. It takes the declared one only when it
  has none.
- A file without a type, under a list of types or a wildcard (`image/png, image/jpeg`), is an
  error before anything is sent. Only the caller knows which type it is.

In a url-encoded form, a property declared JSON is one field that holds its JSON: `meta={"a":1}`.
A list too.

- From a list of types, the first one that fits is used.
- An object under a type that is not JSON cannot be written: `runtime.ErrContentType`.
  Generation warns (`encoding-unsupported`).

##### Styles

A property with `style`, `explode` or `allowReserved` is written as a query parameter of that style.
Its `contentType` is ignored, as OpenAPI says. Without `style`, the style is `form`.

| Encoding | Value | Url-encoded form |
|---|---|---|
| `style: form` | `tags: [a, b]` | `tags=a,b` |
| `style: form, explode: true` | `tags: [a, b]` | `tags=a&tags=b` |
| `style: form, explode: true` | `color: {R: 1, G: 2}` | `R=1&G=2` |
| `style: spaceDelimited` | `tags: [a, b]` | `tags=a%20b` |
| `style: pipeDelimited` | `tags: [a, b]` | `tags=a%7Cb` |
| `style: deepObject` | `address: {city: Rome}` | `address[city]=Rome` |
| `style: deepObject` | `expand: [a, b]` | `expand[0]=a&expand[1]=b` |
| `allowReserved: true` | `url: a/b?c` | `url=a/b?c` |

- `deepObject` past an object of scalars follows [the bracket form](#deepobject-past-an-object).
- A union under `deepObject` is written by the variant that is set: `address[city]=Rome`, or
  `address=` for an empty string.
- In a multipart form each pair is a text part of its own, not percent-encoded: a part named
  `address[city]` holds `Rome`.
- Styles count in a multipart form in every OpenAPI version. 3.0 names url-encoded forms only, but
  specs written for 3.0 use them in multipart forms too.

A style that cannot write the property is dropped, and the property is written as if it had no
style. Generation warns (`encoding-ignored`):

- a style a query does not take, such as `matrix`
- `spaceDelimited` or `pipeDelimited` on one value, or with `explode: true`
- a list or object inside a `form` or delimited object
- a union of more than scalars under any style but `deepObject`
- a file

## Envelopes

With `with-response: true`, every operation also gets an envelope and a method that returns it,
listed in the interface next to the plain method:

```go
type SubmitJobResponse struct {
	HTTPResponse   *http.Response
	Body           []byte
	JSON201        *Result
	JSON202        *Queued
	ProblemJSON400 *Problem
	Headers201     *SubmitJobResponse201Headers
	Headers202     *SubmitJobResponse202Headers
}

func (r *SubmitJobResponse) StatusCode() int
func (c *Client) SubmitJobWithResponse(ctx context.Context, opts *SubmitJobRequestOptions, editors ...RequestEditor) (*SubmitJobResponse, error)
```

### Fields

| Field | Holds |
|---|---|
| `HTTPResponse` | the response, with its body read and closed |
| `Body` | the raw bytes. `HTTPResponse.Body` reads them again |
| one field per documented body the client decodes | named after the media type and the status: `JSON200`, `Text200`, `ProblemJSON4XX`, `JSONDefault` |
| `Headers<status>` | one per response that declares headers: the struct the server side uses too, filled from the response headers |

- Two media types with one tag at a status are told apart by the type, then by a number.
- A body the client cannot decode, such as XML into a struct, has no field.

### Decode errors

A body or header that does not decode is an error. The envelope comes back with it, so the status
and the raw body are still there.

### Which fields a response fills

The response fills the fields of the first of these the spec documents:

1. its status
2. its range, such as `2XX`
3. `default`

Among those fields, the body field whose media type fits the response best is filled. The order of
fit is the same media type, then JSON for any JSON, then a wildcard.

A documented status fills only its own fields. A `410` without a body fills nothing, even next to a
`4XX` with one.

A status outside 2xx is not an error. These are errors: a request that cannot be built or sent,
and a body that does not decode.

## Streaming

Some responses are a sequence of frames that keeps coming: a Server-Sent Events feed, or a log
tailed as one JSON value per line. The plain method reads such a body whole, so it returns only when
the server closes the connection. With `streaming: true`, every operation that documents a
sequential response also gets a method that reads it frame by frame:

```go
func (c *PetClient) ChatStream(ctx context.Context, opts *ChatRequestOptions, editors ...RequestEditor) (*httpclient.Stream[Chunk], error)
```

A response is sequential when its media type is one of:

| Media type | Framing |
|---|---|
| `text/event-stream` | Server-Sent Events: the `data` lines of an event, joined with newlines |
| `application/x-ndjson`, `application/ndjson`, `application/jsonl`, `application/x-jsonlines`, `application/json-lines` | one JSON value per line |

### Which response is streamed

Only a 2xx response in a sequential media type is streamed.

- The stream reads the lowest 2xx response with a sequential media type, the first such media type
  when there are several.
- `<Op>` and `<Op>WithResponse` are not changed. An operation that documents `application/json`
  next to `text/event-stream` at one status keeps both shapes.
- A 2xx response without a body, such as 204, is a stream without frames.
- A 2xx response with a body in another media type is `runtime.ErrContentType`. You do not get a
  stream that yields nothing.
- A response outside 2xx is a `*httpclient.APIError`, with the error type of its status decoded, as
  with `<Op>`.

### Frame type

The frame type comes from `itemSchema` (OpenAPI 3.2), else from `schema`, the way specs before 3.2
describe one event.

- A `$ref` reuses the component.
- An inline schema becomes `<Op>ResponseItem` ([naming](naming.md)).
- Without a schema, or with one whose JSON is a string, such as a bare string, a `date-time` or a
  string enum, frames come as `[]byte`. The data of an event is text, not a JSON string.

### Accept and the request

`<Op>Stream` sends `Accept: <media type>` unless the request sets one.

An endpoint that answers either way usually decides from a request field, which you still have to
set: `&ChatRequestOptions{Body: &Prompt{Text: "hi", Stream: runtime.Ptr(true)}}`.

### Stream timeout

The timeout of the client covers the wait for the response headers only, not the frames that
follow.

### Warnings

| Code | When |
|---|---|
| `stream-only` | `streaming` is off, and the 2xx responses of an operation come in sequential media types only. Its plain method blocks until the server hangs up |
| `stream-unread` | a sequential response is documented under `default` alone. It gets no stream method, since `default` never covers a 2xx. Document it under `200` or `2XX` |

### Reading a stream

`httpclient.Stream[T]` reads like `bufio.Scanner`. The caller owns the connection and closes the
stream:

```go
stream, err := client.ListEventsStream(ctx, nil)
if err != nil {
	return err
}
defer stream.Close()

for stream.Next() {
	event := stream.Current()   // one frame, decoded into T
	raw := stream.Event()       // the frame behind it: SSE id, event and retry, and the data
	log.Println(raw.ID, event.Seq)
}
return stream.Err()
```

| Member | What it does |
|---|---|
| `All()` | the same loop as a range-over-func iterator, with the error that stops the stream delivered as the last pair: `for event, err := range stream.All()`. Breaking out of the loop leaves the stream open, so the caller still closes it |
| `Close()` | may be called from another goroutine to end a pending `Next`, which then returns false |
| `Sentinels` | frames that end the stream instead of being decoded, see below |
| `MaxFrameSize` | the size limit of one frame, see below |
| `Unmarshal` | what decodes a frame, the client's [JSON library](#json-library) in `<Op>Stream` and `json.Unmarshal` when nil |

#### Err

| `Err()` is | When |
|---|---|
| nil | the stream ended, a sentinel ended it, or `Close()` was called |
| the read error | the read failed |
| `httpclient.ErrFrame` | a frame did not decode (the decode error) |
| `context.Canceled` | the request's context was canceled. This also unblocks a pending `Next` |
| `io.ErrUnexpectedEOF` | an event stream ended inside an event with data, before its blank line. That event is not delivered, as a browser drops it |

#### Sentinels

`Sentinels` lists frames that end the stream instead of being decoded. APIs in the style of OpenAI
end a stream with `data: [DONE]`, which is not JSON. Set `stream.Sentinels = []string{"[DONE]"}`
before the first `Next`.

#### MaxFrameSize

`MaxFrameSize` caps one line of the body and the data of one event, in bytes. It is 0, no limit,
unless you set it before the first `Next`. A longer frame stops the stream with
`httpclient.ErrFrameSize`.

#### Event streams

- Comments are skipped.
- An event without `data` is not dispatched.
- `retry` must be a whole number of milliseconds.
- A line may end in LF, CRLF or a lone CR.
- A byte order mark at the start of the body is dropped.
- `Event().ID` is the last event ID. It stays from one event to the next until an `id` field
  changes it, as in a browser.
- A stream of anything but `[]byte` skips an event whose data is empty, which servers send to keep
  a connection alive.

#### Line-delimited streams

Lines that are empty or hold only spaces are skipped.

#### Content-Type

A `Content-Type` parameter without a value, such as `text/event-stream; charset`, keeps the media
type.

### With envelopes

With `with-response: true`, the envelope gains a `Stream<status>` field, and
`<Op>StreamWithResponse` fills it. For a streamed response:

- `Body` is nil.
- `HTTPResponse.Body` stays open until the stream is closed.
- `Headers<status>` is filled as for any other response. A header that does not decode is an error
  and closes the stream.

Any other response is read and decoded into the usual fields. It is not an error.

### Helpers

The helpers work off any `*http.Response`:

| Helper | What it does |
|---|---|
| `httpclient.NewStream[T]` | picks the framing from the `Content-Type` |
| `httpclient.NewEventStream[T]` and `httpclient.NewLineStream[T]` | set the framing |
| `httpclient.SendStream` | sends a request and leaves the body of a streamed response unread |
| `httpclient.OpenStream[T]` | turns what `SendStream` returns into a stream or an error, as `<Op>Stream` does |

### Limits

- Request bodies are not streamed.
- `multipart/mixed` and `application/json-seq` are not framed.
- A generated server writes a sequential response as one document, since writing Server-Sent Events
  from a handler is not generated yet.

## Not supported yet

### Encoding keys

- `headers` in the `encoding` object are not read. Each part goes without them.
- The encoding of a body that is a union is not read, nor is the encoding of a response. Their
  properties are written as if they had none.

Generation warns (`encoding-ignored`).

### allowReserved on path parameters

`allowReserved` is read on query parameters only. OpenAPI 3.2 allows it on a path parameter too,
where it is not read.

## Layout

The client has four parts for `output.files`:

| Part | Holds |
|---|---|
| `client.options` | the request options |
| `client.responses` | the envelopes |
| `client.core` | the interface, the client type and its options |
| `client.operations` | the methods |

A file that holds several of them has them in this order.

The operations add methods to the client type, so they must stay in the folder of the core. The
options and the envelopes may go anywhere, with the models.

## Runtime

Generated clients use the package `pkg/runtime/httpclient`, on top of the codecs of the runtime
package the server uses too.

### Building requests

`RequestBuilder` puts a request together: `PathParam`, `QueryParam`, `HeaderParam`, `CookieParam`
and the body methods, then `Build` against the base URL.

- The first error stops the rest and comes back from `Build`.
- A form or multipart body is written in the shapes `runtime.DecodeForm` and
  `runtime.DecodeMultipart` read.

### Sending and decoding

- `Send` sends with a `Doer` and reads the body within a timeout.
- `DecodeSuccess` and `DecodeResponse` fill the `ResponseTarget` of the response's status, its typed
  headers too. A header that does not parse is `runtime.ErrHeaderValue`.
- `APIError` is the error of a status outside 2xx, or of a 2xx the spec does not list, with the
  status in `StatusCode`.

### Streams

`Stream[T]` reads a sequential response frame by frame. `SendStream`, `OpenStream`, `IsStreaming`
and `runtime.IsSequential` are what the stream methods are built on.
