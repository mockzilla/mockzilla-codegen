# Client

A `client` block in the config generates an HTTP client: a client type with one method per
operation, a request options type each method takes, and, on request, an envelope type per
operation that carries the whole response.

```yaml
client:
  name: PetClient      # the client type; defaults to Client
  timeout: 5s          # what the default http.Client gives up after; defaults to 3s
  with-response: true  # also generate <Op>WithResponse and the envelopes
```

## Client

```go
type HTTPDoer = runtime.Doer                                   // Do(*http.Request) (*http.Response, error)
type RequestEditor func(ctx context.Context, req *http.Request) error
type PetClientOption func(*PetClient)

func NewPetClient(baseURL string, opts ...PetClientOption) (*PetClient, error)
func WithHTTPClient(d HTTPDoer) PetClientOption
func WithRequestEditor(fns ...RequestEditor) PetClientOption
```

- `NewPetClient` needs a base URL with a scheme and a host, such as `https://api.example.test/v1`;
  the path of every operation goes after its path.
- The client sends with an `http.Client` whose timeout is `client.timeout`. `WithHTTPClient`
  replaces it with anything that has the `Do` method of `*http.Client`, so retries, tracing and
  transports are set up there.
- Request editors run on every request before it is sent, in the order they were added, and stop
  the request when they return an error. They are the place for credentials.

`PetClientInterface` lists every method of the client, so a test double can stand in for it. The
client satisfies it, which is checked at compile time.

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
- The method returns the body of the lowest 2xx response the spec documents with a body: its JSON
  media type, else its first one. An operation without such a response returns the error alone.
  A 2xx response without a body gives the zero value.
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
  `runtime.ErrContentType`. A response without a `Content-Type` is decoded as the documented type.
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
  so that the delimiters of the styles survive.
- The body goes as its media type: JSON for `application/json` and `+json`,
  `application/x-www-form-urlencoded` through `EncodeForm`, `multipart/form-data` through
  `EncodeMultipart` (a `runtime.File` streams as a file part), a `runtime.File` body streamed,
  text and bytes as they are. With several body fields, the first one set is sent. A required body
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
- `EncodeForm` and `EncodeMultipart` write a struct as a form, in the shapes `DecodeForm` and
  `DecodeMultipart` read.
- `Send` sends with a `Doer` and reads the body; `DecodeSuccess` and `Decode` fill the targets of
  the response, `DecodeHeaders` a struct of typed headers; `APIError` is the error of a status
  outside 2xx.
