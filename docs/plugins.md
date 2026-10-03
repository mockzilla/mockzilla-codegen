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
	Name() string                                                         // [a-z][a-z0-9]*, names the parts plugin.<name>.<part>
	Reserve(ctx context.Context, in *ReserveInput) (*Reservations, error) // before names are resolved
	Contribute(ctx context.Context, api *API) (*Contribution, error)      // after names are resolved
}
```

Plugins run in the order `WithPlugins` gives them. Both methods get the context `Generate` was
given, and a nil result from either adds nothing. Every error a plugin returns, and every
problem with what it gives, comes back from `Generate` wrapped with the plugin's name, as
`codegen.ErrPlugin`. The exception is an import cycle that its code closes: `Generate` reports
it like any other cycle, with the parts that close it.

### Reserve

```go
type ReserveInput struct {
	UserContext map[string]any // a copy of the config's user-context
}

type Reservations struct {
	Idents []string // package-level names the plugin's parts declare
}
```

`Reserve` runs before the spec is read, so its input holds the config's `user-context` alone.
Like the [API](#the-api), the struct only ever gains fields, and each plugin gets a copy of its
own.

`Idents` are reserved before the models are named, so a schema called `Routes` becomes
`RoutesSchema` when a plugin declares `Routes`. List every package-level name the parts and
scaffolds declare; the built-in scaffold names such as `ErrNotImplemented` are reserved already.
Each name is a Go identifier. Reserving a name keeps the models away from it and does no more: a
name that two plugins, or a plugin and the generator, declare in one package is left to the
compiler.

### Contribute

```go
type Contribution struct {
	Parts               []PartSource            // named plugin.<name>.<part> in output.files
	RequestOptionFields map[string][]FieldSpec  // fields of <Op>ServiceRequestOptions, by operation ID
	Scaffolds           map[ScaffoldKind]string // replacement templates of the scaffold files
	Funcs               template.FuncMap        // funcs for this plugin's templates only
}

type PartSource struct {
	Name     string   // [a-z][a-z0-9]*
	Template string   // a text/template
	Data     any      // what the template runs on
	Imports  []Import // packages the code does not name, {Path, Alias}
}

type FieldSpec struct {
	Name string  // an exported identifier
	Type TypeRef // for example {Name: "func() any"}
	Doc  string  // the comment above the field, empty for none
}
```

A part is placed like a built-in one. It goes to `output.file`, unless a selector in
`output.files` moves it: `plugin.<name>.<part>` names one part, `plugin.<name>` the parts of one
plugin and `plugin` those of every plugin, and the closest selector wins.

```yaml
output:
  file: ./api/gen.go
  files:
    ./api/plugins.go: [plugin]                    # every part of every plugin
    ./api/register.go: [plugin.sample.register]   # but this one
```

A part gets the same header, import declaration, formatting and layout rules as a built-in part,
and can share a file with any other part. In a file, the parts of plugins come below the built-in
ones, in the order `WithPlugins` and `Parts` give them.

What a part writes is Go declarations, without a package clause. The file puts them on lines of
their own below a blank line, so the text needs no line break at its start or its end, and a part
that writes nothing adds nothing. Text that Go cannot parse is an error, in a file that is not
formatted too. It names the plugin and the part, and shows the lines around the problem, counted
from the first line the part wrote:

```
./api/register.go: plugin sample: parse generated code: plugin.sample.register:2:28: missing ',' in parameter list (and 2 more errors)
     1 | // Register mounts the routes on r.
