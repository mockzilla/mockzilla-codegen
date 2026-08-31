# Server

A `server` block in the config generates the service contract: an interface with one method per
operation, a request options type each method receives, and a response data type each returns.
The HTTP side, which decodes requests and calls the service, is generated for the framework the
block names.

```yaml
server:
  framework: chi
  name: Pets   # the interface is PetsInterface; defaults to Service
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
  type, named after it: `BodyJSON`, `BodyForm`, `BodyMultipart`, `BodyText`.
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
  declare headers.

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
