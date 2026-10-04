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
type HTTPDoer = runtime.Doer                                   // Do(*http.Request) (*http.Response, error)
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
  the request when they return an error. They are the place for credentials.

`PetClientInterface` lists every method of the client but `<Op>Request`, so a test double can
stand in for it. The client satisfies it, which is checked at compile time.

## Methods

```go
type PetClientInterface interface {
	// List pets
	ListPets(ctx context.Context, opts *ListPetsRequestOptions) (ListPetsResponse200, error)
	CreatePet(ctx context.Context, opts *CreatePetRequestOptions) (*Pet, error)
	DeletePet(ctx context.Context, opts *DeletePetRequestOptions) error
}

func (c *PetClient) ListPetsRequest(ctx context.Context, opts *ListPetsRequestOptions) (*http.Request, error)
```

- Every operation has the same shape, even one without parameters or body, and `opts` may be nil
  when there is nothing to send. Webhooks get no method, since they come in.
- The method returns the body of the lowest 2xx response the spec documents with a body the client
  can decode: its JSON media type, else its first one. An operation without such a response
  returns the error alone and takes any 2xx. Another 2xx the spec documents, or one without a
  body, gives the zero value.
- A 2xx body the client cannot decode, such as XML into a struct, is no body the method returns:
  `GetPet(ctx, opts) error` for a spec that documents only `application/xml`. Generation warns
  (`client-body-unread`), and `<Op>WithResponse` holds the raw body.
- A 2xx the spec does not list, such as 202 where it documents 201 and 204, is a
  `*runtime.APIError` with the raw body and no error type: `default` never covers a 2xx.
- A response outside 2xx is a `*runtime.APIError` with the status, the headers and the raw body.
  When the spec documents an error type for the status (see `models.error-mapping`), the body is
  decoded into it and `errors.As` finds it through the `APIError`:

  ```go
  pet, err := c.GetPet(ctx, &GetPetRequestOptions{PathParams: &GetPetPathParams{ID: 7}})
  var problem *Problem
  if errors.As(err, &problem) {
  	log.Println(problem.Detail)
  }
  var apiErr *runtime.APIError
  if errors.As(err, &apiErr) && apiErr.Status == http.StatusNotFound {
  	return nil
  }
  ```

- A 2xx body in a media type the method does not take, such as HTML where JSON is documented, is
  `runtime.ErrContentType`. A response without a `Content-Type` is decoded as the documented
  type, the JSON one when there are several.
  A binary body (`format: binary`) comes back as a `runtime.File` that holds the body as it came,
  under the response's media type. Under a wildcard media type (`*/*`, `application/*`) a string,
  bytes or a `runtime.File` takes the body as it came whatever the response's media type, also
  JSON, the same way the client sends them; anything else is read as JSON.
- `<Op>Request` builds the request without sending it, with the editors applied. Use it to send
  through something else, to log, or to test what an operation sends.

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
  before anything is sent, and so is a path parameter, whatever the spec says.
- Parameters are written in the style of the spec with the runtime codecs, path values escaped
  so that the delimiters of the styles survive. A path value written as nothing, such as an empty
  string, is `runtime.ErrParamMissing`, since `/pets/` is another path. A cookie value with a byte
  no cookie holds, such as `;` or `"`, is `runtime.ErrParamValue`, where net/http would drop the
  byte and log it. An object leaves out a property that is nil or a
  list or map with no items. A `deepObject` writes a list inside it once per item,
  `filter[tags]=a&filter[tags]=b`, and an object inside it nested, `filter[size][x]=1`. The other
  styles have no way to write a list or object inside an object, so setting one is
  `runtime.ErrParamValue`.
- A `?` in a path of the spec starts a query, which is sent as written, ahead of the query
  parameters: `/rest?method=photos.search` with `text` set sends
  `/rest?method=photos.search&text=fox`. A key written there and declared as a query parameter goes
  out twice. A path parameter fills its placeholder in the query too, escaped for a query. A `#`
  starts a fragment, which is not sent: `/#Action=ListUsers` goes to `/`, with the `Action` query
  parameter the spec declares next to it.
- A placeholder that no path parameter fills, such as `{query}` in `/search?query={query}` when
  `query` is a query parameter, is `runtime.ErrParamMissing` on every call. Generation warns about
  it (`path-param-missing`).
