# Adding a framework

A framework is a Go package under `internal/gen/server/framework/<name>`. It holds two things:

- an implementation of the `framework.Framework` interface
- the router template, `templates/router.tmpl`

Everything else is shared by all frameworks: the service contract, the adapter, the errors and the
scaffolds. The adapter writes its handlers in the shape the framework asks for, and the router
template registers them.

The interface is meant to stay as it is. If your framework needs something it does not offer, open
an issue first, so we can find a way that works for the other frameworks too.

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
| `Imports` | every package the framework's templates write, its own module first. The templates get the first as `.Framework` and each as `.Packages.<name>`. |
| `RoutePattern` | the route as the framework writes it, for a method and an OpenAPI path. It can fail, see below. |
| `Conflicts` | the routes the framework holds together, and the ones it cannot. See below. |
| `Handler` | the shape of the adapter's handlers, see [Handler shape](#handler-shape) |
| `PathParam` | the expression that reads a path parameter in a handler |
| `Templates` | an `fs.FS` whose `templates` folder holds `router.tmpl`. A framework that serves in its own way adds `scaffold-main.tmpl`. |

### What RoutePattern returns

`RoutePattern` fails in two cases:

| Error | When |
|---|---|
| `framework.ErrPattern`, wrapped with the reason | the framework rejects the path |
| `framework.ErrMethod` | the framework has no way to register the method |

For a router with one function per method, `framework.CheckMethod` gives the second answer.

### What Conflicts returns

- the routes the framework holds together, in the order they are registered
- each route it cannot hold next to an earlier one, with the reason

A framework may rewrite the `Pattern` of a kept route and set its `Names` (see gin).

## Route rules

`RoutePattern` and `Conflicts` hold the rules of the framework. Together they make two promises
about the generated router:

- it never panics
- it never answers a request with the wrong operation

A spec path the framework rejects, or a route it cannot hold next to an earlier one, is left out
with a `route-dropped` warning.

To find the rules, write down what the router panics on and what it replaces without a word. Then
test both sides: the patterns it takes, and, for a router that panics, that it panics on each route
you drop (see `stdhttp`).

The `paths` example holds path shapes found in real-world specs:

- a parameter with a literal after it
- two parameters in one segment
- a name with a colon, a literal colon, a dollar sign
- a trailing slash
- the TRACE method
- the same position named differently in two paths
- escaped values

Generate it for the new framework. Look at what panics, what is replaced without a word, and what
the parameter values look like. Every rule in [server](server.md) came from such a run.

### stdhttp

`stdhttp` is the one framework whose `Conflicts` holds no rules of its own. Its router is the
standard library, so it registers the routes on a `ServeMux` and drops each one that panics. The
generator does not import the other routers, so their rules are written out.

### Helpers

The `framework` package holds what the rules share:

| Helper | Does |
|---|---|
| `Check` | fails on what no router takes: no leading slash, an unclosed brace, a `*` that is not last |
| `CheckClean` | fails on a path that is not clean, for a router that cleans paths |
| `CheckMethod` | fails on a method that a router with one function per method has no function for |
| `Colon` | writes the routes of a router that takes parameters as `:name`. Its fields `Literal`, `Name`, `Wildcard`, `IsWildcardNamed` and `IsPrefixAllowed` cover what differs. Echo, gin, hertz, fiber, beego and go-zero use it. |
| `Brace` | writes the routes of a router that takes `{name}` too. You pass it the functions that write the names and the wildcard. Gorilla-mux, kratos and fasthttp use it. |
| `Rename` | writes each `{name}` as the router names it, and fails on two names it makes one |
| `RestName` | names the wildcard so that no parameter has its name |
| `Escaping`, `EscapingColon` | `Literal` functions that escape the characters the router reads as a parameter |
| `Rejecting` | a `Literal` function that rejects those characters |
| `Same`, `Identifier`, `Unmarked` | `Name` functions: keep a name, make it an identifier, or write a colon and a star in it as underscores |
| `Params` | the parameters of a path |
| `Shape` | the path with its parameter names blanked |
| `ConflictsByShape`, `ConflictsByKey` | drop every route that repeats the shape, or the key, of an earlier one. They fit a router that replaces or shadows such a route without a word. |
| `StaticFirst` | orders routes for a router that takes the first match. At each position the order is: a literal, then a parameter next to a literal (the longer literal first), then a parameter alone, then the wildcard. |
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

The adapter template writes every handler as `func (a *HTTPAdapter) Op<Signature> {`, then the
prologue, then the decoding of the request, with `Return` on every early exit.

### Names in a handler

The body of a handler always names the response writer `w`, the request `r` and the framework's
context `c`. So `PathParam` may use them.

`PathParam` has to give the value unescaped once, whatever the router hands over. For a router that
cuts values from the raw path, use `runtime.UnescapePath`.

### Two shapes

- A framework whose handlers are `http.HandlerFunc`s returns `framework.HTTPHandler(s)`.
- A framework whose handlers take its context returns `framework.ContextHandler`, with the
  context's type spelled through `s.Import`.

Handlers of the second kind read their errors from the error handler. They write every failed
request themselves and return nil, so an operation answers the same under every framework.

### When a handler takes the framework's context

A handler that takes the framework's context fits a framework when both hold:

- its context gives an `http.ResponseWriter` and an `*http.Request`
- its middleware can wrap that writer, as echo's `WrapMiddleware` and kratos's filters do

### Every other framework

Every other framework takes `http.HandlerFunc`s. Its router template serves the handler from the
framework's context with a small function, `handle`. `handle` does two things:

1. It puts the path parameters on the request with `r.SetPathValue`, so `PathParam` reads
   `r.PathValue(name)`.
