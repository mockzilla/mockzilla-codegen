# Templates

The config changes the generated code in two ways, both with Go templates:

- a [block](#blocks) replaces a small piece of a built-in template, such as the body of every
  method of the service scaffold
- an [extra file](#extra-files) is a file of your own, written from your template on every run

Nothing replaces a whole built-in template, so you never copy one to change a few lines. To write a
file your own way, leave its scaffold out and write it as an extra file.

## Examples

From simple to complex. Each one starts from a need and shows the config for it. The health route,
the 501 stubs and the mock server are examples in this repository, built and tested on every
change.

### A health route

A load balancer asks the server for `/health`, which the spec does not have. Add the route in
`server.router-extra`:

```yaml
templates:
  server.router-extra: |
    r.Get("/health", health)
```

Write the handler in a file of your own, in the package of the router:

```go
var health = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	_, _ = w.Write([]byte("ok"))
})
```

The route goes next to the generated ones, so the middleware wraps it too. This line is for chi;
[Routes](#routes) has it for every router. See `examples/templates/blocks`.

### Stubs that answer 501

Every method of the service scaffold returns `ErrNotImplemented`, and the server answers that with
500. To answer 501 Not Implemented until a method is written:

```yaml
imports:
  - package: net/http
templates:
  server.scaffold.service-method: |
    return &{{.Data}}{Status: http.StatusNotImplemented}, nil
```

Each method of the scaffold is then:

```go
func (s *Pets) ListPets(ctx context.Context, opts *ListPetsServiceRequestOptions) (*ListPetsResponseData, error) {
	return &ListPetsResponseData{Status: http.StatusNotImplemented}, nil
}
```

The generator cannot know which packages your text names, so `net/http` is listed under
[`imports`](#imports). See `examples/templates/blocks`.

### A mock of the service

To test the HTTP layer against a mock of the service, let [mockgen](https://github.com/uber-go/mock)
write one. A `go:generate` line before the interface does it:

```yaml
templates:
  server.service-header: |
    //go:generate go run go.uber.org/mock/mockgen@v0.6.0 -destination=mock_test.go -package=$GOPACKAGE . {{.Name}}
```

`{{.Name}}` is the name of the interface, `PetsInterface` here. `go generate ./...` then writes
`MockPetsInterface` into `mock_test.go`. The tests need `go.uber.org/mock` in the module. The same
line under `client.interface-header` mocks the interface of the client.

### A mock server

`examples/templates/wrapper` answers every operation with an empty success response, so a client
can be built before the service is written. It takes three pieces:

```yaml
server:
  framework: chi
  name: Pets
  scaffold: {service: ./service.go}
templates:
  server.request-options-extra: |
    // GenerateResponse makes the response, when the service is asked for one.
    GenerateResponse func() (*{{.Data}}, error)
  server.scaffold.service-method: |
    return opts.GenerateResponse()
extra-files:
  ./wrap.go: {file: ./wrapper.tmpl}
```

1. `server.request-options-extra` gives the request options of every operation a typed field.
2. `server.scaffold.service-method` makes every method of the scaffold return what that field
   makes.
3. The handlers leave the field unset. The extra file `wrap.go` writes `WithBodies`, a service that
   sets the field and then calls the service it wraps.

`wrapper.tmpl` ranges over the operations of [the data](#the-data). For the success response of
each operation, it calls the constructor:

- with `new` for a pointer body
- with a zero value for another body
- without a body when there is none

An operation without a success response answers 200:

```
{{- $context := import "context"}}
// withBodies sets GenerateResponse on the options of every operation, then calls the service.
type withBodies struct {
	svc {{expr .Service}}
}

// WithBodies returns svc with GenerateResponse set to answer with an empty success body.
func WithBodies(svc {{expr .Service}}) {{expr .Service}} {
	return withBodies{svc: svc}
}
{{- range $op := .Operations}}

func (s withBodies) {{.ID}}(ctx {{$context}}.Context, opts *{{expr .RequestOptions}}) (*{{expr .ResponseData}}, error) {
	opts.GenerateResponse = func() (*{{expr .ResponseData}}, error) {
{{- with .Success}}
{{- if .Body.Elem.Name}}
		return {{expr .Constructor}}({{if .HasStatusArg}}{{.Code}}, {{end}}new({{expr .Body.Elem}})), nil
{{- else if .Body.Name}}
		var body {{expr .Body}}
		return {{expr .Constructor}}({{if .HasStatusArg}}{{.Code}}, {{end}}body), nil
{{- else}}
		return {{expr .Constructor}}({{if .HasStatusArg}}{{.Code}}{{end}}), nil
{{- end}}
{{- else}}
		return &{{expr $op.ResponseData}}{Status: 200}, nil
{{- end}}
	}
	return s.svc.{{.ID}}(ctx, opts)
}
{{- end}}
```

The server is `NewRouter(WithBodies(NewPets()))`.

## Blocks

A block is a named piece of a built-in template. Until the config replaces it with text of its own,
the block writes its default.

The value of a block is the template text. To keep the text in a file, write `{file: <path>}`
instead, with the path relative to the config. The form of the value tells the two apart. A text is
never read as a path, whatever it looks like.

```yaml
templates:
  server.service-header: {file: ./templates/header.tmpl}
  server.router-extra: |
    r.Get("/health", {{.User.health}})
user-context:
  health: healthHandler
```

| Block | Where | Data | Default |
|---|---|---|---|
| `client.interface-header` | before the client interface | the interface | nothing |
| `server.service-header` | before the service interface | the service | nothing |
| `server.request-options-extra` | in every request options struct, before `RawRequest` | the operation | nothing |
| `server.response-data-extra` | in every response data struct, after `Body` | the operation | nothing |
| `server.router-extra` | in the router's registration, after the routes | the router | nothing |
| `server.scaffold.service-fields` | in the struct of the service scaffold | the scaffold | nothing |
| `server.scaffold.service-method` | the body of every method of the service scaffold | the method | `return nil, ErrNotImplemented` |

### Line breaks

The text of a block goes on lines of its own, without the blank lines around it. So it needs no
line break at its start or its end.

`server.service-header` and `client.interface-header` are followed by a blank line. That keeps them
out of the comment of the interface.

A block whose text comes out empty adds nothing.

### Names in a block

| Block | Field | Holds |
|---|---|---|
| `server.service-header` | `.Name` | the name of the service interface |
| `client.interface-header` | `.Name` | the name of the client interface |
| `server.request-options-extra`, `server.response-data-extra` | `.Options`, `.Data` | the names of the two structs of the operation |
| `server.scaffold.service-fields` | `.Name` | the name of the service struct |
| `server.scaffold.service-method` | `.Name` | the name of the method |
| `server.scaffold.service-method` | `.Options`, `.Data` | the method's two structs, written as the scaffold's file spells them |

### Funcs in a block

The text of a block is a `text/template` of its own. It can call the funcs every template of the
generator has, such as `comment` and `quote`, listed under [Funcs](#funcs).

It has no `expr`, `import` or `symbol`. The packages its code names are listed under
[`imports`](#imports).

### User context

`user-context` is available as `.User` in every block and as `.UserContext` in an extra file.

A key it does not have is an error: `{{.User.team}}` fails in a config that sets no `team`. To read
a key that may be missing, use `index` under `if` or `with`, as in `{{if index .User "team"}}`.

A block that writes `<no value>` anyway is an error too. It prints a key the config gives no value,
or an `index` on its own.

### Block errors

These are config errors:

- a block that does not exist, with the list of those that do
- a `server` block in a config without `server`, and a `client` block in one without `client`
- a scaffold block in a config that does not write that scaffold
- a value without text, and `{file: }` without a path
- text that can only be a path, such as `./header.tmpl` or `templates/header.txt`

Text that can only be a path would be written into the code as it is. So the error asks for
`{file: ./header.tmpl}`.

### Routes

In `server.router-extra`, the router and the way you add a route to it depend on the framework. Each
line below adds `GET /health`, with `health` an `http.HandlerFunc` of yours, as in
[A health route](#a-health-route).

| Framework | Router | The route |
|---|---|---|
| chi | `r`, a `chi.Router` | `r.Get("/health", health)` |
| std-http | `mux`, an `*http.ServeMux` | `mux.Handle("GET /health", route(health))` |
| echo, echo-v5 | `e`, an `*echo.Echo` | `e.GET("/health", echo.WrapHandler(health), m...)` |
| kratos | `r`, a `*khttp.Router` | `r.GET("/health", func(c khttp.Context) error { health(c.Response(), c.Request()); return nil })` |
| gin | `e`, a `*gin.Engine` | `e.GET("/health", handle(route(health)))` |
| fiber | `app`, a `*fiber.App` | `app.Get("/health", handle(route(health)))` |
| iris | `app`, an `*iris.Application` | `app.Get("/health", handle(route(health)))` |
| gorilla-mux | `r`, a `*mux.Router` | `r.Handle("/health", route(health)).Methods("GET")` |
| fasthttp | `r`, a `*router.Router` | `r.GET("/health", handle(route(health)))` |
| beego | `r`, a `*web.ControllerRegister` | `r.AddMethod("GET", "/health", handle(route(health)))` |
| go-zero | `r`, an `httpx.Router` | `handle(r, "GET", "/health", route(health))` |
| hertz | `h`, a `*server.Hertz` | `h.GET("/health", handle(route(health)))` |
| goframe | `s`, a `*ghttp.Server` | `s.BindHandler("GET:/health", handle(route(health)))` |

- Pass a route through `route`, or `m...` on echo, and the middleware of `WithMiddleware` wraps it
  like the generated ones. On chi and kratos the router `r` adds the middleware itself.
- `handle` makes the framework's handler from an `http.Handler`.
- On gin, `handle` also takes the names of the path values after the handler, in path order:
  `handle(route(h), "id")`. A value without a name keeps the name in gin's pattern.

## Imports

The text of a block is Go code the generator did not write, so it cannot know which packages that
code needs. List them under `imports`:

```yaml
imports:
  - package: github.com/google/uuid
  - {package: example.com/shop/tenant, alias: tn}
templates:
  server.request-options-extra: |
    TraceID uuid.UUID
    Tenant  tn.ID
```

The file that holds the request options now imports `github.com/google/uuid`, and
`example.com/shop/tenant` as `tn`. A generated file imports a listed package when its code names
it, and leaves it out otherwise. So one list serves every file.

A package that an [`x-go-type`](extensions.md#x-go-type) names is listed the same way.

### Package names

The name of a listed package is its `alias`. Without an alias, the name is the last element of the
path, without:

- a major version: `chi` for `github.com/go-chi/chi/v5`
- a `go-` prefix
- what follows a dot: `yaml` for `gopkg.in/yaml.v3`

A package named any other way needs the alias.

### Name clashes

In a file that names a listed package, the name is that package's. Another package the generator
imports under the same name gets a number in that file, such as `models2`.

A listed package that the generator imports too goes by the listed name in the generated code. So
an alias for such a package must not be a name that code gives a variable, like `ctx` or `r`.

### Side effects and unused entries

- With `alias: _` every generated file imports the package, for its side effects.
- An entry that no file names changes nothing. It is reported as an `import-unused` warning.

### Import errors

These are config errors:

- an entry without `package`
- an alias that is not a Go identifier
- `alias: .`: Go rejects a file that imports a package without using it, and with `.` the use
  cannot be checked
- a path that is listed twice under a name, or twice under `_`
- two entries with one name
- a path that gives no name, such as `example.com/9lives`, without an alias

## Extra files

`extra-files` maps a path to a template: its text, or `{file: <path>}` with the path relative to
the config, as for a block.

```yaml
extra-files:
  ./wrap.go: {file: ./wrapper.tmpl}
  ./routes.go: |
    // Routes lists the method and path of every route the router registers.
    var Routes = []string{
    {{- range .Operations}}{{if .IsRouted}}
    	{{quote (print .Method " " .Path)}},
    {{- end}}{{end}}
    }
```

An extra file is a generated file like the others. It gets the header, the package of its folder
and the imports its code names, and it is written on every run.

Its template runs on [the data](#the-data): the service, every operation with its types, responses
and constructors, and every model type. The template writes Go declarations, and the generator
formats them. For the pets API, `routes.go` is:

```go
// Routes lists the method and path of every route the router registers.
var Routes = []string{
	"GET /pets",
	"POST /pets",
	"DELETE /pets/{id}",
	"GET /ping",
}
```

[A mock server](#a-mock-server) shows a larger one, `wrapper.tmpl`.

### Funcs

The template of an extra file sees two sets of funcs. Three are bound to the file it writes: `expr`,
`import` and `symbol`. The others are the funcs every template of the generator has.

| Func | Does |
|---|---|
| `expr <TypeRef>` | writes the type as the file spells it, qualified and imported when it lives in another package |
| `import <path>` | imports the path and returns the name to qualify with |
| `symbol <part> <name>` | writes a name the part declares as the file spells it, qualified and imported when the part is in another package |
| `comment <text>` | writes the text as `//` lines, wrapped to 100 columns where a space allows, and nothing for blank text |
| `quote <text>` | writes the text as a Go string literal |
| `lower <text>` | lowers every letter |
| `ucFirst <text>` | raises the first character |

`toGoComment` is `comment` under another name. `escapeGoString` is `quote` without the quotes around
the text. The generator's `tag` is of no use here: it takes the tags of a model field, and an extra
file has no such value.

#### symbol

`symbol` reaches what the data does not list: the functions of the server, such as `NewRouter` and
`WithErrorHandler`, and what another extra file declares.

The part is named as in `output.files`, such as `server.router`. A scaffold is named
`server.scaffold.service`, and an extra file by its path as the config writes it, such as
`./wrap.go`. For a chi router:

```
{{- $http := import "net/http"}}
// Handler returns the router of svc.
func Handler(svc {{expr .Service}}) {{$http}}.Handler {
	return {{symbol "server.router" "NewRouter"}}(svc)
}
```

In the folder of the router this writes `NewRouter`. In another folder it writes `api.NewRouter`,
for a router in package `api`, and imports that package. So an extra file can sit in a package of
its own.

These are errors:

- a part the config does not write, such as `server.router` without a `server` block
- a name that is not an identifier
- a name that is not exported, when the part is in another folder

The generator does not check that the part declares the name. The Go compiler reports that.

### Import cycles

An extra file can use the types and functions of any package the generator writes. Its path decides
what it imports. When it makes two output folders import each other, `Generate` fails with the
cycle and the file that closes it, as it does for the built-in parts:

```
import cycle: api -> models -> api (server.service uses example.com/work/models, ./wrap.go uses example.com/work/api)
```

Move the extra file to a folder that the folders it uses do not import.

### Missing keys

`text/template` alone writes `<no value>` into the code for a key that a map does not have. In an
extra file that is an error: `{{.UserContext.owner}}` fails in a config that sets no `owner`.

To read a key that may be missing, use `index` under `with` or `if`, as in
`{{with index .UserContext "owner"}}// Owned by {{.}}.{{end}}`.

### Extra file errors

A template that does not parse or run, or that writes no Go declarations, is an `ErrExtraFile` with
the path.

These are config errors:

- a path that is not a `.go` file
- a path that `output.file`, `output.files` or a scaffold writes too, or two paths that are one
  file
- a value as for a block: without text, with text and a file, or text that can only be a path

## The data

The template of an extra file runs on an `API`: the generated code once names are resolved and
files are laid out. The struct only ever gains fields.

```go
type API struct {
	Package     string         // package of output.file, as package or output.packages names it
	Service     TypeRef        // the service interface, empty without a server block
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
	RequestOptions       TypeRef    // <Op>ServiceRequestOptions, empty without a server block
	ResponseData         TypeRef    // <Op>ResponseData, empty without a server block
	ClientRequestOptions TypeRef    // <Op>RequestOptions, empty without a client block or for a webhook
	ClientResponse       TypeRef    // <Op>Response, empty unless client.with-response is set
	Bodies               []Body     // every request body, in spec order
	Responses            []Response // every response, in the order of the generated code
	Success              *Response  // the first of them with a Code from 200 to 299, or nil
}

type Body struct {
	ContentType string  // application/json
	Field       string  // the field of the request options that holds it: Body, or BodyJSON next to others
	Type        TypeRef // the type of that field, such as *Pet
}

type Response struct {
	Status       string  // the key as the spec writes it: 200, 2XX, default
	Code         int     // the code read from the key: 200 for 2XX, 0 for default
	ContentType  string  // of the JSON body, else the first one read whole, else the first stream
	Body         TypeRef // the type the constructor takes, one frame with IsStream; empty without a body
	IsRaw        bool    // the body has no schema: any, string or []byte
	IsStream     bool    // the body is sequential: the constructor takes an iter.Seq of Body
	Constructor  TypeRef // makes the response data of this status; empty without a server block
	HasStatusArg bool    // the constructor takes the status first
}
```

### Responses

`Responses` come in the order of the generated code: codes, ranges, other keys, then `default`. The
order in the spec does not matter.

`Code` is what the generated client and error mapping read from the key:

| Key | `Code` |
|---|---|
| a number | the number |
| a range such as `2XX` | the first digit times 100 |
| anything else (`default`) | 0 |

A key that is not a code from 100 to 599, not a range and not `default` is left out. Generation
warns (`invalid-status`).

`Success` is the first response with a `Code` from 200 to 299. It points into `Responses`.

### Bodies

`Bodies` lists the request bodies the server and the client options hold, each with the field it
is in, so a template can make one. For a JSON body of type `*Pet`:

```
{{- range .Bodies}}{{if eq .ContentType "application/json"}}
	body := new({{expr .Type.Elem}})
{{- end}}{{end}}
```

### Constructors

`Constructor` is the function a hand-written service calls to answer with one status, such as
`NewGetPetResponseData404`. It takes these arguments, in order:

1. The status, when `HasStatusArg` is set. That is when the key is not a number, such as a range or
   `default`.
2. The body, when the response has one. With `IsStream` set, the body is an `iter.Seq` of `Body`:
   the frames of a response that is only a stream.

A response with a body read whole and a stream describes the first. Its stream has a constructor of
the same name with the suffix `Stream`.

A constructor also sets the content type the spec gives. That field is unexported, so a literal of
`ResponseData` in another package cannot set it.

Write the constructor with `expr`, like a type. For a success response with a body, and `body` of
the type it takes:

```
{{- with .Success}}
	return {{expr .Constructor}}({{if .HasStatusArg}}{{.Code}}, {{end}}body), nil
{{- end}}
```

`examples/templates/wrapper/wrapper.tmpl` calls it for every operation: with a body, with `new` for
a pointer body, and without one.

### TypeRef

`TypeRef` is a Go type, or a function such as `Constructor`. It has three parts:

| Part | Holds |
|---|---|
| `Name` | the type as the package that declares it writes it |
| `Package` | the name of the package of the identifier in it |
| `ImportPath` | the import path of the identifier in it |

#### With an import path

`Name` is an identifier, or a pointer, slice, array, map or channel around one (`Pet`, `[]Pet`,
`*Pet`, `map[string]Pet`). The package goes before that identifier. A map key or an array length is
written as it is.

A generic type, a func type or a name that is qualified already cannot carry an import path.

`Package`, when set, is the name of the package: an identifier other than `_`, and only with an
import path.

#### Without an import path

The type needs no import. `Name` is written as it is (`string`, `func() any`) and `Package` is
empty.

A type the generator declares has neither a package nor an import path when the output is one
package outside a module.

#### Writing a type with expr

`expr` writes a type as the file being written spells it and adds the import.

It fails on a type whose `Name` cannot carry its import path, and on a `Package` without an import
path. It fails in every file, so a template does not start to fail when the output is split into
packages.

To write a type around one of another package, put it together in the template:
`Page[{{expr .Body}}]`.

#### Elem

`Elem()` is the type a pointer points to, in the same package: `Pet` for `*Pet`. It is empty for
any other type.

A constructor that takes `*Pet` gets a body that is not nil from `new({{expr .Body.Elem}})`, or
from `var body {{expr .Body.Elem}}` passed as `&body`.
