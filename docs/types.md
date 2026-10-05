# Types

How schemas become Go types. Names follow the [naming rules](naming.md).

## Which schemas get a named type

| Schema | Declaration |
|---|---|
| Every schema under `components/schemas` | always, named after the component |
| Inline object with properties | a struct, named after where it sits (`OrderClientAddress`) |
| Inline enum | a named type with constants |
| Inline `oneOf`, `anyOf`, or a 3.1 type list such as `[string, integer]` | a union type |
| Request body or response schema written inline | always: `CreatePetRequestBody`, `GetPetResponse200` |
| Request body or response schema that is only a `$ref` | none, the referenced type is used |
| Parameters of an operation | one struct per location: `GetPetPathParams`, `GetPetQuery`, `GetPetHeaders`, `GetPetCookies` |

Other inline schemas are written in place: `[]string`, `map[string]int64`, `*time.Time`.

A named type is a struct for an object, a type over its base for an enum, `type Pets []Pet` for an
array, `type Labels map[string]string` for a map, and an alias such as `type PetID = string` for
everything else. A component that is only a `$ref` is an alias of the referenced type.

## Type mapping

| Schema | Go type |
|---|---|
| `integer` | `int`, or `models.int-type` |
| `integer` with format `int8`, `int16`, `int32`, `int64`, `uint`, `uint8`, `uint16`, `uint32`, `uint64` | the matching Go type |
| `number` | `float64` |
| `number` with format `double` | `float64` |
| `number` with format `float` | `float32` |
| `number` with format `int32` or `int64` | `int32` or `int64` |
| `number` with format `integer` or `int` | same as `integer` |
| `boolean` | `bool` |
| `string` | `string` |
| `string` with format `date` | `runtime.Date` |
| `string` with format `date-time` | `time.Time` |
| `string` with format `email` | `runtime.Email` |
| `string` with format `uuid` | `string` |
| `string` with format `byte` | `[]byte` |
| `string` with format `binary` | `runtime.File` |
| `string` with format `json` | `json.RawMessage` |
| `object` with properties | struct |
| `object` without properties | `map[string]any` |
| `array` | `[]T`, `[]any` without `items` |
| no `type` and no format | `any` |
| union (`oneOf`, `anyOf`, type list, `if` with `then` and `else`) | a union struct, see [Unions](#unions) |

Formats are matched in any case. A string format not in the table gives `string`.

The `[]byte` of `format: byte` is base64 text wherever it goes: JSON, a form field, a multipart
part, a parameter or a header. Text that is no base64 is a 400. A multipart file part into a
`[]byte` field gives the bytes of the file.

### Your own type for a format

`models.format-types` gives a format a Go type of your own, in place of the one the table gives:

```yaml
models:
  format-types:
    uuid: {type: uuid.UUID, import: github.com/google/uuid}
    ipv4: {type: netip.Addr, import: net/netip}
```

```go
type DeviceID = uuid.UUID

type Device struct {
	ID      DeviceID    `json:"id"`
	Address *netip.Addr `json:"address,omitempty"`
}
```

- `type` is read as [`x-go-type`](extensions.md#x-go-type) reads it. Without `import`, the package
  comes from the config's `imports`, else from the standard library package before the dot.
- It applies where the table does: a string, integer, number or boolean schema, or one without a
  type. An enum keeps its base type and an object stays a struct.
- `x-go-type` on a schema wins.
- The type is not validated, so `minLength` or `pattern` next to such a format are not checked, and
  a union takes any JSON for it.
- A parameter or header of such a type needs one that implements `encoding.TextMarshaler` and
  `encoding.TextUnmarshaler`. A union parameter with such a member gets no field, with a warning.

A schema without `type` is read from its other keywords: `properties` or `additionalProperties`
make an object, `items` makes an array, a known format picks the type the table gives it
(`format: binary` gives `runtime.File`), and a `const` picks the type of its value.

`runtime` is `github.com/mockzilla/mockzilla-codegen/pkg/runtime`. It uses the standard library only.

- `runtime.Date` holds a calendar date and reads and writes `2006-01-02`.
- `runtime.Email` is a string. Decoding accepts any string; `Validate` checks the address.
- `runtime.File` holds binary content from bytes, a reader or a multipart form. In JSON it is base64.

## Pointers

| Field | Go type |
|---|---|
| required | `T` |
| optional | `*T` |
| required and nullable | `*T` |
| optional and nullable | `*T` |
| slice, map, `any`, `json.RawMessage`, `[]byte`, or a named type over one of them | `T`, whatever the rules above say, since `nil` already means absent |
| array item or map value | `T`, or `*T` when the item or value is nullable |

A schema is nullable with `nullable: true` (3.0), `null` in its type list (3.1), or `null` among its
enum values. A `$ref` to a nullable schema is nullable too.

Optional fields get `omitempty` in their JSON tag. Required fields do not, apart from `readOnly` and
`writeOnly` ones. A field with `omitempty` that holds a struct or a type from another package by
value, such as `time.Time`, also gets `omitzero`: `omitempty` alone never leaves out a struct.

## Recursion

A struct cannot hold itself by value. When a type reaches itself through fields that are not
pointers, slices or maps, every field on that loop becomes a pointer, even a required one:

```yaml
Node:
  type: object
  required: [next]
  properties:
    next: {$ref: '#/components/schemas/Node'}
    children: {type: array, items: {$ref: '#/components/schemas/Node'}}
```

```go
type Node struct {
	Next     *Node  `json:"next"`
	Children []Node `json:"children,omitempty"`
}
```

Aliases that refer to each other in a loop (`A: {$ref: B}`, `B: {$ref: A}`) cannot be written in Go:
the first one becomes `any`, with a warning.

## allOf

The members of an `allOf` are merged into one type, deeply: properties, `required`, `items`,
`additionalProperties`, limits and flags. A `$ref` member brings everything its target has, and a
member that is itself an `allOf` is merged too.

```yaml
Dog:
  allOf:
    - $ref: '#/components/schemas/Animal'
    - type: object
      required: [bark]
      properties:
        bark: {type: boolean}
```

```go
type Dog struct {
	Name string `json:"name"`
	Bark bool   `json:"bark"`
}
```

- A property in several members is merged the same way. Inline types that came from the
  referenced type keep the names they have there.
- Members that disagree on the type keep the first type, with a warning.
- A number and an integer give an integer.
- Limits keep the strictest value: the largest minimum, the smallest maximum, and the same for
  lengths, items and properties. On a tie the exclusive bound wins. Every `pattern` and every
  `multipleOf` is checked. A member without a type counts too: `name: {maxLength: 5}` limits the
  `name` its `$ref` brings.
- `enum` and `const` keep the values every member allows. `enum: [car, bike]` in one member and
  `const: car` in another give an enum of `car` alone; two enums keep the values both list. With no
  value in common, the first enum stays, with a warning (`allof-conflict`).
- A member with `x-go-type` makes the type an alias of that type. When another member adds
  properties, items, variants or another type, that is not generated, with a warning.
- An `allOf` that includes itself is an error; the loop is left out.
- The description comes from the schema and its inline members, never from a referenced type.
- An `allOf` of one `$ref` plus members that only add a description or flags is that `$ref`: no new
  type is made. `allOf: [{$ref: Pet}, {description: The owner's pet}]` is a `Pet`, and
  `nullable: true` in such a member makes the field nullable.

## Unions

`oneOf`, `anyOf`, a 3.1 type list with several types, and `if` with both `then` and `else` become a
struct with one field per variant. The field holds a pointer, or the type itself when it can be nil.

```yaml
PaymentMethod:
  oneOf:
    - $ref: '#/components/schemas/Card'
    - $ref: '#/components/schemas/BankAccount'
    - {type: string}
```

```go
type PaymentMethod struct {
	Card        *Card        `json:"-"`
	BankAccount *BankAccount `json:"-"`
	String      *string      `json:"-"`
}

func (p PaymentMethod) MarshalJSON() ([]byte, error)
func (p *PaymentMethod) UnmarshalJSON(data []byte) error
func (p PaymentMethod) Validate() error
```

Variant fields:

| Member | Field name | Field type |
|---|---|---|
| `$ref` | the referenced type | `*Card` |
| inline object, enum or union | title, else discriminator value, else `Option` + position from 1 | `*PaymentMethodOption2` |
| inline primitive, array or map | its Go type | `String *string`, `Int64 *int64`, `Time *time.Time`, `Strings []string`, `StringMap map[string]string` |
| type list member | its Go type; an object or enum takes the type name | `Object *TagObject` |

- A `null` member makes the union nullable and gets no field.
- One member plus `null` is no union: `oneOf: [{$ref: Pet}, {type: 'null'}]` is a nullable `Pet`.
- Members with the same Go type share one field, with an info diagnostic.
- A member that is the union itself is left out, with a warning.

Decoding, in `UnmarshalJSON`:

1. With a discriminator, its value picks the variant: the values the mapping lists for it, else the
   `const` or single-value `enum` of its discriminator property, else its component name. A 3.2
   `defaultMapping` takes any other value; without one, an unknown value is an error that lists the
   allowed ones. A missing property falls back to step 2.
2. Only variants that take the JSON kind are tried (object, array, string, number, boolean). An
   integer goes to integer variants before float ones.
3. Objects are ranked by required properties present less unknown keys. A variant with
   `additionalProperties: false` is ruled out by an unknown key. A variant that is itself a union
   ranks by the best of the objects it can be, at any depth. For `oneOf`, two variants that
   match exactly with the same rank are an error. Generation warns (`union-ambiguous`) when object
   variants of a `oneOf` without a discriminator require the same properties, or none: an object
   with only those always hits this error.
4. The first variant in that order that decodes is set. For `anyOf`, every variant whose required
   properties are present and that decodes is set.

`null` sets nothing. Decoding resets the union first.

`MarshalJSON` writes the variant that is set. When several are set, objects are merged, a later key
replacing an earlier one; otherwise the first set variant is written. Nothing set writes `null`.

With a discriminator, `MarshalJSON` also writes the discriminator value. An empty one gets the
variant's value when it has exactly one: `Pet{Cat: &Cat{}}` writes `{"kind":"cat"}`. The variant
itself is not changed. A value that decoding would not read as the variant set is an error, and
`Validate` reports the same: `kind: "dog" picks Dog, not Cat`.

A union of strings, numbers and booleans with no shared properties also gets `MarshalText` and
`UnmarshalText`, so it works as a parameter, a header or a form field. The text is the set variant
without JSON quotes. Text that reads as a JSON number or boolean is tried as one first, then as a
string. A union with nothing set has no text: sending it is an error. A query, header or cookie
parameter whose union has an object or array variant gets no field, with a warning, and so does a
list or map of such unions.

Any other union is one field of a form or multipart body: the text of a string, number or boolean
variant, the JSON of an object or array variant, `vertex=abc` or `vertex={"x":1,"y":2}`. The server
reads a field that is JSON as JSON and other text as a string. An object with additional
properties goes as JSON in a url-encoded form too, so its extra keys arrive.

A union that a form body holds, as the body or as a property at any depth, gets `UnmarshalForm`.
The server picks the variant of a form as it picks one of a JSON object, by the discriminator, then
by the required names, and reads each field with its type, a file part as a file. A union
property written with brackets, `vertex[x]=1&vertex[y]=2`, is read the same way. In a multipart
body the client writes the fields of the variant that is set. An object with additional
properties that a form holds gets `UnmarshalForm` too, which keeps the other names of the form.

`Validate` checks the count: exactly one for `oneOf`, at most one when nullable, at least one for
`anyOf`, anything for a nullable `anyOf`. With a discriminator it also checks the value, as above.

### Shared properties

Properties next to a `oneOf` or `anyOf`, or merged in by `allOf`, are fields of the union struct
next to the variants. `MarshalJSON` merges them with the variant; `UnmarshalJSON` fills both.

```yaml
Contact:
  allOf:
    - $ref: '#/components/schemas/Base'
    - oneOf:
        - $ref: '#/components/schemas/Email'
        - $ref: '#/components/schemas/Phone'
```

```go
type Contact struct {
	ID    string `json:"id"`
	Email *Email `json:"-"`
	Phone *Phone `json:"-"`
}
```

A type that extends a parent through `allOf`, where the parent lists it in its `oneOf` or `anyOf`,
is one of the parent's variants: it gets the parent's properties, not its union.

### if, then, else

With both branches, `then` and `else` are the variants, named after their `$ref` type, else `Then`
and `Else`. When `if` tests one property against a `const` or a single-value `enum`, that value
picks `then` and any other picks `else`; otherwise the branches are matched by shape. With one
branch, its properties join the type as optional fields.

## Enums

An enum is a named type over its base type with one constant per value, in spec order:

```go
type Status string

const (
	StatusActive     Status = "active"
	StatusInProgress Status = "in_progress"
)
```

- The base type comes from `type`, or from the values when there is none.
- `null` among the values makes the type nullable and gets no constant.
- A value that does not fit the type is left out, with a warning. In a string enum, numbers and
  booleans become strings.
- Repeated values get one constant.
- A `date-time` string with an enum stays a `string`, since constants cannot be `time.Time`.
- An enum on an object or an array, or with values of different kinds, is left out, with a warning.
- `naming.enum-prefix: false` drops the type name from constants, unless the short name is taken.
- `x-enum-names` names the constants as written. A name that is not exported gets a warning.

## additionalProperties

| Schema | Go type |
|---|---|
| no properties, `additionalProperties` schema `T` | `map[string]T` |
| no properties, `additionalProperties: true` or left out | `map[string]any` |
| no properties, `additionalProperties: false` | empty struct |
| properties and `additionalProperties` schema or `true` | struct with an `AdditionalProperties map[string]T` field |
| properties and `additionalProperties: false` or left out | struct |

The `AdditionalProperties` field has the JSON name `-`: generated `MarshalJSON` and `UnmarshalJSON`
methods write its keys next to the properties, and `Get` and `Set` read and write single keys.
`MarshalJSON` writes the properties first, then the additional keys sorted; a key that has the
name of a property is left out. The helpers live in the runtime package.

## readOnly and writeOnly

A `readOnly` or `writeOnly` field keeps its `required` flag but gets `omitempty`, since one struct
serves both requests and responses. A required `readOnly` id is a plain `string` that a request
leaves out.

## const

A `const` sets the type of a field and is checked by validation. It makes no constant.
