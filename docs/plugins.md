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
Each name is a Go identifier. Reserving a name keeps the models away from it and does no more: a
name that two plugins, or a plugin and the generator, declare in one package is left to the
compiler.

`RequestOptionFields` go into the request options of every operation, after the parameter groups
and the bodies and before `RawRequest`. A field cannot take a name the options declare themselves:
the parameter groups (`PathParams`, `Query`, `QueryString`, `Headers`, `Cookies`), a name that
starts with `Body`, `RawRequest` or `Validate`. Two plugins cannot add the same field.

The type of a field follows the rules of a [`TypeRef`](#the-api). With an import path it is an
identifier, or a pointer, slice, array, map or channel around one. A package needs its import
path and has to be a package name:

```go
TypeRef{Name: "func() any"}                                                    // written as it is
TypeRef{Name: "*Span", Package: "trace", ImportPath: "example.com/trace"}      // *trace.Span
TypeRef{Name: "Option[Pet]", Package: "opt", ImportPath: "example.com/opt"}    // an error
TypeRef{Name: "*Span", Package: "trace"}                                       // an error: no import path
TypeRef{Name: "*Span", Package: "open-trace", ImportPath: "example.com/trace"} // an error: no package name
```

### Contribute

```go
type Contribution struct {
	Parts     []PartSource            // placed with output.files as plugin.<name>.<part>
	Scaffolds map[ScaffoldKind]string // replacement templates of the scaffold files
	Funcs     template.FuncMap        // funcs for this plugin's templates only
}

type PartSource struct {
	Name     string   // [a-z][a-z0-9]*
	Template string   // a text/template
	Data     any      // what the template runs on
	Imports  []Import // packages the code does not name, {Path, Alias}
}
```

Every part must be placed: list `plugin.<name>.<part>`, `plugin.<name>` or `plugin` in
`output.files`. A part gets the same header, import declaration, formatting and layout rules as a
built-in part, and can share a file with any other part.

`Imports` are for the packages the code needs and does not name: one imported for its side
effects, under `_`, or one whose names the code writes bare, under `.`.

```go
Imports: []codegen.Import{{Path: "embed", Alias: "_"}}
```

For a package the code names, call [`import`](#templates) in the template. It returns the name the
file gave the package, which is not always the package's own: two imports of one file cannot share
a name, so the second one gets a number, such as `models2`. An alias in `Imports` other than `_`
and `.` is an identifier and only a wish for that name. An import needs a path.

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

// Client is what the bodies are sent with.
var Client = {{$http}}.DefaultClient
```

A part can use the types of any package the generator writes, wherever the config places it. That
placing decides what the part's file imports. When it makes two output folders import each other,
`Generate` fails with the cycle and the part that closes it, as it does for the built-in parts:

```
import cycle: api -> models -> api (models.responses uses example.com/work/models, plugin.sample.register uses example.com/work/api)
```

Move the part to a folder that the folders it uses do not import.

A func in `Funcs` replaces one of the same name, `expr` and `import` included, in the templates of
that plugin only. So a plugin can pass a whole library of funcs, and a func the generator gains
later never changes what a plugin's template calls. `Funcs` has to be a map `text/template` takes:
a name is an identifier, and a value is a func that returns one value, or one value and an error.

The rules for the built-in templates apply: decide everything in Go and keep the template to
`range` and `if` over the data.

A key that a map does not have is an error, where `text/template` alone writes `<no value>` into
the code: `{{.owner}}` fails on a `UserContext` without `owner`. Ask for a key that may be
missing with `index`, as in `{{with index . "owner"}}// Owned by {{.}}.{{end}}`.

## The API

`Contribute` sees the generated code once names are resolved and files are laid out. The struct
only ever gains fields. Each plugin gets its own copy, down to the nested maps and lists of
`UserContext`. What a plugin changes in it reaches no other plugin, no template and not the config.

```go
type API struct {
	Package     string         // package of output.file, as package or output.packages names it
	Operations  []Operation    // every operation and webhook, in spec order
	Types       []TypeRef      // every declared model type
	UserContext map[string]any // a copy of the config's user-context
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

`TypeRef` is a Go type: `Name` as the package that declares it writes it, and `Package` and
`ImportPath` of the identifier in it. With an import path, `Name` is an identifier, or a pointer,
slice, array, map or channel around one (`Pet`, `[]Pet`, `*Pet`, `map[string]Pet`): the package
goes before that identifier, and a map key or an array length is written as it is. A generic type,
a func type or a name that is qualified already cannot carry an import path. `Package`, when set,
is the name of the package: an identifier other than `_`. Without an import path, the type needs
no import and `Name` is written as it is (`string`, `func() any`). A type the generator declares
has no import path when the output is one package outside a module.

`Expr(from)` writes a type as the package with import path `from` spells it; in a template, `expr`
does the same for the file being written and adds the import. `expr` fails on a type whose `Name`
cannot carry its import path, in every file, so a template does not start to fail when the output
is split into packages. `Expr` runs no check. To write a type around one of another package, put
it together in the template: `Page[{{expr .Body}}]`.

## Scaffold data

A replacement scaffold template runs on the built-in view. Names are written as the scaffold's
file spells them; the package fields hold the name each package is imported under. The file
imports what the replacement's text names and nothing else of the view, so a replacement can leave
out any field. Import cycles are checked on those imports too, as for a part.

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

Without a plugin, the config can replace a closed list of blocks in the built-in templates, which
are empty until it does. A value is the template text. A value of one line that ends in `.tmpl`
is the path of a file that holds the text, relative to the config:

```yaml
templates:
  server.service-header: ./templates/header.tmpl
  server.request-options-extra: Tenant string
  server.router-extra: |
    r.Get("/health", health)
    r.Get("/owner", {{.User.handler}})
user-context:
  handler: ownerHandler
```

The text of a block goes on lines of its own, without the blank lines around it, so it needs no
line break at its start or its end. `server.service-header` is followed by a blank line, which
keeps it out of the comment of the interface. A block whose text comes out empty adds nothing.

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
`handle(route(h))`, or `route(h)` alone on gorilla-mux and go-zero.

`user-context` is available as `.User` in every block and as `API.UserContext` to plugins. A key
it does not have is an error: `{{.User.team}}` fails in a config that sets no `team`. Ask for a
key that may be missing with `index`, as in `{{if index .User "team"}}`.

These are config errors:

- a block that does not exist, with the list of those that do
- a `server` block in a config without `server`
- a value that starts with `./` or `../` and does not end in `.tmpl`: it would be taken as the
  text and written into the code
