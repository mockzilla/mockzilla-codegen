# Adding a framework

A framework is a package under `internal/gen/server/framework/<name>` that implements
`framework.Framework` and holds the template of the router part in `templates/router.tmpl`.
Everything else, the service contract, the adapter, the errors and the scaffolds, is shared: the
adapter's handlers take the shape the framework asks for, and the router template registers them.
The interface is frozen; a framework that needs more is a reason to talk, not to widen it.

## The interface

```go
type Framework interface {
	Name() string
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
| `Imports` | every package the framework's templates write, its own module first; the templates get the first as `.Framework` and each as `.Packages.<name>` |
| `RoutePattern` | the route as the framework writes it, for a method and an OpenAPI path; `framework.ErrPattern` wrapped with the reason for a path the framework rejects, and `framework.ErrMethod` for a method it has no way to register, which `framework.CheckMethod` answers for a router with one function per method |
| `Conflicts` | the routes the framework holds together, in the order they are registered, and each route it cannot hold next to an earlier one with the reason; a framework may rewrite a kept route's `Pattern` and set its `Names` (see gin) |
| `Handler` | the shape of the adapter's handlers, see below |
| `PathParam` | the expression that reads a path parameter in a handler |
| `Templates` | an `fs.FS` whose `templates` folder holds `router.tmpl`, and `scaffold-main.tmpl` for a framework that serves in its own way |

`RoutePattern` and `Conflicts` are where the framework's rules live. Together they promise that
the generated router never panics and never answers a request with the wrong operation: a spec
path the framework rejects and a route it cannot hold next to an earlier one are left out with a
`route-dropped` warning instead. Write down what the framework panics on, or replaces without a
word, and test both sides: the patterns it takes, and for a framework that panics, that it panics
on each dropped route (see `stdhttp`). The `paths` example holds the shapes the OpenAPI corpus
brings: a parameter with a literal after it, two in one segment, a name with a colon, a literal
colon, a dollar sign, a trailing slash, TRACE, the same position named otherwise, escaped values.
Generate it for the new framework and look at what panics, what is silently replaced and what the
parameter values look like. Every rule in `docs/server.md` came from such a run.

`stdhttp` is the one framework whose `Conflicts` holds no rules of its own. Its router is the
standard library, so it registers the routes on a `ServeMux` and drops each one that panics. The
generator does not import the other routers, so their rules are written out.

The `framework` package holds what the rules share:

| Helper | Does |
|---|---|
| `Check`, `CheckClean`, `CheckMethod` | fail on what no router takes: no leading slash, an unclosed brace, a `*` that is not last; on a path that is not clean, for a router that cleans paths; on a method a router with one function per method has none for |
| `Colon` | writes routes of a router that takes parameters as `:name`, with `Literal`, `Name`, `Wildcard`, `IsWildcardNamed` and `IsPrefixAllowed` for what differs: echo, gin, hertz, fiber, beego and go-zero use it |
| `Brace` | writes routes of a router that takes `{name}` too, with the name and the wildcard it takes: gorilla-mux, kratos and fasthttp use it |
| `Rename`, `RestName` | write each `{name}` as the router names it and fail on two names it makes one; name the wildcard so no parameter has its name |
| `Escaping`, `EscapingColon`, `Rejecting`, `Same`, `Identifier`, `Unmarked` | the usual `Literal` and `Name` functions: escape or reject the characters the router reads as a parameter, keep a name, make it an identifier, or write a colon and a star in it as underscores |
| `Params`, `Shape` | the parameters of a path, and the path with their names blanked |
| `ConflictsByShape`, `ConflictsByKey` | drop every route that repeats the shape, or the key, of an earlier one: the rule of a router that replaces or shadows such a route without a word |
| `StaticFirst` | order routes for a router that takes the first match: at each position a literal, then a parameter next to a literal, the longer literal first, then a parameter alone, then the wildcard |
| `HTTPHandler`, `ContextHandler` | the two handler shapes, see below |

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
`PathParam` may use them, and it has to give the value unescaped once, whatever the router hands
over (`runtime.UnescapePath` for a router that cuts values from the raw path). A framework whose
handlers are `http.HandlerFunc`s returns `framework.HTTPHandler(s)`; one whose handlers take its
context returns `framework.ContextHandler` with the context's type spelled through `s.Import`,
and its handlers read their errors from the error handler: they write every failed request
themselves and return nil, so an operation answers the same under every framework.

A handler of the framework's context fits a framework whose context gives an
`http.ResponseWriter` and an `*http.Request` and whose middleware can wrap that writer, as echo's
`WrapMiddleware` and kratos's filters do. Every other framework takes `http.HandlerFunc`s, and its
router template serves the handler from the framework's context with a small function, `handle`,
that puts the path parameters on the request with `r.SetPathValue` so `PathParam` reads
`r.PathValue(name)`, then calls it with the framework's writer and request, or with a request and
a writer made from the framework's own for one that is not built on net/http (fasthttp, fiber,
hertz). That keeps the middleware contract whole: a middleware that wraps the writer, to log the
status say, sees every write. A server that reuses the memory of a request for the next one on
its connection, as fasthttp does, gets copies (`httpserver.DetachRequest`,
`httpserver.DetachPathValue`), and one that ends the process on a panic recovers in `wrap`
(`httpserver.Recover`).

## The router template

`router.tmpl` renders the router part with a `server.RouterView`:

| Field | Holds |
|---|---|
| `.Framework` | the name the framework's first import is written under in the file |
| `.Packages` | the name each import of `Imports` is written under, by the package's own name: `.Packages.http` for `net/http`, `.Packages.bcontext` for an import with that alias |
| `.Runtime` | the name of the runtime package, when the template writes it |
| `.Service`, `.Option`, `.Options`, `.NewOptions`, `.NewAdapter` | the shared names, as the file spells them |
| `.Routes` | one `RouteView` per operation: `.Method` as `Get`, `.HTTPMethod` as `GET`, `.Pattern` quoted, `.Operation`, `.IsShortcut` for a method routers have a function of their own for (all but TRACE and the methods a spec adds), and `.Names` quoted, the route's `Names` |
| `.User` | the config's `user-context` |

A package of `Imports` is imported into the file when the template, or the config's
`server.router-extra` override, writes it as `.Packages.<name>`, so a file imports what it uses and
nothing else.

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
whose `.Framework` and `.Packages` are those of the router view. `.WithRouter` names the option
that gives `NewRouter` a server the main built with its address and timeout; `.WithConfig` names
the option fiber and hertz declare instead, which makes the new server with them, so that the
middleware wraps unknown paths too. Keep the shape of the shared one: `run` serves until the process is told to stop,
then lets the requests in flight finish within `.Timeout`.

## Wiring

1. Add the framework to `server.Frameworks()` in `internal/gen/server/server.go`, and to the
   `enum` tag of `Server.Framework` in `pkg/config/blocks.go`; then `make schema`.
2. `internal/gen/server/server_test.go` renders `testdata/server.router.<name>.golden` for every
   framework, `server.adapter.<name>.golden` for one whose handlers take its context and
   `server.scaffold.main.<name>.golden` for one with a main of its own; write them with
   `UPDATE=1`.
3. Add a variant to `servers` in `test/integration/integration_test.go`, with the packages its
   code imports; its `Init` builds the router, which must not panic on any spec.
4. Add examples under `examples/server/<name>`: `basic`, `bodies`, `errors`, `params`, `paths`,
   `scaffolds` and `split`, with the specs and configs of an existing framework and
   `framework: <name>`. Their tests run the shared cases of `examples/server/internal/servertest`,
   and `basic` adds the framework's own: `WithRouter` with middleware, middleware on a new router,
   the adapter called alone, method not allowed. `paths` runs `servertest.Paths` and lists what the
   framework answers to the rest, route by route; a server that reuses memory between requests
   also runs `servertest.KeepAlive` on a real listener. A router that is no `http.Handler` gets a helper package under
   `examples/server/<name>/internal` that serves it as one, as `fasthttptest`, `hertztest` and
   `goframetest` do. Add the module to `examples/go.mod`, then `make examples`.
5. Document the framework in [server](server.md): its row in the router table and a section with
   its pattern rules, what it drops and how it differs from the others. Give it a row in the
   [routes table](templates.md#routes) of templates, with a `server.router-extra` line that you
   have built and served.
6. `make check`, then `make test-integration FRAMEWORKS=<name>` on the spec corpus, which should
   pass at least 95% of it.