- The body goes as its media type: JSON for `application/json` and `+json`,
  `application/x-www-form-urlencoded` through `EncodeForm`, `multipart/form-data` through
  `WriteMultipart`, a `runtime.File` body streamed, text and bytes as they are. A multipart form is
  written while it is sent, so its files stream too, each as a file part named `blob` when it has
  no name, as browsers name a Blob. It goes with a `Content-Length` when every file knows its
  size, and chunked when one does not, such as a `runtime.NewFileReader` of size -1. With several body fields, the first one set is sent. A required body
  with none set is `runtime.ErrBodyEmpty`; a body the client cannot write, such as XML into a
  struct, is `runtime.ErrContentType`. A wildcard media type sends its field as JSON, text or bytes,
  whichever the field is.
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
func (c *Client) SubmitJobWithResponse(ctx context.Context, opts *SubmitJobRequestOptions) (*SubmitJobResponse, error)
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
func (c *PetClient) ChatStream(ctx context.Context, opts *ChatRequestOptions) (*runtime.Stream[Chunk], error)
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
  `*runtime.APIError`, with the error type of its status decoded, as with `<Op>`.
- Without `streaming`, generation warns (`stream-only`) about every operation whose 2xx responses
  come in sequential media types only, since its plain method blocks until the server hangs up.
- A sequential response documented under `default` alone gets no stream method, since `default`
  never covers a 2xx. Generation warns (`stream-unread`): document it under `200` or `2XX`.

`runtime.Stream[T]` reads like `bufio.Scanner`. The caller owns the connection and closes the
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
  error, the decode error (`runtime.ErrFrame`) or `context.Canceled` when the request's context was
  canceled, which unblocks a pending `Next`. When an event stream ends inside an event with data,
  before its blank line, that event is not delivered and `Err()` is `io.ErrUnexpectedEOF`, as a
  browser drops it.
- `Close()` may be called from another goroutine to end a pending `Next`, which then returns false.
- `Sentinels` lists frames that end the stream instead of being decoded. APIs in the style of
  OpenAI end a stream with `data: [DONE]`, which is no JSON: set `stream.Sentinels =
  []string{"[DONE]"}` before the first `Next`.
- `MaxFrameSize` caps one line of the body and the data of one event, in bytes. It is 0, no limit,
  unless set before the first `Next`; a longer frame stops the stream with `runtime.ErrFrameSize`.
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

The helpers work off any `*http.Response`: `runtime.NewStream[T]` picks the framing from the
`Content-Type`, `runtime.NewEventStream[T]` and `runtime.NewLineStream[T]` set it;
`runtime.SendStream` sends a request and leaves the body of a streamed response unread, and
`runtime.OpenStream[T]` turns what it returns into a stream or an error, as `<Op>Stream` does.

Limits: request bodies are not streamed, `multipart/mixed` and `application/json-seq` are not
framed, and a generated server writes a sequential response as one document, since writing
Server-Sent Events from a handler is not generated yet.

## Not supported yet

- `in: querystring` (OpenAPI 3.2) gets no field, on the client or the server: the parameter is
  neither sent nor read. Generation warns (`querystring-unsupported`).
- The `encoding` object of a body is not read: the parts of a form are written and read by their
  schema types. Generation warns (`encoding-ignored`).

## Layout

The client has four parts for `output.files`: `client.core` (the client type and its options),
`client.options` (the request options), `client.operations` (the interface and the methods) and
`client.responses` (the envelopes). The operations add methods to the client type, so they must
stay in the folder of the core; the options and the envelopes may go anywhere, with the models.

## Runtime

Generated clients use these helpers of the runtime package, next to the codecs the server uses:

- `RequestBuilder` puts a request together: `PathParam`, `QueryParam`, `HeaderParam`,
  `CookieParam` and the body methods, then `Build` against the base URL. The first error stops
  the rest and comes back from `Build`.
- `EncodeForm` and `WriteMultipart` write a struct as a form, in the shapes `DecodeForm` and
  `DecodeMultipart` read.
- `Send` sends with a `Doer` and reads the body within a timeout; `DecodeSuccess` and `Decode`
  fill the targets of the response, `DecodeHeaders` a struct of typed headers; `APIError` is the
  error of a status outside 2xx, or of a 2xx the spec does not list.
- `Stream[T]` reads a sequential response frame by frame; `SendStream`, `OpenStream`,
  `IsStreaming` and `IsSequential` are what the stream methods are built on.
