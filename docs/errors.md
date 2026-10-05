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

- A path is dotted JSON property names. A name ending in `[]` takes the first item of that array.
- `Error` returns the value at the path, as text. A missing value gives the type name.
- The constructor needs a string at the end of the path.
- A path may go through a union. The union's shared properties come first, else each variant is
  tried; one that has the rest of the path is enough. A union gets no constructor: a message does
  not say which variant to build.
- A name that is no struct or union, or a path that leads nowhere, is a warning. That type gets no
  `Error` method.
- A field of an error type that would be named `Error` is renamed, since the method takes the name.

Error types work with `errors.As`. The client puts a pointer in the error it returns, so the target
is a pointer too:

```go
var e *api.ErrorResponse
if errors.As(err, &e) {
	log.Print(e.Error())
}
```

A service may return the error type as a value or as a pointer. The server finds both and answers
with the status the spec gives it.
