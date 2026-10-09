<h1 align="center">
  <img src="docs/images/gopher.svg" alt="A gopher with its paws in its pocket" width="140" /><br>
  mockzilla-codegen
</h1>

<div align="center">

[![CI](https://github.com/mockzilla/mockzilla-codegen/actions/workflows/ci.yaml/badge.svg?branch=main)](https://github.com/mockzilla/mockzilla-codegen/actions/workflows/ci.yaml?query=branch%3Amain)
[![codecov](https://codecov.io/gh/mockzilla/mockzilla-codegen/graph/badge.svg)](https://codecov.io/gh/mockzilla/mockzilla-codegen)
[![Go Reference](https://pkg.go.dev/badge/github.com/mockzilla/mockzilla-codegen.svg)](https://pkg.go.dev/github.com/mockzilla/mockzilla-codegen)
[![License](https://img.shields.io/github/license/mockzilla/mockzilla-codegen?cacheSeconds=3600)](LICENSE)

</div>

Go models, HTTP servers, clients and MCP tools from OpenAPI 3.0, 3.1 and 3.2 specs.

## Why this one

Every generator works on the petstore. This one works on yours. The specs of Stripe, GitHub, OpenAI
and Adyen generate, build and run, and so do 2,200+ other real-world specs.

### Specs

- OpenAPI 3.1 and 3.2, next to 3.0.
- `oneOf` and `anyOf`, with or without a discriminator, nested in each other. In a body, a
  parameter, a header or a form field.
- `allOf` merged into one struct, its limits combined to the strictest.
- Every inline object gets a named type, however deep. No anonymous structs.
- `$ref` into other files, and overlays.
- A part that cannot be generated gets a warning and is left out. The rest is written.

### Parameters and bodies

- Parameters in every style OpenAPI defines, `deepObject` and the 3.2 `cookie` style included.
- Defaults filled in for parameters the request leaves out.
- JSON, form and multipart bodies, nested objects included. Binary bodies stream.

### Idiomatic Go

- Fields in the order of the spec. Name clashes resolved for you.
- A pointer only where a value can be missing, never on a slice or map.
- `runtime.Nullable[T]` tells `null` from absent, so a PATCH can clear a value or leave it as it
  is. Turn it on for the whole spec or for one field.
- Validation in plain Go: required fields, patterns, limits, `multipleOf`, `uniqueItems`. The
  server can check every request and response with it.
- An error response the spec documents is a Go error, on the server and on the client.
- One file, many files, or a package per part. Any block of the templates can be replaced.

### No heavy dependencies

- Generated code imports the standard library, your router, and a runtime package that uses only
  the standard library.
- No YAML parser, logger or third-party JSON library ends up in your binary. Plug one in if you
  want it.

### Server and client

- 14 routers, `net/http`'s `ServeMux` among them. Your service stays the same on each.
- Hooks for OpenTelemetry on server and client: middleware around each operation, and the HTTP
  client you pass in. Both see the operation name ([observability](docs/observability.md)).
- MCP tools over the client, so an AI assistant can call your API.

Coming from another generator? The [migration guides](docs/migration.md) map every flag, config key
and extension.

## Quick start

Go 1.26 or newer.

```sh
go get -tool github.com/mockzilla/mockzilla-codegen/cmd/mockzilla-codegen
go tool mockzilla-codegen generate openapi.yaml               # models in ./gen.go
go tool mockzilla-codegen generate openapi.yaml -server chi   # and a chi server
go tool mockzilla-codegen generate -c codegen.yaml            # what the config lists
```

## Example

This spec:

```yaml
paths:
  /pets/{id}:
    get:
      operationId: getPet
      parameters:
        - {name: id, in: path, required: true, schema: {type: integer}}
      responses:
        "200":
          description: ok
          content: {application/json: {schema: {$ref: "#/components/schemas/Pet"}}}
components:
  schemas:
    Pet:
      type: object
      required: [id, name]
      properties:
        id: {type: integer}
        name: {type: string, maxLength: 64}
        tag: {type: string}
```

gives this, among other code:

```go
type Pet struct {
	ID   int     `json:"id"`
	Name string  `json:"name"`
	Tag  *string `json:"tag,omitempty"`
}

func (p Pet) Validate() error {
	var errs validation.Errors
	errs.Append("name", validation.MaxLength(p.Name, 64))
	return errs.Err()
}

type ServiceInterface interface {
	// GetPet handles GET /pets/{id}.
	GetPet(ctx context.Context, opts *GetPetServiceRequestOptions) (*GetPetResponseData, error)
}
```

You implement the service. The generated router decodes the request and writes the response:

```go
type pets struct{}

func (pets) GetPet(ctx context.Context, opts *api.GetPetServiceRequestOptions) (*api.GetPetResponseData, error) {
	return api.NewGetPetResponseData(&api.Pet{ID: opts.PathParams.ID, Name: "Rex"}), nil
}

http.ListenAndServe(":8080", api.NewRouter(pets{}))
```

## Docs

| Guide | What it covers |
|---|---|
| [Getting started](docs/getting-started.md) | install, commands, checking generated files in CI, the runtime guard |
| [Configuration](docs/config.md) | every key, output files, defaults |
| [Types](docs/types.md) | how schemas become Go types: unions, `allOf`, nullable values |
| [Naming](docs/naming.md) | how names are built and how clashes are resolved |
| [Validation](docs/validation.md) | the generated checks for requests and responses |
| [Error types](docs/errors.md) | response types that are Go errors |
| [Extensions](docs/extensions.md) | the `x-go-*` extensions and masking of sensitive values |
| [Server](docs/server.md) | the service interface, the HTTP adapter, 14 routers, starter files |
| [Client](docs/client.md) | one method per operation, envelopes, streams |
| [MCP](docs/mcp.md) | MCP tools over the client |
| [Observability](docs/observability.md) | tracing and metrics with OpenTelemetry, on server and client |
| [Templates](docs/templates.md) | replacing template blocks, extra files from your own templates |
| [Migration](docs/migration.md) | moving from another generator |

## Contributing

Issues and pull requests are welcome. [CONTRIBUTING.md](CONTRIBUTING.md) lists the make targets, the
checks a pull request has to pass, and how to add a router.

## License

MIT, see [LICENSE](LICENSE). The Go gopher was designed by Renée French and is licensed under
[CC BY 4.0](https://creativecommons.org/licenses/by/4.0/). The gopher above is a new drawing of it.
