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
- `NewMCPTools` panics on a nil client. The SDK does not recover a panic in a tool, so a nil
  client would end the server at the first call.
- `<Op>Tool` is the definition of a tool: its name, its description, the schema of its input, and
  hints: a tool of a `GET`, `HEAD`, `OPTIONS`, `TRACE` or `QUERY` operation is marked read-only
  and idempotent, one of a `PUT` or `DELETE` idempotent.
- `<Op>` is the handler. The SDK validates the arguments against the schema, decodes them into the
  input type and calls it; the handler builds the request options of the client from the input
  and calls the client method.
- The SDK reads the numbers of the arguments as float64, which rounds an integer above 2^53. So
  the handler of an input that holds a wider integer than `int32`, raw JSON or a type of another
  package first decodes the arguments again with `mcptool.Input`. An integer sent as plain
  digits then keeps all of them, and a default the SDK filled in stays. A host written in
  JavaScript may round such an integer before the server gets it.
- The tool name is the operation ID in snake case: `listPets` and `list-pets` give `list_pets`.
  Two operations whose names collide are numbered, `list_pets2`, with a `name-clash` note. Some
  hosts take only letters, digits, `_` and `-` up to 64 characters. A longer name, or one with a
  dot, is kept with an `mcp-tool-name` warning; `x-mcp.name` sets another.
- The description is the operation's summary and description. An operation with neither is
  described by its method and path, `GET /pets`.
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
  schema of its media type. A `querystring` parameter is one property too, with the schema of its
  media type.
- A header parameter named `Accept`, `Content-Type` or `Authorization` is not in the input, since
  OpenAPI ignores it. The client sets the first two. A token goes on the client with
  `WithRequestEditor`, as in the server above, so the assistant never sees it.
- `body` for the request body, with the schema of its JSON media type, else of its first one. A body
  without a schema is anything for JSON, a string for `text/*`, and a base64 string otherwise. A
  required body is a required property. A body the client sends as bytes, such as an `image/png`
  file, names its media type in `contentMediaType` unless its schema names one.
- Bytes come as base64: a schema of `format: binary` or `format: byte`, a whole body or a file
  field of a form, gets `contentEncoding: base64` unless it names an encoding. A file decoded from
  base64 has no name; in a form it goes out as `blob`.
- Two parameters of one name in two locations, or a parameter named `body`, are told apart by the
  location: `query_id`, `request_body`. The Go fields follow: `QueryID`, `RequestBody`.
- Schemas of components go to `$defs` once and are referred to, so a schema that refers to itself
  ends. Keywords come over as they are: types, formats, bounds, lengths, enums, defaults and
  `deprecated`. The `discriminator` keyword and `x-*` extensions are left out.
- A `oneOf` with a discriminator tells its variants apart by the discriminator value, as the Go
  union does. Each variant requires the property and takes the values that pick it: those the
  mapping lists for it, else its component name. A variant that holds the property to one value
  itself keeps that value. Without this, two variants of the same shape both match, and `oneOf`
  turns the body down. A variant no value picks alone, such as an inline one without a value or
  the `defaultMapping` one, matches next to the others, so the list becomes `anyOf`.
- A `readOnly` property is not in the input and not required, since a request does not carry it.
  `Validate` leaves it out the same way. A property `readOnly` in one `allOf` member is left out of
  every member, as the Go type merges them.
- A nullable schema takes null. `null` joins its types, and a schema without types takes null
  already. When its `$ref`, a composition, an enum without `null` or a const would turn null away,
  as in the 3.0 form `{nullable: true, allOf: [{$ref: Pet}]}`, the schema becomes `anyOf` of itself
  and `{"type": "null"}`, with its description and default outside.
- A default that does not fit its own schema, such as `default: "20"` on an integer, is left out
  with a `default-ignored` warning. The SDK checks every default when a tool is added and panics
  on one that does not fit.
- A pattern comes over when Go's regexp, which the SDK checks it with, compiles it. It is
  written as for [validation](validation.md#patterns), with `\xHH` up to U+00FF and the
  character itself above, so Go and an ECMA-262 engine read the pattern the same. A pattern Go
  cannot compile, such as one with a lookahead or a repeat count above 1000, is left out with a
  `pattern-unsupported` warning, since the SDK panics on it too. A default that does not match
  its pattern is left out as above.
- No other property is allowed, so a misspelled parameter is an error the assistant sees, not a
  parameter silently dropped.

## Results

- A response body comes back as structured content, the JSON of what the client method returns,
  and as text content holding the same JSON, which every client reads.
- Structured content is always a JSON object, since clients before protocol 2026-07-28 take
  nothing else there. An object comes as it is. Anything else, such as a list or a number, comes
  as `{"result": <value>}`. The generated handler returns `mcptool.Result{Value: out}`, which
  writes it that way.
- A `text/*` body comes back as text content alone.
- A file comes back as image content when the response names an `image/*` media type, as audio
  content for `audio/*`, so the assistant sees or hears it. Any other file comes as base64 under
  `result`. Bytes without a schema take the media type the spec documents, unless it is a wildcard.
- An operation without a response body answers with the text `ok`. So does another 2xx the spec
  lists next to the one with the body, such as a 204, since the client does not read its body.
- An error of the client is the error of the tool: `IsError` is set and the text is the error's
  message. A response outside 2xx, or a 2xx the spec does not list, reads
  `unexpected status 404 Not Found`, followed by the message of the error type when the spec
  documents one (see `models.error-mapping`). The response body follows on the next line, so the
  assistant can act on it. The body is cut after 4 KiB, and one that is no UTF-8 text shows only
  its size. The generated handler returns `mcptool.Error(err)`, which writes it that way.
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
| `name` | the tool name: letters, digits, `_`, `-` and `.`, up to 128 characters; anything else is left out with an `mcp-tool-name` warning, and a name over 64 characters or with a dot is kept with one |
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
tools with `x-mcp`, for one that streams, for one with a default that does not fit, for
patterns, for nullable schemas of 3.0, and for results that are no object or a file. The pet
store example also sends an id above 2^53.

## Layout

The MCP code has two parts for `output.files`: `mcp.tools` (the tools type, the definitions and
the handlers) and `mcp.inputs` (the input types). The tools refer to the client and the inputs; the
inputs refer to the models. Any of them may go in another folder.
