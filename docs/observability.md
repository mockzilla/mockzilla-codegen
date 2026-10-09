# Observability

The generated code imports no tracing or metrics library. It gives you two hooks, and you bring the
library. The recipes below use OpenTelemetry's `otelhttp`.

| Side | Hook |
|---|---|
| server | `WithOperationMiddleware`, see [operation middleware](server.md#operation-middleware) |
| client | `WithHTTPClient`, see [HTTP client](client.md#http-client) |

## Operation names

Server and client put the name of the operation, `GetPet`, on the request's context.
`runtime.OperationID(ctx)` reads it. Use it as a span name, a metric label or a log field.

```go
func operationName(_ string, r *http.Request) string {
	return runtime.OperationID(r.Context())
}
```

## Server

```go
import (
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel/attribute"

	"github.com/mockzilla/mockzilla-codegen/pkg/runtime"
)

func labelOperation(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		labeler, _ := otelhttp.LabelerFromContext(r.Context())
		labeler.Add(attribute.String("operation", runtime.OperationID(r.Context())))
		next.ServeHTTP(w, r)
	})
}

tracing := otelhttp.NewMiddleware("", otelhttp.WithSpanNameFormatter(operationName))
router := NewRouter(svc, WithOperationMiddleware(tracing, labelOperation))
```

- Each request gets a server span named `GetPet`.
- The span covers reading the request, the service and writing the response. A 400 for a bad
  parameter is in it too.
- The service's `ctx` holds the span, so the spans the service starts are its children.
- `labelOperation` adds `operation="GetPet"` to otelhttp's request metrics, such as
  `http.server.request.duration`. Leave it out if you only want spans.

`otelhttp.NewMiddleware` also fits `WithMiddleware`. There it runs before the route is picked, so
its spans cannot be named after the operation.

## Client

```go
httpClient := &http.Client{
	Transport: otelhttp.NewTransport(http.DefaultTransport, otelhttp.WithSpanNameFormatter(operationName)),
}
c, err := NewPetClient(baseURL, WithHTTPClient(httpClient))
```

Each call gets a client span named `GetPet`.

## One trace across services

Set a propagator once, in the client's program and in the server's:

```go
otel.SetTextMapPropagator(propagation.TraceContext{})
```

The client then sends a `traceparent` header, and the server span joins the caller's trace.
