# Plugins

A plugin adds generated code of its own next to the built-in parts, from Go, without forking the
templates. It is a value that implements `codegen.Plugin`, passed to `Generate` with
`WithPlugins`:

```go
res, err := codegen.Generate(ctx, cfg, codegen.WithPlugins(sample.Plugin{}))
```

Plugins are a Go API: the CLI has none. A tool built on the generator embeds the plugin and calls
`Generate` itself. `examples/plugin/sample` is a complete plugin, and `examples/plugin/basic` the
output it generates.

## The interface

```go
type Plugin interface {
	Name() string                               // [a-z][a-z0-9]*, names the parts plugin.<name>.<part>
	Reserve() Reservations                      // before names are resolved
	Contribute(api *API) (*Contribution, error) // after names are resolved
}
```

Plugins run in the order `WithPlugins` gives them. Every error a plugin returns, and every
problem with what it gives, comes back from `Generate` wrapped with the plugin's name, as
`codegen.ErrPlugin`.

### Reserve

```go
type Reservations struct {
	Idents              []string    // package-level names the plugin's parts declare
	RequestOptionFields []FieldSpec // fields added to every <Op>ServiceRequestOptions
}

type FieldSpec struct {
	Name string  // an exported identifier
	Type TypeRef // for example {Name: "func() any"}
	Doc  string  // the comment above the field, empty for none
}
```

`Idents` are reserved before the models are named, so a schema called `Routes` becomes
`RoutesSchema` when a plugin declares `Routes`. List every package-level name the parts and
scaffolds declare; the built-in scaffold names such as `ErrNotImplemented` are reserved already.

`RequestOptionFields` go into the request options of every operation, after the parameter groups
and the bodies and before `RawRequest`. A field cannot take a name the options declare themselves:
the parameter groups (`PathParams`, `Query`, `QueryString`, `Headers`, `Cookies`), a name that
starts with `Body`, `RawRequest` or `Validate`. Two plugins cannot add the same field.

### Contribute

```go
type Contribution struct {
	Parts     []PartSource            // placed with output.files as plugin.<name>.<part>
	Scaffolds map[ScaffoldKind]string // replacement templates of the scaffold files
	Funcs     template.FuncMap        // extra funcs for this plugin's templates only
}

type PartSource struct {
	Name     string   // [a-z][a-z0-9]*
	Template string   // a text/template
	Data     any      // what the template runs on
	Imports  []Import // packages the code needs, {Path, Alias}
}
```

Every part must be placed: list `plugin.<name>.<part>`, `plugin.<name>` or `plugin` in
`output.files`. A part gets the same header, import declaration, formatting and layout rules as a
built-in part, and can share a file with any other part.

`Scaffolds` replace the template of a scaffold file the config writes (`ScaffoldService`,
`ScaffoldMiddleware`, `ScaffoldMain`); a replacement for a scaffold the config does not name is
ignored. The replacement runs on the same data as the built-in template, listed under
[Scaffold data](#scaffold-data). One scaffold can be replaced by one plugin.

### Templates

A plugin template is a `text/template`. It sees the generator's funcs (`comment`, `quote`, `tag`,
`lower`, `ucFirst`), the plugin's own `Funcs`, and two funcs bound to the file the part lands in:

| Func | Does |
|---|---|
| `expr <TypeRef>` | writes the type as the file spells it, qualified and imported when it lives in another package |
| `import <path>` | imports the path and returns the name to qualify with |

```
{{- $http := import "net/http"}}
// Bodies makes an empty success body per operation.
var Bodies = map[string]func() any{
{{- range .}}
	{{quote .ID}}: func() any { return new({{expr .Body}}) },
{{- end}}
}
```

The rules for the built-in templates apply: decide everything in Go and keep the template to
`range` and `if` over the data.

## The API

`Contribute` sees the generated code once names are resolved and files are laid out. The struct
only ever gains fields.

