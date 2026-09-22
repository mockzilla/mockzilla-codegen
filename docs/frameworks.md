# Adding a framework

A framework is a package under `internal/gen/server/framework/<name>` that implements
`framework.Framework` and holds the template of the router part. Everything else, the service
contract, the adapter, the errors and the scaffolds, is shared: the adapter's handlers take the
shape the framework asks for, and the router template registers them. The interface is frozen;
a framework that needs more is a reason to talk, not to widen it.

## The interface

```go
type Framework interface {
	Name() string
	Family() Family
	Imports() []gomodel.Import
	RoutePattern(method, path string) (string, error)
	Conflicts(routes []Route) ([]Route, []Conflict)
	Handler(s *gocode.Scope) Handler
	PathParam(s *gocode.Scope, name string) string
	Templates() fs.FS
}
```

| Method | Answers |
|---|---|
| `Name` | the value of `server.framework`, in kebab case: `std-http` |
| `Family` | `NetHTTP` when handlers are `http.HandlerFunc`s, `Native` when they have the framework's own shape |
| `Imports` | the framework's module, first; the router template gets its package name as `.Framework` |
| `RoutePattern` | the route as the framework writes it, for a method and an OpenAPI path; `framework.ErrPattern` wrapped with the reason for a path the framework rejects |
| `Conflicts` | the routes the framework holds together, and each route it cannot hold next to an earlier one with the reason |
| `Handler` | the shape of the adapter's handlers, see below |
| `PathParam` | the expression that reads a path parameter in a handler |
| `Templates` | an `fs.FS` with `router.tmpl` |

`RoutePattern` and `Conflicts` are where the framework's rules live. Together they promise that
the generated router never panics: a spec path the framework rejects and a route it cannot hold
next to an earlier one are left out with a `route-dropped` warning instead. Write down what the
framework panics on, or replaces without a word, and test both sides: the patterns it takes, and
for a framework that panics, that it panics on each dropped route (see `stdhttp`). The helpers
`framework.Params` and `framework.Shape` read the parameters of a path and blank their names;
`framework.ConflictsByShape` is the rule of a router that keys parameters by position and lets a
later route replace an earlier one, which chi and echo share.

## Handler shape

```go
type Handler struct {
	Signature string // after the name: "(w http.ResponseWriter, r *http.Request)" or "(c echo.Context) error"
	Prologue  string // binds w and r when the parameters are others: "w, r := c.Response(), c.Request()"
	Return    string // leaves the handler early: "return" or "return nil"
	Epilogue  string // ends the handler: "" or "return nil"
}
```

The adapter template writes every handler as `func (a *HTTPAdapter) Op<Signature> {`, the
prologue, the decoding of the request, and `Return` on every early exit. The body of a handler
always names the response writer `w`, the request `r` and the framework's context `c`, so
`PathParam` may use them. A `NetHTTP` framework returns `framework.HTTPHandler(s)`; a `Native`
one writes its own, with the framework's types spelled through `s.Import`, and reads its errors
from the error handler: the handlers write every failed request themselves and return nil, so an
operation answers the same under every framework.

## The router template

`router.tmpl` renders the router part with a `server.RouterView`:

| Field | Holds |
|---|---|
| `.Framework` | the name the framework's first import is written under in the file |
| `.Service`, `.Option`, `.Options`, `.NewOptions`, `.NewAdapter` | the shared names, as the file spells them |
| `.Routes` | one `RouteView` per operation: `.Method` as `Get`, `.HTTPMethod` as `GET`, `.Pattern` quoted, `.Operation` |
| `.User` | the config's `user-context` |

The template declares `WithRouter`, which takes a router of the framework and stores it in
`ServerOptions.Router`, and `NewRouter`, which registers every route on the router given or on a
new one and returns it. Keep the contract of the others: on a new router the middleware of
`WithMiddleware`, `func(http.Handler) http.Handler` values outermost first, wraps everything,
unknown paths too; on a given router it wraps the generated routes and nothing else. The routes
sit in a `register` closure, and the block `server.router-extra` comes right after them, so a
config can add routes with the same names. Templates hold no logic beyond `range` and `if` on
the view's fields; anything else is computed in Go. Only `.Framework` and its own names are
imported; an import the template needs and the view does not offer is a reason to precompute a
field, never to call.

## Wiring

1. Add the framework to `server.Frameworks()` in `internal/gen/server/server.go`, and to the
   `enum` tag of `Server.Framework` in `pkg/config/blocks.go`; then `make schema`.
2. `internal/gen/server/server_test.go` renders `testdata/server.router.<name>.golden` for every
   framework, and `server.adapter.<name>.golden` for a `Native` one; write them with `UPDATE=1`.
3. Add a variant to `servers` in `test/integration/integration_test.go`, with the module its code
   imports; its `Init` builds the router, which must not panic on any spec.
4. Add examples under `examples/server/<name>`: `basic`, `bodies`, `errors`, `params`, `scaffolds`
   and `split`, with the specs and configs of an existing framework and `framework: <name>`. Their
   tests run the shared cases of `examples/server/internal/servertest`, and `basic` adds the
   framework's own: `WithRouter` with middleware, the adapter called alone, method not allowed.
   Add the module to `examples/go.mod`, then `make examples`.
5. Document the framework in [server](server.md): its row in the router table and a section with
   its pattern rules and what it drops.
6. `make check`, then `make test-integration FRAMEWORKS=<name>` on the spec corpus, which should
   pass at least 95% of it.