2. It calls the handler with the framework's writer and request. For a framework that is not built
   on net/http (fasthttp, fiber, hertz), it calls the handler with a request and a writer made from
   the framework's own.

This keeps the middleware contract whole: a middleware that wraps the writer, to log the status for
example, sees every write.

Two more cases:

- A server that reuses the memory of a request for the next one on its connection, as fasthttp
  does, gets copies (`httpserver.DetachRequest`, `httpserver.DetachPathValue`).
- A server that ends the process on a panic recovers in `wrap` (`httpserver.Recover`).

## The router template

`router.tmpl` renders the router part with a `server.RouterView`:

| Field | Holds |
|---|---|
| `.Framework` | the name the framework's first import is written under in the file |
| `.Packages` | the name each import of `Imports` is written under, looked up by the package's own name: `.Packages.http` for `net/http`, `.Packages.bcontext` for an import with that alias |
| `.Runtime` | the name of the runtime package, when the template writes it |
| `.Service`, `.Option`, `.Options`, `.NewOptions`, `.NewAdapter` | the shared names, as the file spells them |
| `.Routes` | one `RouteView` per operation, see below |
| `.User` | the config's `user-context` |

A `RouteView` has these fields:

| Field | Holds |
|---|---|
| `.Method` | the method as `Get` |
| `.HTTPMethod` | the method as `GET` |
| `.Pattern` | the route, quoted |
| `.Operation` | the operation, the name of its handler on the adapter |
| `.IsShortcut` | true for a method routers have a function of their own for (all but TRACE and the methods a spec adds) |
| `.Names` | the route's `Names`, quoted |

A package of `Imports` is imported into the file when the template, or the config's
`server.router-extra` override, writes it as `.Packages.<name>`. So a file imports what it uses and
nothing else.

### What the template declares

- `WithRouter` takes a router of the framework and stores it in `ServerOptions.Router`.
- `NewRouter` registers every route on the router given, or on a new one, and returns it.

### Middleware

Keep the contract of the other frameworks. `WithMiddleware` takes `func(http.Handler) http.Handler`
values, outermost first.

- On a new router, the middleware wraps everything, unknown paths too.
- On a given router, it wraps the generated routes and nothing else.

Most templates keep a `wrap` closure that folds the middleware over an `http.Handler`. They pass it
to the `register` closure as `route`. On a new router they also give the framework's not-found and
method-not-allowed handlers `wrap(http.NotFoundHandler())` and the like, so that a middleware sees
those answers.

### The register closure

The routes sit in the `register` closure. `{{- override "server.router-extra" .}}` comes right after
them. It writes the text a config gives that block, on lines of its own, so a config can add routes
with the same names.

### Logic in the template

Templates hold no logic beyond `range` and `if` on the view's fields. Anything else is computed in
Go, or is plain Go in the template, such as `handle`.

If the template needs an import the view does not offer, add it to `Imports`.

## The main scaffold

The shared `scaffold-main.tmpl` serves the router with an `http.Server`. Every router that is an
`http.Handler` fits it.

A framework whose server is served in its own way, such as fiber, fasthttp, hertz and goframe,
brings a `scaffold-main.tmpl` of its own in `Templates`. It replaces the shared one.

It renders with the `server.ScaffoldMainView` of the shared template. Its `.Framework` and
`.Packages` are those of the router view. Two more fields name an option:

| Field | Names |
|---|---|
| `.WithRouter` | the option that gives `NewRouter` a server the main built with its address and timeout |
| `.WithConfig` | the option fiber and hertz declare instead. It makes the new server from the main's settings, so that the middleware wraps unknown paths too. |

Keep the shape of the shared one: `run` serves until the process is told to stop, then lets the
requests in flight finish within `.Timeout`.

## Wiring

1. Add the framework to `server.Frameworks()` in `internal/gen/server/server.go`, and to the
   `enum` tag of `Server.Framework` in `pkg/config/blocks.go`. Then run `make schema`.
2. `internal/gen/server/server_test.go` renders these golden files. Write them with `UPDATE=1`.
   - `testdata/server.router.<name>.golden` for every framework
   - `server.adapter.<name>.golden` for one whose handlers take its context
   - `server.scaffold.main.<name>.golden` for one with a main of its own
3. Add a variant to `servers` in `test/integration/integration_test.go`, with the packages its code
   imports. Its `Init` builds the router, which must not panic on any spec.
4. Add examples under `examples/server/<name>`:
   - Add `basic`, `bodies`, `errors`, `params`, `paths`, `scaffolds` and `split`, with the specs
     and configs of an existing framework and `framework: <name>`.
   - Their tests run the shared cases of `examples/server/internal/servertest`.
   - `basic` adds the framework's own cases: `WithRouter` with middleware, middleware on a new
     router, the adapter called alone, method not allowed.
   - `paths` runs `servertest.Paths` and lists what the framework answers to the rest, route by
     route.
   - A server that reuses memory between requests also runs `servertest.KeepAlive` on a real
     listener.
   - A router that is not an `http.Handler` gets a helper package under
     `examples/server/<name>/internal` that serves it as one, as `fasthttptest`, `hertztest` and
     `goframetest` do.
   - Add the module to `examples/go.mod`, then run `make examples`.
5. Document the framework in [server](server.md): its row in the router table and a section with
   its pattern rules, what it drops and how it differs from the others. Give it a row in the
   [routes table](templates.md#routes) of templates, with a `server.router-extra` line that you
   have built and served.
6. Run `make check`, then `make test-integration FRAMEWORKS=<name>` on the spec corpus. At least 95%
   of it should pass.
