# Extensions

Extensions change what mockzilla-codegen writes for one schema, property or parameter.

| Extension | Where | Effect |
|---|---|---|
| `x-go-type` | schema | the Go type to use instead of the one mockzilla-codegen picks |
| `x-go-type-import` | next to `x-go-type` | the package of that type: a path, or `{path, name}` to import it under a name |
| `x-go-type-name` | schema | the name of the type the schema declares |
| `x-go-name` | schema, property, parameter or its schema | the name of the type, field or parameter field |
| `x-go-name-exact` | next to `x-go-name` | use the name as written, even unexported |
| `x-go-type-skip-optional-pointer` | property, parameter or its schema | no pointer for an optional field |
| `x-go-json-ignore` | property | JSON tag `-` |
| `x-omitempty` | property | `omitempty` on (`true`) or off (`false`) |
| `x-go-extra-tags` | property, parameter or its schema | extra struct tags; they win over `models.extra-tags` on the same key |
| `x-enum-names` | enum schema | constant names, in value order, used as written |
| `x-deprecated-reason` | schema, property | the text of `// Deprecated:` when `deprecated: true` is set |
| `x-sensitive-data` | property | masked in `Masked()` and in logs |
| `x-mcp` | operation | MCP tool settings: `skip`, `name`, `description` ([MCP](mcp.md#x-mcp)) |

A parameter's field reads `x-go-name`, `x-go-extra-tags` and `x-go-type-skip-optional-pointer` from
the parameter and from its schema. When both set a name, or a tag of one key, the parameter's
wins, with a warning.

A value of the wrong kind is left out, with a warning. An unknown extension starting with `x-go-`
is left out with a warning too, since it is likely a typo. Other `x-*` extensions are ignored. Booleans may be written as strings, `"true"`, as older specs do.

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

- A name with one dot is a type of a package. `x-go-type-import` gives its path, else the entry
  of that name in the config's [`imports`](templates.md#imports) does, else the part before the dot
  is taken as the path, which works for standard library packages such as `time`.
- Anything else is written as is: `int64`, `[]string`, `map[string]string`. A slice, map or
  pointer type written this way gets no extra pointer. A package it names, as in `[]uuid.UUID`,
  is imported when the config's `imports` list it.
- A component with `x-go-type` becomes an alias of that type; its properties are not generated.
- So does an `allOf` with a member that has `x-go-type`. What the other members add is not
  generated, with a warning; members that only add docs or limits get none.
- The type is not validated, and a union takes any JSON for it.

## Names

`x-go-name` names a field, a parameter field, or a type declared under `components` or for a body.
`x-go-type-name` names the type any schema declares, inline ones included, and never a field. Both
take part in clash resolution with the highest rank; a name that still has to change gets a
warning. The name is exported unless `x-go-name-exact: true` is set.

## x-sensitive-data

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

A type with a sensitive value gets two methods:

- `Masked()` returns a copy with the sensitive values masked. A sensitive value that is no string is
  cleared. Nested types, slices and maps of them are masked too.
- `LogValue()` makes `log/slog` log the masked copy.

JSON encoding stays raw: `json.Marshal(user)` sends the real values, `json.Marshal(user.Masked())`
the masked ones. A regex pattern is ECMA-262 and read as for [validation](validation.md#patterns).
One Go cannot compile falls back to the full mask, with a warning.
