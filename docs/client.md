# Client

A `client` block in the config generates an HTTP client: a client type with one method per
operation, a request options type each method takes, and, on request, an envelope type per
operation that carries the whole response.

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
```

- `NewPetClient` needs a base URL with a scheme and a host, such as `https://api.example.test/v1`;
  the path of every operation goes after its path. Its query, such as `?key=abc`, comes first in
  the query of every request. Its fragment is not sent.
- The client sends with an `http.Client`. `WithHTTPClient` replaces it with anything that has the
  `Do` method of `*http.Client`, so retries, tracing and transports are set up there. A nil one
  panics at once, as a nil editor given to `WithRequestEditor` does, not on the first call.
- A call gives up after `client.timeout`, whatever sends it. `WithTimeout` sets another limit, and
  0 means none, as `timeout: 0s` does in the config. A plain method has that long for the whole
  call, the body included. A stream method has that long to get the response headers; the frames
  then come until the server ends the stream or the context is canceled. The `Timeout` of an
  `http.Client` covers reading the body too, so it cuts a stream: leave it unset and use
  `WithTimeout`.
- Request editors run on every request before it is sent, in the order they were added, and stop
  the request when they return an error. They are the place for credentials. A method takes
  editors of its own too, see [Methods](#methods).
- The context of a request holds the name of its operation, `ListPets`. Editors and the
  `HTTPDoer` read it with `runtime.OperationID(ctx)` or `runtime.OperationID(req.Context())`, to
  tag a metric or a trace span.

`PetClientInterface` lists every method of the client but `<Op>Request`, so a test double can
stand in for it. The client satisfies it, which is checked at compile time. A
`client.interface-header` block writes lines before it, such as a `go:generate` line for a mock
([templates](templates.md#blocks)).

## Methods

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

- Every operation has the same shape, even one without parameters or body, and `opts` may be nil
  when there is nothing to send. Webhooks get no method, since they come in.
- The comment of a method starts with the HTTP method and the path it calls, then the summary and
  the description of the spec. `models.descriptions: false` leaves out the summary and the
  description.
- `editors` run on the request of that one call, after the editors of the client. They are for
  what changes from call to call, such as a request ID:

  ```go
  pets, err := c.ListPets(ctx, nil, func(_ context.Context, req *http.Request) error {
  	req.Header.Set("X-Request-ID", id)
  	return nil
  })
  ```
- The method returns the body of the lowest 2xx response the spec documents with a body the client
  can decode: its JSON media type, else its first one. An operation without such a response
  returns the error alone and takes any 2xx. Another 2xx the spec documents, or one without a
  body, gives the zero value.
- A 2xx body the client cannot decode, such as XML into a struct, is no body the method returns:
  `GetPet(ctx, opts) error` for a spec that documents only `application/xml`. Generation warns
  (`client-body-unread`), and `<Op>WithResponse` holds the raw body.
- A 2xx the spec does not list, such as 202 where it documents 201 and 204, is a
  `*httpclient.APIError` with the raw body and no error type: `default` never covers a 2xx.
- A response outside 2xx is a `*httpclient.APIError` with the status, the headers and the raw body.
  When the spec documents an error type for the status (see `models.error-mapping`), the body is
  decoded into it and `errors.As` finds it through the `APIError`:

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

- A 2xx body in a media type the method does not take, such as HTML where JSON is documented, is
  `runtime.ErrContentType`. A response without a `Content-Type` is decoded as the documented
  type, the JSON one when there are several.
  A binary body (`format: binary`) comes back as a `runtime.File` that holds the body as it came,
  under the response's media type. An `application/x-www-form-urlencoded` body is read as a form,
  with the keys of `DecodeForm`. A `multipart/form-data` body is read into its struct the way the
  server reads a multipart request, every part held in memory. Under a wildcard media type
  (`*/*`, `application/*`) a string, bytes or a `runtime.File` takes the body as it came whatever
  the response's media type, also JSON, the same way the client sends them; anything else is read
  as JSON. A schema without a type, an `any`, holds a body that is no JSON as an absent schema
  does: the text under `text/*`, else the bytes. A text body into a number, a boolean or a time
  is read from its text. Media types are
  compared without their parameters and in lower case, so `application/json; charset=utf-8` is
  JSON; a request body goes under the media type as the spec writes it.
- `<Op>` and `<Op>WithResponse` send `Accept` with every media type the responses of the
  operation come in, the one `<Op>` returns first, then the others in the order of the spec:
  `Accept: application/json, application/xml, application/problem+json`. Sequential media types
  are left out; the stream methods ask for theirs. An `Accept` the request already has, set by an
  editor, is kept.
- `<Op>Request` builds the request without sending it, with the editors of the client and of the
  call applied. Use it to send through something else, to log, or to test what an operation sends.
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

- One field per parameter location the operation uses, holding the struct of its parameters, and
  `Body` for the request body; an operation with several media types gets one field per media
  type, named as the [server's request options](server.md#request-options) are. A group left nil
  sends none of its parameters; a required parameter that is nil is `runtime.ErrParamMissing`
  before anything is sent, and so is a path parameter, whatever the spec says. An optional
  parameter held as a plain value (`x-go-type-skip-optional-pointer`) is not sent at its zero
  value.
- Parameters are written in the style of the spec with the runtime codecs, path values escaped
  so that the delimiters of the styles survive. A path value written as nothing, such as an empty
  string, is `runtime.ErrParamMissing`, since `/pets/` is another path. A cookie value with a byte
  no cookie holds, such as `;` or `"`, is `runtime.ErrParamValue`, where net/http would drop the
  byte and log it. An object leaves out a property that is nil, a list or map with no items, or
  a zero value tagged `omitempty`. A `deepObject` writes a list inside it once per item,
  `filter[tags]=a&filter[tags]=b`, and an object inside it nested, `filter[size][x]=1`. An
  exploded `form` or `cookie` object writes a list inside it as its key once per item,
  `status=a&status=b`. The other styles have no way to write a list or object inside an object,
  so setting one is `runtime.ErrParamValue`; generation leaves such parameters out with a warning.
- A `form` cookie is percent-encoded as a query value is, `allowReserved` included; a
  `cookie`-style one goes as it is, so a value that needs escaping is escaped by the caller.
- Query parameters go in the order of the spec, percent-encoded as RFC 6570 writes a form-style
  query: every byte but letters, digits and `-._~` is escaped, a space as `%20`. A separator goes
  as it is and the same byte inside a value is escaped, so `[]string{"a", "b,c"}` goes as
  `tags=a,b%2Cc`. With `allowReserved: true`, a value keeps the reserved characters a query holds
  and its `%XX` escapes: `ids=List(1,2)`. `[`, `]` and `#` are still escaped. `&`, `=` and `+` go
  as they are, so a value that holds them as data has to escape them itself.
- A `?` in a path of the spec starts a query, which is sent as written, ahead of the query
  parameters: `/rest?method=photos.search` with `text` set sends
  `/rest?method=photos.search&text=fox`. A key written there and declared as a query parameter goes
  out twice. A path parameter fills its placeholder in the query too, escaped for a query. A `#`
  starts a fragment, which is not sent: `/#Action=ListUsers` goes to `/`, with the `Action` query
  parameter the spec declares next to it.
- A `querystring` parameter has a field of its own, see the
  [server's request options](server.md#request-options). It goes after the query parameters: a
  form as form values, `name=rex&tag=a&tag=b`, JSON as its text, with every byte but letters,
  digits and `-._~` percent-encoded in both. A nil field sends nothing, unless the parameter is
  required.
- A placeholder that no path parameter fills, such as `{query}` in `/search?query={query}` when
  `query` is a query parameter, is `runtime.ErrParamMissing` on every call. Generation warns about
  it (`path-param-missing`).
- The body goes as its media type: JSON for `application/json` and `+json`,
  `application/x-www-form-urlencoded` as a form (a union or an object with additional
  properties in it goes as one JSON value), `multipart/form-data` as a multipart form, a
  `runtime.File` body streamed, text and bytes as they are, and a number, a boolean, a time or an
  `any` under a text media type as its text, see `runtime.EncodeText`. A multipart form is
  written while it is sent, so its files stream too, each as a file part named `blob` when it has
  no name, as browsers name a Blob. It goes with a `Content-Length` when every file knows its
  size, and chunked when one does not, such as a `runtime.NewFileReader` of size -1. A list goes
  as one part per item, a list of objects as one JSON part per item. With several body fields, the first one set is sent. A required body
  with none set is `runtime.ErrBodyEmpty`; a body the client cannot write, such as XML into a
  struct, is `runtime.ErrContentType`, and generation warns about it (`client-body-unwritable`). A
  range goes as its member: `text/*` as `text/plain`, `multipart/*` as `multipart/form-data`.
  Another wildcard media type sends its field as JSON, text or bytes, whichever the field is.
- The `encoding` object of a form body names the content type of a property. Multipart and
  url-encoded forms follow it. In a multipart form each part goes in that type:
  `application/json` writes the JSON of the value, `"p1"` for a string, and any other type the
  text of a string, number or boolean. A list goes item by item, each in that type. A file part
  keeps the content type of its `runtime.File`, and takes the declared one when it has none. A
  file without a type under a list or a wildcard, `image/png, image/jpeg`, is an error before
  anything is sent, since only the caller knows which it is. In a url-encoded form a property
  declared JSON is one field that holds its JSON, `meta={"a":1}`, also for a list. Of a list of
  types the first one that fits is used. A property that holds an object under a type that is
  not JSON cannot be written: `runtime.ErrContentType`, and generation warns
  (`encoding-unsupported`).
- `Validate` checks the parameters and the body against the spec, like the server's; the client
  does not call it on its own.

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

- `HTTPResponse` is the response with its body read and closed; `Body` holds the raw bytes, and
  `HTTPResponse.Body` reads them again.
- A body or header that does not decode is an error, and the envelope comes back with it, so the
  status and the raw body are still there.
- One field per documented body the client decodes, named after the media type and the status:
  `JSON200`, `Text200`, `ProblemJSON4XX`, `JSONDefault`. Two media types with one tag at a status
  are told apart by the type, then by a number. A body the client cannot decode, such as XML into
  a struct, has no field.
- One `Headers<status>` field per response that declares headers, holding the struct the server
  side uses too, filled from the response headers.
- The response fills the fields of its status, else of its range (`2XX`), else of `default`,
  and among them the body field whose media type fits the response's best: the same one, then
  JSON for any JSON, then a wildcard. A status outside 2xx is no error; only a request that
  cannot be built or sent, or a body that does not decode, is.

## Streaming

Some responses are not one document but a sequence of frames that keeps coming: a Server-Sent
Events feed, or a log tailed as one JSON value per line. The plain method reads such a body whole,
so it returns only when the server closes the connection. With `streaming: true`, every operation
that documents a sequential response also gets a method that reads it frame by frame:

```go
func (c *PetClient) ChatStream(ctx context.Context, opts *ChatRequestOptions, editors ...RequestEditor) (*httpclient.Stream[Chunk], error)
```

A response is sequential when its media type is one of:

| Media type | Framing |
|---|---|
| `text/event-stream` | Server-Sent Events: the `data` lines of an event, joined with newlines |
| `application/x-ndjson`, `application/ndjson`, `application/jsonl`, `application/x-jsonlines`, `application/json-lines` | one JSON value per line |

- The stream reads the lowest 2xx response with a sequential media type, the first such media
  type when there are several. `<Op>` and `<Op>WithResponse` are not changed: an operation that
  documents `application/json` next to `text/event-stream` at one status keeps both shapes.
- The frame type comes from `itemSchema` (OpenAPI 3.2), else from `schema`, the way specs before
  3.2 describe one event. A `$ref` reuses the component; an inline schema becomes
  `<Op>ResponseItem` ([naming](naming.md)). Without a schema, or with one whose JSON is a string,
  such as a bare string, a `date-time` or a string enum, frames come as `[]byte`: the data of an
  event is text, not a JSON string.
- `<Op>Stream` sends `Accept: <media type>` unless the request sets one. For an endpoint that
  answers either way, the server usually decides from a request field, which the caller still has
  to set: `&ChatRequestOptions{Body: &Prompt{Text: "hi", Stream: runtime.Ptr(true)}}`.
- The timeout of the client covers the wait for the response headers only, not the frames that
  follow.
- Only a 2xx response in a sequential media type is streamed. A 2xx response without a body, such
  as 204, is a stream without frames. A 2xx response with a body in another media type is
  `runtime.ErrContentType` rather than a stream that yields nothing; a response outside 2xx is a
  `*httpclient.APIError`, with the error type of its status decoded, as with `<Op>`.
- Without `streaming`, generation warns (`stream-only`) about every operation whose 2xx responses
  come in sequential media types only, since its plain method blocks until the server hangs up.
- A sequential response documented under `default` alone gets no stream method, since `default`
  never covers a 2xx. Generation warns (`stream-unread`): document it under `200` or `2XX`.

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

- `All()` is the same loop as a range-over-func iterator, with the error that stops the stream
  delivered as the last pair: `for event, err := range stream.All()`. Breaking out of the loop
  leaves the stream open, so the caller still closes it.
- `Err()` is nil at the end of the stream, after a sentinel and after `Close()`, else the read
  error, the decode error (`httpclient.ErrFrame`) or `context.Canceled` when the request's context
  was canceled, which unblocks a pending `Next`. When an event stream ends inside an event with data,
  before its blank line, that event is not delivered and `Err()` is `io.ErrUnexpectedEOF`, as a
  browser drops it.
- `Close()` may be called from another goroutine to end a pending `Next`, which then returns false.
- `Sentinels` lists frames that end the stream instead of being decoded. APIs in the style of
  OpenAI end a stream with `data: [DONE]`, which is no JSON: set `stream.Sentinels =
  []string{"[DONE]"}` before the first `Next`.
- `MaxFrameSize` caps one line of the body and the data of one event, in bytes. It is 0, no limit,
  unless set before the first `Next`; a longer frame stops the stream with
  `httpclient.ErrFrameSize`.
- SSE comments are skipped, an event without `data` is not dispatched, and `retry` must be a whole
  number of milliseconds. A line may end in LF, CRLF or a lone CR, and a byte order mark at the
  start of the body is dropped. `Event().ID` is the last event ID: it stays from one event to the
  next until an `id` field changes it, as in a browser. A stream of anything but `[]byte` skips
  an event whose data is empty, which servers send to keep a connection alive. Lines of a
  line-delimited stream that are empty or hold only spaces are skipped. A `Content-Type` parameter
  without a value, such as `text/event-stream; charset`, keeps the media type.

With `with-response: true`, the envelope gains a `Stream<status>` field and
`<Op>StreamWithResponse` fills it: for a streamed response, `Body` is nil,
`HTTPResponse.Body` stays open until the stream is closed, and `Headers<status>` is filled as for
any other response. A header that does not decode is an error and closes the stream. Any other
response is read and decoded into the usual fields, and is no error.

The helpers work off any `*http.Response`: `httpclient.NewStream[T]` picks the framing from the
`Content-Type`, `httpclient.NewEventStream[T]` and `httpclient.NewLineStream[T]` set it;
`httpclient.SendStream` sends a request and leaves the body of a streamed response unread, and
`httpclient.OpenStream[T]` turns what it returns into a stream or an error, as `<Op>Stream` does.

Limits: request bodies are not streamed, `multipart/mixed` and `application/json-seq` are not
framed, and a generated server writes a sequential response as one document, since writing
Server-Sent Events from a handler is not generated yet.

## Not supported yet

- In the `encoding` object, `style`, `explode`, `allowReserved` and `headers` are not read. A
  property with one of the first three is written and read by its schema type, and its
  `contentType` is ignored, as the spec says. The encoding of a body that is a union, and of a
  response, is not read either. Generation warns (`encoding-ignored`).
- `allowReserved` is read on query parameters only. OpenAPI 3.2 allows it on a path parameter
  too, where it is not read.

## Layout

The client has four parts for `output.files`: `client.options` (the request options),
`client.responses` (the envelopes), `client.core` (the interface, the client type and its options)
and `client.operations` (the methods). A file that holds several of them has them in this order.
The operations add methods to the client type, so they must stay in the folder of the core; the
options and the envelopes may go anywhere, with the models.

## Runtime

Generated clients use the package `pkg/runtime/httpclient`, on top of the codecs of the runtime
package the server uses too:

- `RequestBuilder` puts a request together: `PathParam`, `QueryParam`, `HeaderParam`,
  `CookieParam` and the body methods, then `Build` against the base URL. The first error stops
  the rest and comes back from `Build`. A form or multipart body is written in the shapes
  `runtime.DecodeForm` and `runtime.DecodeMultipart` read.
- `Send` sends with a `Doer` and reads the body within a timeout; `DecodeSuccess` and
  `DecodeResponse` fill the `ResponseTarget` of the response's status, its typed headers too. A
  header that does not parse is `runtime.ErrHeaderValue`. `APIError` is the error of a status
  outside 2xx, or of a 2xx the spec does not list, with the status in `StatusCode`.
- `Stream[T]` reads a sequential response frame by frame; `SendStream`, `OpenStream`,
  `IsStreaming` and `runtime.IsSequential` are what the stream methods are built on.
