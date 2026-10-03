# MCP

An `mcp` block in the config generates [Model Context Protocol](https://modelcontextprotocol.io/)
tools over the generated [client](client.md): one tool per operation, each with an input schema
built from the spec, and a type that registers them on a server of the official Go SDK,
`github.com/modelcontextprotocol/go-sdk`. An AI assistant connected to that server calls the API
through the tools.

```yaml
client:
mcp:
  default-skip: false  # true leaves every operation out unless x-mcp turns it on
```

`mcp` needs `client`, since the tools call it. Generated code imports the SDK, so add it to your
module:

```sh
go get github.com/modelcontextprotocol/go-sdk
```

## Tools

```go
type MCPTools struct{ ... }

func NewMCPTools(c PetClientInterface) *MCPTools
func (t *MCPTools) Register(s *mcp.Server)

func (t *MCPTools) ListPetsTool() *mcp.Tool
func (t *MCPTools) ListPets(ctx context.Context, req *mcp.CallToolRequest, in ListPetsToolInput) (*mcp.CallToolResult, any, error)
```

- `NewMCPTools` takes the client interface, so the tools call the generated client or a test
  double. `Register` adds every tool to a server. To expose a few, add each yourself:
  `mcp.AddTool(s, t.ListPetsTool(), t.ListPets)`.
- `<Op>Tool` is the definition of a tool: its name, its description, the schema of its input, and
  hints: a tool of a `GET`, `HEAD`, `OPTIONS`, `TRACE` or `QUERY` operation is marked read-only
  and idempotent, one of a `PUT` or `DELETE` idempotent.
- `<Op>` is the handler. The SDK validates the arguments against the schema, decodes them into the
  input type and calls it; the handler builds the request options of the client from the input
  and calls the client method.
- The tool name is the operation ID in snake case: `listPets` and `list-pets` give `list_pets`.
  Two operations whose names collide are numbered, `list_pets2`, with a `name-clash` note. The
  description is the operation's summary and description.
- Webhooks get no tool, since they come in.

A server that serves the tools over stdio, for a desktop assistant:

```go
func main() {
	c, err := api.NewPetClient(os.Getenv("PETS_URL"), api.WithRequestEditor(func(_ context.Context, r *http.Request) error {
		r.Header.Set("Authorization", "Bearer "+os.Getenv("PETS_TOKEN"))
		return nil
	}))
	if err != nil {
		log.Fatal(err)
	}

	s := mcp.NewServer(&mcp.Implementation{Name: "pets", Version: "1.0.0"}, nil)
	api.NewMCPTools(c).Register(s)
	if err := s.Run(context.Background(), &mcp.StdioTransport{}); err != nil {
		log.Fatal(err)
	}
}
```

## Input

```go
type ListPetsToolInput struct {
	// How many pets to return at most.
	Limit   *int   `json:"limit,omitempty"`
	XTenant string `json:"X-Tenant"`
}

type CreatePetToolInput struct {
	// The pet to add.
	Body *Pet `json:"body"`
}
```

The input of a tool is one JSON object, described by a JSON Schema 2020-12 document the SDK
validates every call against:

- One property per path, query, header and cookie parameter, named as the spec names the parameter
  and typed by its schema, with the parameter's description. Required parameters and path
  parameters are required properties. A parameter with `content` instead of a schema takes the
  schema of its media type.
- `body` for the request body, with the schema of its JSON media type, else of its first one. A body
  without a schema is anything for JSON, a string for `text/*`, and a base64 string otherwise. A
  required body is a required property. A body that is a file stream is left out, since JSON
  cannot carry it.
- Two parameters of one name in two locations, or a parameter named `body`, are told apart by the
  location: `query_id`, `request_body`. The Go fields follow: `QueryID`, `RequestBody`.
- Schemas of components go to `$defs` once and are referred to, so a schema that refers to itself
  ends. Keywords come over as they are: types with `null` for nullable schemas, formats, bounds,
  lengths, patterns, enums, defaults, `readOnly` and `deprecated`. Discriminators and `x-*`
  extensions are left out.
- A default that does not fit its own schema, such as `default: "20"` on an integer, is left out
  with a `default-ignored` warning. The SDK checks every default when a tool is added and panics
  on one that does not fit.
- No other property is allowed, so a misspelled parameter is an error the assistant sees, not a
  parameter silently dropped.

## Results

- A response body comes back as structured content, the JSON of what the client method returns,
  and as text content holding the same JSON, which every client reads.
- A `text/*` body comes back as text content alone.
- An operation without a response body answers with the text `ok`.
- An error of the client is the error of the tool: `IsError` is set and the text is the error's
  message. A response outside 2xx reads `unexpected status 404 Not Found`, followed by the message
  of the error type when the spec documents one (see `models.error-mapping`), so the assistant
  can act on it.
- An operation whose 2xx responses come only as `text/event-stream` or line-delimited JSON answers
  with the error `ErrMCPStreaming`, since a tool result cannot carry a stream. Read such an
  operation through the client's `<Op>Stream` method instead. An operation that documents JSON
  next to a stream is read as JSON.

## x-mcp

The `x-mcp` extension on an operation picks and describes its tool:

```yaml
paths:
  /pets:
    get:
      operationId: listPets
      x-mcp:
        name: list_all_pets
        description: Every pet, the newest first. Pass limit to keep the answer short.
  /internal/reset:
    post:
      operationId: reset
      x-mcp:
        skip: true
```

| Property | Effect |
|---|---|
| `skip` | `true` leaves the operation out, `false` keeps it, also under `default-skip: true` |
| `name` | the tool name: letters, digits, `_`, `-` and `.`, up to 128 characters; anything else is left out with an `mcp-tool-name` warning |
| `description` | the tool description, instead of the summary and description |

| `default-skip` | `x-mcp.skip` | Tool |
|---|---|---|
| `false` | not set, `false` | yes |
| `false` | `true` | no |
| `true` | not set, `true` | no |
| `true` | `false` | yes |

## Testing

The SDK connects a client to a server in process, so a test calls the tools against an
`httptest.Server` of the generated router, or a test double of the client:

```go
srv := httptest.NewServer(NewRouter(&service{}))
c, _ := NewPetClient(srv.URL)
server := mcp.NewServer(&mcp.Implementation{Name: "pets", Version: "1"}, nil)
NewMCPTools(c).Register(server)

serverTransport, clientTransport := mcp.NewInMemoryTransports()
serverSession, _ := server.Connect(ctx, serverTransport, nil)
session, _ := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "1"}, nil).Connect(ctx, clientTransport, nil)
res, _ := session.CallTool(ctx, &mcp.CallToolParams{Name: "list_pets", Arguments: map[string]any{"limit": 1}})
```

The [examples](../examples/mcp) do this for the tools of a pet store, for a spec that picks its
tools with `x-mcp`, for one that streams, and for one with a default that does not fit.

## Layout

The MCP code has two parts for `output.files`: `mcp.tools` (the tools type, the definitions and
the handlers) and `mcp.inputs` (the input types). The tools refer to the client and the inputs; the
inputs refer to the models. Any of them may go in another folder.
