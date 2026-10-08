# Error types

`models.error-mapping` turns response types into Go errors. Each key is a generated type name, each
value the path of its message:

```yaml
models:
  error-mapping:
    ErrorResponse: error.message
    ValidationProblem: errors[].detail
```

```go
// Error returns the message at error.message.
func (e ErrorResponse) Error() string

// NewErrorResponse returns an ErrorResponse with message at error.message.
func NewErrorResponse(message string) ErrorResponse
```

## The path

A path is dotted JSON property names: `error.message`. A name ending in `[]` takes the first item of
that array: `errors[].detail`.

A path may go through a union. The union's shared properties come first. If the path is not there,
each variant is tried, and one that has the rest of the path is enough.

## Error and the constructor

| Generated | Behavior |
|---|---|
| `Error` | returns the value at the path, as text. A missing value gives the type name |
| the constructor, `NewErrorResponse` above | needs a string at the end of the path |

A union gets no constructor, since a message does not say which variant to build.

A field of an error type that would be named `Error` is renamed, since the method takes the name.

## Mappings that do not work

A name that is not a struct or a union, or a path that leads nowhere, is a warning. That type gets
no `Error` method.

## errors.As

Error types work with `errors.As`. The client puts a pointer in the error it returns, so the target
is a pointer too:

```go
var e *api.ErrorResponse
if errors.As(err, &e) {
	log.Print(e.Error())
}
```

## In the server

A service may return the error type as a value or as a pointer. The server finds both and answers
with the status the spec gives it.

A request the server turns away itself, such as one that fails validation, is answered with the
type the operation documents for 400, built by the constructor. See
[server errors](server.md#errors).
