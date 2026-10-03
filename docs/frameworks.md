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
| `Imports` | every package the framework's templates write, its own module first; the templates get the first as `.Framework` and each as `.Packages.<name>` |
| `RoutePattern` | the route as the framework writes it, for a method and an OpenAPI path; `framework.ErrPattern` wrapped with the reason for a path the framework rejects, and `framework.ErrMethod` for a method it has no way to register, which `framework.CheckMethod` answers for a router with one function per method |
| `Conflicts` | the routes the framework holds together, in the order they are registered, and each route it cannot hold next to an earlier one with the reason |
| `Handler` | the shape of the adapter's handlers, see below |
| `PathParam` | the expression that reads a path parameter in a handler |
| `Templates` | an `fs.FS` with `router.tmpl`, and `scaffold-main.tmpl` for a framework that serves in its own way |

`RoutePattern` and `Conflicts` are where the framework's rules live. Together they promise that
the generated router never panics: a spec path the framework rejects and a route it cannot hold
next to an earlier one are left out with a `route-dropped` warning instead. Write down what the
framework panics on, or replaces without a word, and test both sides: the patterns it takes, and
for a framework that panics, that it panics on each dropped route (see `stdhttp`). Before that,
probe the framework itself with a throwaway program: register the shapes the OpenAPI corpus
brings, a parameter with a prefix or a suffix, two in one segment, a literal colon, a trailing
slash, a wildcard, the same shape twice with other names, a literal next to a parameter, and
see what panics, what is silently replaced and what the parameter values look like. Every rule
in `docs/server.md` came from such a run.

`stdhttp` is the one framework whose `Conflicts` holds no rules of its own. Its router is the
standard library, so it registers the routes on a `ServeMux` and drops each one that panics. The
generator does not import the other routers, so their rules are written out.

The `framework` package holds what the rules share:

| Helper | Does |
|---|---|
| `Check` | fails on what no router takes: no leading slash, an unclosed brace, a `*` that is not last |
| `Colon` | writes routes of a router that takes parameters as `:name`, with `Literal`, `Name`, `Wildcard` and `IsPrefixAllowed` for what differs: echo, gin, hertz, fiber, beego and go-zero use it |
| `Brace` | writes routes of a router that takes `{name}` too, with the wildcard it takes: gorilla-mux, kratos and fasthttp use it |
| `Escaping`, `Rejecting`, `Same`, `Identifier` | the usual `Literal` and `Name` functions: escape or reject the characters the router reads as a parameter, keep a name or make it an identifier |
| `Params`, `Shape` | the parameters of a path, and the path with their names blanked |
| `ConflictsByShape`, `ConflictsByKey` | drop every route that repeats the shape, or the key, of an earlier one: the rule of a router that replaces or shadows such a route without a word |
| `StaticFirst` | order routes for a router that takes the first match, literals before parameters and parameters before the wildcard at each position |

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

`Native` fits a framework whose context gives an `http.ResponseWriter` and an `*http.Request`
and whose middleware can wrap that writer, as echo's `WrapMiddleware` and kratos's filters do.
Every other framework stays `NetHTTP`, and its router template serves the handler from the
framework's context with a small function, `handle`, that puts the path parameters on the
request with `r.SetPathValue` so `PathParam` reads `r.PathValue(name)`, then calls it with the
framework's writer and request, or with a request and a writer made from the framework's own for
one that is not built on net/http (fasthttp, fiber, hertz). That keeps the middleware contract
whole: a middleware that wraps the writer, to log the status say, sees every write.

## The router template

`router.tmpl` renders the router part with a `server.RouterView`:

| Field | Holds |
|---|---|
| `.Framework` | the name the framework's first import is written under in the file |
| `.Packages` | the name each import of `Imports` is written under, by the package's own name: `.Packages.http` for `net/http`, `.Packages.bcontext` for an import with that alias |
| `.Service`, `.Option`, `.Options`, `.NewOptions`, `.NewAdapter` | the shared names, as the file spells them |
| `.Routes` | one `RouteView` per operation: `.Method` as `Get`, `.HTTPMethod` as `GET`, `.Pattern` quoted, `.Operation` |
| `.User` | the config's `user-context` |

A package of `Imports` is imported into the file when the template writes it as
`.Packages.<name>`, so a template imports what it uses and nothing else.

The template declares `WithRouter`, which takes a router of the framework and stores it in
`ServerOptions.Router`, and `NewRouter`, which registers every route on the router given or on a
new one and returns it. Keep the contract of the others: on a new router the middleware of
`WithMiddleware`, `func(http.Handler) http.Handler` values outermost first, wraps everything,
unknown paths too; on a given router it wraps the generated routes and nothing else. Most
templates keep a `wrap` closure that folds the middleware over an `http.Handler`, pass it to the
`register` closure as `route`, and on a new router also give the framework's not-found and
method-not-allowed handlers `wrap(http.NotFoundHandler())` and the like, so a middleware sees
those answers. The routes sit in the `register` closure, and
`{{- override "server.router-extra" .}}` comes right after them: it writes the text a config
gives that block, on lines of its own, so a config can add routes with the same names. Templates
hold no logic beyond `range` and `if` on the view's fields; anything else is computed in Go, or
is plain Go in the template, such as `handle`. An import the template needs and the view does not
offer is a reason to add it to `Imports`, never to call.

## The main scaffold

The shared `scaffold-main.tmpl` serves the router with an `http.Server`, which every router
that is an `http.Handler` fits. A framework whose server is served in its own way, such as fiber,
fasthttp, hertz and goframe, brings a `scaffold-main.tmpl` of its own in `Templates`, which
replaces the shared one. It renders with the `server.ScaffoldMainView` of the shared template,
whose `.Framework` and `.Packages` are those of the router view, and `.WithRouter` names the
option, so the main can build the server with its address and timeout and give it to
`NewRouter`. Keep the shape of the shared one: `run` serves until the process is told to stop,
then lets the requests in flight finish within `.Timeout`.

## Wiring

1. Add the framework to `server.Frameworks()` in `internal/gen/server/server.go`, and to the
   `enum` tag of `Server.Framework` in `pkg/config/blocks.go`; then `make schema`.
2. `internal/gen/server/server_test.go` renders `testdata/server.router.<name>.golden` for every
   framework, `server.adapter.<name>.golden` for a `Native` one and
   `server.scaffold.main.<name>.golden` for one with a main of its own; write them with
   `UPDATE=1`.
3. Add a variant to `servers` in `test/integration/integration_test.go`, with the packages its
   code imports; its `Init` builds the router, which must not panic on any spec.
4. Add examples under `examples/server/<name>`: `basic`, `bodies`, `errors`, `params`, `scaffolds`
   and `split`, with the specs and configs of an existing framework and `framework: <name>`. Their
   tests run the shared cases of `examples/server/internal/servertest`, and `basic` adds the
   framework's own: `WithRouter` with middleware, middleware on a new router, the adapter called
   alone, method not allowed. A router that is no `http.Handler` gets a helper package under
   `examples/server/<name>/internal` that serves it as one, as `fasthttptest`, `hertztest` and
   `goframetest` do. Add the module to `examples/go.mod`, then `make examples`.
5. Document the framework in [server](server.md): its row in the router table and a section with
   its pattern rules, what it drops and how it differs from the others, and the router it takes a
   `server.router-extra` route on in [templates](templates.md#blocks).
6. `make check`, then `make test-integration FRAMEWORKS=<name>` on the spec corpus, which should
   pass at least 95% of it.