>    2 | func Register(r chi.Router {
     3 | 	r.Get("/routes", list)
     4 | 	r.Get("/health", health)
     5 | }
```

The text of a replaced scaffold is checked the same way.

`Imports` are for the packages the code needs and does not name: one imported for its side
effects, under `_`, or one whose names the code writes bare, under `.`.

```go
Imports: []codegen.Import{{Path: "embed", Alias: "_"}}
```

For a package the code names, call [`import`](#templates) in the template. It returns the name the
file gave the package, which is not always the package's own: two imports of one file cannot share
a name, so the second one gets a number, such as `models2`. An alias in `Imports` other than `_`
and `.` is an identifier and only a wish for that name. An import needs a path.

`RequestOptionFields` adds fields to the request options of the server, operation by operation. A
key is the `ID` of an operation of the [API](#the-api), which is its Go name, such as `ListPets`.
Its fields go into `ListPetsServiceRequestOptions`, after the parameter groups and the bodies and
before `RawRequest`. So a field can have a type of its own in every operation:

```go
fields := make(map[string][]codegen.FieldSpec, len(api.Operations))
for _, op := range api.Operations {
	fields[op.ID] = []codegen.FieldSpec{{
		Name: "GenerateResponse",
		Type: codegen.TypeRef{Name: "func() (*" + op.ResponseData.Name + ", error)"},
		Doc:  "GenerateResponse makes the response.",
	}}
}
```

The request options and the response data of an operation are declared side by side, so this type
needs no import path, wherever the config places the service. `ResponseData` is empty in a config
without a `server` block: a plugin that builds a type from it gives its fields only when
`api.Service.Name` is set.

Every operation of the API takes fields, also a webhook and one the router drops. A key that is no
operation ID is an error. A field cannot take a name the options declare themselves: the parameter
groups (`PathParams`, `Query`, `QueryString`, `Headers`, `Cookies`), a name that starts with
`Body`, `RawRequest` or `Validate`. Two plugins can add fields to one operation, and they come in
the order `WithPlugins` gives the plugins, but one operation cannot get a field twice. These are
the request options of the server. In a config without a `server` block the fields are checked
all the same and then left out: the request options of the client do not get them.

The generated handlers do not set an added field. A part of the plugin does, see
[Setting a field](#setting-a-field).

The type of a field follows the rules of a [`TypeRef`](#the-api). With an import path it is an
identifier, or a pointer, slice, array, map or channel around one. Without one it is any Go type,
and the type alone: no space, comment or tag around it. A package needs its import path and has
to be a package name:

```go
TypeRef{Name: "func() any"}                                                    // written as it is
TypeRef{Name: "func( any"}                                                     // an error: no Go type
TypeRef{Name: "*Span", Package: "trace", ImportPath: "example.com/trace"}      // *trace.Span
TypeRef{Name: "Option[Pet]", Package: "opt", ImportPath: "example.com/opt"}    // an error
TypeRef{Name: "*Span", Package: "trace"}                                       // an error: no import path
TypeRef{Name: "*Span", Package: "open-trace", ImportPath: "example.com/trace"} // an error: no package name
```

Outside a module the output is one package, and a type of the API has no package and no import
path. A field takes it as it is, as in `Type: op.Success.Body`.

A type with an import path makes the file of the service import that package. When this closes
an import cycle, `Generate` fails with the cycle and names `server.service` for that import, as
it names a part under [Templates](#templates).

`Scaffolds` replace the template of a scaffold file the config writes (`ScaffoldService`,
`ScaffoldMiddleware`, `ScaffoldMain`). A replacement is used when the config names that file
under `server.scaffold`. For any other scaffold its template is never run, and a config without
a `server` block writes no scaffold at all. The replacement runs on the same data as the built-in
template, listed under [Scaffold data](#scaffold-data). One scaffold can be replaced by one
plugin.

### Templates

A plugin template is a `text/template`. It sees the plugin's own `Funcs` and the funcs of the
template of an extra file, listed under [funcs](templates.md#funcs): `expr`, `import` and `symbol`
write for the file the part lands in. `symbol` names a part of a plugin as
`plugin.<name>.<part>`. When the place of a part makes two output folders import each other,
`Generate` fails with the cycle and the part that closes it.

A func in `Funcs` replaces one of the same name, `expr`, `import` and `symbol` included, in the
templates of that plugin only. So a plugin can pass a whole library of funcs, and a func the
generator gains later never changes what a plugin's template calls. `Funcs` has to be a map
`text/template` takes: a name is an identifier, and a value is a func that returns one value, or
one value and an error. A template can run more than once for a file, so a func has to answer the
same each time.

The rules for the built-in templates apply: decide everything in Go and keep the template to
`range` and `if` over the data.

A key that a map does not have is an error, where `text/template` alone writes `<no value>` into
the code: `{{.owner}}` fails on a `UserContext` without `owner`. Ask for a key that may be
missing with `index` under `with` or `if`, as in
`{{with index . "owner"}}// Owned by {{.}}.{{end}}`. Printed on its own, `index` writes
`<no value>` for such a key.

### Setting a field

The generated handlers fill the request options from the request and call the service. They
cannot know what a field from `RequestOptionFields` holds, so they leave it empty. The plugin
sets it in a part: a service that wraps the user's one, sets the field and passes the call on.
`API.Service` is the interface both implement.

This part runs on the `API`. It sets the `GenerateResponse` field that
[Contribute](#contribute) adds to every operation, to a response without a body. `status` is a
func this plugin gives in `Funcs`: it returns the `Code` of the success response of an operation,
or 200 for an operation without one.

```
{{- $context := import "context"}}
// withResponses sets GenerateResponse on the options, then calls the service.
type withResponses struct {
	svc {{expr .Service}}
}

// WithResponses returns svc with GenerateResponse set for every operation.
func WithResponses(svc {{expr .Service}}) {{expr .Service}} {
	return &withResponses{svc: svc}
}
{{- range .Operations}}

func (s *withResponses) {{.ID}}(ctx {{$context}}.Context, opts *{{expr .RequestOptions}}) (*{{expr .ResponseData}}, error) {
	opts.GenerateResponse = func() (*{{expr .ResponseData}}, error) {
		return &{{expr .ResponseData}}{Status: {{status .ID}}}, nil
	}
	return s.svc.{{.ID}}(ctx, opts)
}
{{- end}}
```

The interface has a method for every operation, also for a webhook and for an operation the
router drops, so the wrapper needs all of them. The router then takes the wrapper in place of the
service:

```go
router := NewRouter(WithResponses(NewPets()))
```

`examples/plugin/basic/wrap/wrapper.go` is a fuller wrapper, as the sample plugin generates it in
a package of its own. It answers with the [constructor](#the-api) of the success response and an
empty body, so the body gets the content type of the spec. It also builds the router of the
wrapped service with [`symbol`](#templates).

## The API

`Contribute` gets the `API` that the template of an extra file runs on, described under
[the data](templates.md#the-data). Each plugin gets its own copy, down to the nested maps and
lists of `UserContext`. What a plugin changes in it reaches no other plugin, no template and not
the config.

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
| `Context`, `Errors` | the `context` and `errors` packages; `Context` is empty when `Operations` is |
| `Operations` | one entry per operation: `Name`, `Method`, `Path`, `Options`, `Data` |

Middleware (`ScaffoldMiddleware`): `HTTP`, `Slog`, `Time`, `Rand`, `Debug`, the packages.

Main (`ScaffoldMain`):

| Field | Holds |
|---|---|
| `Context`, `Slog`, `OS`, `Signal`, `Syscall` | the packages |
| `HTTP` | the `net/http` package; empty on fasthttp, fiber, goframe and hertz, which an `http.Server` does not serve |
| `Framework` | the package of the server on fiber (`fiber`), goframe (`ghttp`) and hertz (`server`); empty on the other routers |
| `Packages` | a map of package names, with the one key `fasthttp` on fasthttp; empty on the other routers |
| `NewRouter`, `NewService`, `WithMiddleware`, `WithRouter` | the functions a main calls |
| `Middleware` | the middleware expressions, empty without the middleware scaffold |
| `Port`, `Timeout` | the port and the timeout expression, such as `30 * time.Second` |

`HTTP`, `Framework` and `Packages` hold what the built-in main of each router writes. A
replacement that has to run on any router imports its packages itself, as in
`{{import "net/http"}}`. A function of the server that the view does not hold is written with
[`symbol`](#templates), as in `{{symbol "server.adapter" "WithErrorHandler"}}`.