```go
type API struct {
	Package     string         // package of output.file
	Operations  []Operation    // every operation and webhook, in spec order
	Types       []TypeRef      // every declared model type
	UserContext map[string]any // the config's user-context
}

type Operation struct {
	ID             string   // the Go name, which handlers report as the operation ID
	Method, Path   string
	Summary        string
	Tags           []string
	HasOptions     bool     // takes parameters or a body
	IsRouted       bool     // the router registers it: not a webhook, not dropped
	RequestOptions       TypeRef  // <Op>ServiceRequestOptions, empty without a server block
	ResponseData         TypeRef  // <Op>ResponseData, empty without a server block
	ClientRequestOptions TypeRef  // <Op>RequestOptions, empty without a client block or for a webhook
	ClientResponse       TypeRef  // <Op>Response, empty unless client.with-response is set
	Success              *Success // the first 2xx response, nil without one
}

type Success struct {
	Status      int     // the code, or the start of a range such as 2XX
	ContentType string  // of the JSON body, else the first one; empty without a body
	Body        TypeRef // the type the response constructor takes; empty without a body
	IsRaw       bool    // the body has no schema: any, string or []byte
}
```

`TypeRef` is a Go type: `Name` as the package that declares it writes it (`Pet`, `[]Pet`,
`*Pet`, `func() any`), and `Package` and `ImportPath` of the identifier in it, empty when it needs
no import. `Expr(from)` writes it as the package with import path `from` spells it; in a template,
`expr` does the same for the file being written and adds the import.

## Scaffold data

A replacement scaffold template runs on the built-in view. Names are written as the scaffold's
file spells them; the package fields hold the name each package is imported under.

Service (`ScaffoldService`):

| Field | Holds |
|---|---|
| `Name` | the service struct, `server.name` |
| `Interface` | the service interface |
| `Context`, `Errors` | the `context` and `errors` packages |
| `Operations` | one entry per operation: `Name`, `Method`, `Path`, `Options`, `Data` |

Middleware (`ScaffoldMiddleware`): `HTTP`, `Slog`, `Time`, `Rand`, `Debug`, the packages.

Main (`ScaffoldMain`):

| Field | Holds |
|---|---|
| `Context`, `HTTP`, `Slog`, `OS`, `Signal`, `Syscall` | the packages |
| `NewRouter`, `NewService`, `WithMiddleware` | the functions main calls |
| `Middleware` | the middleware expressions, empty without the middleware scaffold |
| `Port`, `Timeout` | the port and the timeout expression |

## Template overrides

Without a plugin, the config can replace a closed list of blocks in the built-in templates. A
value is the template text, or the path of a `.tmpl` file relative to the config:

```yaml
templates:
  server.service-header: ./templates/header.tmpl
  server.request-options-extra: "\n\tTenant string"
user-context:
  owner: platform
```

| Block | Where | Data |
|---|---|---|
| `server.service-header` | before the service interface | the service |
| `server.request-options-extra` | in every request options struct, before `RawRequest` | the operation |
| `server.response-data-extra` | in every response data struct, after `Body` | the operation |
| `server.router-extra` | in the router's registration, after the routes | the router |

In `server.router-extra`, a chi route goes on `r`; a std-http route on `mux`, its handler wrapped
with `route`; an echo route on `e`, with `m...` as its middleware; a kratos route on `r`, a kratos
router. On the other frameworks the route goes on the router the `register` closure of
`router.tmpl` names, `e` for gin, `app` for fiber and iris, `r` for gorilla-mux, fasthttp, beego
and go-zero, `h` for hertz and `s` for goframe, and its handler is an `http.Handler` wrapped as
`handle(route(h))`, or `route(h)` alone on gorilla-mux and go-zero. An unknown block is a config
error that lists the blocks. The blocks inside a struct or a function start after the line before
them, so their text begins with a newline. `user-context` is
available as `.User` in every block and as `API.UserContext` to plugins.
