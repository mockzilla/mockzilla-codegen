# Extensions

Extensions change what mockzilla-codegen writes for one schema, property or parameter.

| Extension | Where | Effect |
|---|---|---|
| `x-go-type` | schema | the Go type to use instead of the one mockzilla-codegen picks |
| `x-go-type-import` | next to `x-go-type` | the package of that type: a path, or `{path, name}` to import it under a name |
| `x-go-type-name` | schema | the name of the type the schema declares |
| `x-go-name` | schema, property, parameter or its schema | the name of the type, field or parameter field |
| `x-go-name-exact` | next to `x-go-name` | use the name as written, even unexported |
| `x-go-type-skip-optional-pointer` | property, parameter or its schema | no pointer for an optional field, whose zero value then counts as absent ([pointers](types.md#pointers)) |
| `x-go-nullable` | property, parameter or its schema | `runtime.Nullable[T]` (`true`) or a pointer (`false`), over `models.nullable` ([nullable](types.md#nullable)) |
| `x-go-json-ignore` | property | JSON tag `-` |
| `x-omitempty` | property | `omitempty` on (`true`) or off (`false`) |
| `x-go-extra-tags` | property, parameter or its schema | extra struct tags; they win over `models.extra-tags` on the same key |
| `x-enum-names` | enum schema | constant names, in value order, used as written |
| `x-deprecated-reason` | schema, property | the text of `// Deprecated:` when `deprecated: true` is set |
| `x-sensitive-data` | property | masked in `Masked()` and in logs |
| `x-mcp` | operation | MCP tool settings: `skip`, `name`, `description` ([MCP](mcp.md#x-mcp)) |

## Parameters

A parameter's field reads `x-go-name`, `x-go-extra-tags`, `x-go-type-skip-optional-pointer` and
`x-go-nullable` from the parameter and from its schema. When both set a name, or a tag of the same
key, the parameter's wins, with a warning.

## What is read and what is ignored

- A value of the wrong kind is left out, with a warning.
- Booleans may be written as strings, `"true"`, as older specs do.
- An unknown extension that starts with `x-go-` is left out with a warning, since it is likely a
  typo.
- Other `x-*` extensions are ignored.

## x-go-type

```yaml
Host:
  type: object
  properties:
    address:
      type: string
      x-go-type: netip.Addr
      x-go-type-import: {path: net/netip}
    timeout:
      type: integer
      x-go-type: time.Duration
Port:
  type: integer
  x-go-type: uint16
```

```go
type Host struct {
	Address *netip.Addr    `json:"address,omitempty"`
	Timeout *time.Duration `json:"timeout,omitempty"`
}

type Port = uint16
```

### Where the package comes from

A value with one dot, such as `netip.Addr`, is a type of a package. The path of the package comes
from the first of these that applies:

1. `x-go-type-import`
2. the entry for that package name in the config's [`imports`](templates.md#imports)
3. the part before the dot, taken as the path. This works for standard library packages such as
   `time`.

### Other values

Anything else is written as is: `int64`, `[]string`, `map[string]string`.

- A slice, map or pointer type written this way gets no extra pointer.
- A package it names, as in `[]uuid.UUID`, is imported when the config's `imports` list it.

### Aliases

- A component with `x-go-type` becomes an alias of that type. Its properties are not generated.
- An `allOf` with a member that has `x-go-type` becomes an alias too. What the other members add is
  not generated, with a warning. A member that only adds docs or limits gives no warning.

The type is not validated, and a union takes any JSON for it.

## Names

Two extensions set a name:

| Extension | Names |
|---|---|
| `x-go-name` | a field, a parameter field, or a type declared under `components` or for a body |
| `x-go-type-name` | the type any schema declares, inline ones included. Never a field. |

- Both take part in clash resolution with the highest rank ([naming](naming.md#clashes)).
- A name that still has to change gets a warning.
- The name is used as written, with its first letter upper case: `ThreeDSACSURL` stays
  `ThreeDSACSURL`. `x-go-name-exact: true` keeps the first letter too, so the name can be
  unexported. A name that is still not exported, such as `_note`, goes through the
  [naming rules](naming.md).

## x-sensitive-data

Set `x-sensitive-data` on a property to mask its value in `Masked()` and in logs. The value is
`true`, the name of a mask, or an object with `mask` and its settings.

```yaml
password: {type: string, x-sensitive-data: true}
ssn: {type: string, x-sensitive-data: {mask: regex, pattern: '\d'}}
card: {type: string, x-sensitive-data: {mask: partial, keepPrefix: 0, keepSuffix: 4}}
apiKey: {type: string, x-sensitive-data: hash}
```

| Mask | Result |
|---|---|
| `full` (or `true`) | `********`, whatever the length |
| `regex` | each character the pattern matches becomes `*`: `***-**-****` |
| `hash` | the first 16 hex digits of the SHA-256: equal values stay equal |
| `partial` | `keepPrefix` and `keepSuffix` characters stay: `********3456` |

### Methods

A type with a sensitive value gets two methods:

- `Masked()` returns a copy with the sensitive values masked. A sensitive value that is not a
  string is cleared. Nested types, slices and maps of them are masked too.
- `LogValue()` makes `log/slog` log the masked copy.

JSON encoding stays raw. `json.Marshal(user)` sends the real values, `json.Marshal(user.Masked())`
the masked ones.

### Regex patterns

A regex pattern is ECMA-262 and read as for [validation](validation.md#patterns). One Go cannot
compile falls back to the full mask, with a warning.
