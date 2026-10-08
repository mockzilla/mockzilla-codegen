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

A named type is:

| Schema | Named type |
|---|---|
| object | a struct |
| enum | a type over its base type |
| array | `type Pets []Pet` |
| map | `type Labels map[string]string` |
| anything else | an alias, such as `type PetID = string` |

A component that is only a `$ref` is an alias of the referenced type.

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

### Schemas without a type

A schema without `type` is read from its other keywords:

| Keyword | Type |
|---|---|
| `properties` or `additionalProperties` | an object |
| `items` | an array |
| a known format | the type the table gives it (`format: binary` gives `runtime.File`) |
| `const` | the type of its value |

### Bytes

The `[]byte` of `format: byte` is base64 text wherever it goes: JSON, a form field, a multipart
part, a parameter or a header.

- Text that is not base64 is a 400.
- A multipart file part into a `[]byte` field gives the bytes of the file.

### The runtime package

`runtime` is `github.com/mockzilla/mockzilla-codegen/pkg/runtime`. It uses the standard library
only.

| Type | What it is |
|---|---|
| `runtime.Date` | a calendar date, read and written as `2006-01-02` |
| `runtime.Email` | a string. Decoding accepts any string; `Validate` checks the address |
| `runtime.File` | binary content from bytes, a reader or a multipart form. In JSON it is base64 |

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
- The type is not validated. A `minLength` or `pattern` next to such a format is not checked, and
  a union takes any JSON for it.
- A parameter or header of such a type needs one that implements `encoding.TextMarshaler` and
  `encoding.TextUnmarshaler`.
- A union parameter with such a member gets no field, with a warning.

## Pointers

An optional field can be a pointer, a `runtime.Nullable[T]` or a plain value. The default is a
pointer. `models.nullable` changes the default for every field. An extension changes one field.

| Choice | How | Optional field | `null` and absent |
|---|---|---|---|
| pointer | the default | `*T` | one state: `nil` |
| `runtime.Nullable[T]` | `models.nullable: true` for every field, `x-go-nullable: true` on one | `runtime.Nullable[T]` | two states, see [Nullable](#nullable) |
| plain value | `x-go-type-skip-optional-pointer` on one field | `T` | the zero value counts as absent |

### Pointer or value

By default, a field is a pointer or a value like this:

| Field | Go type |
|---|---|
| required | `T` |
| optional | `*T` |
| required and nullable | `*T` |
| optional and nullable | `*T` |
| slice, map, `any`, `json.RawMessage`, `[]byte`, or a named type over one of them | `T` in every case above, since `nil` already means absent |
| array item or map value | `T`, or `*T` when the item or value is nullable |

A schema is nullable when it has one of these:

- `nullable: true` (3.0)
- `null` in its type list (3.1)
- `null` among its enum values

A `$ref` to a nullable schema is nullable too.

### JSON tags

Optional fields get `omitempty` in their JSON tag. Required fields do not, apart from `readOnly`
and `writeOnly` ones.

Two cases get `omitzero`:

- A field with `omitempty` that holds a struct or a type from another package by value, such as
  `time.Time`, also gets `omitzero`. `omitempty` alone never leaves out a struct.
- A slice or map gets `omitzero` instead of `omitempty`. `nil` is left out, and an empty one is
  sent as `[]` or `{}`.

### Plain values

`x-go-type-skip-optional-pointer` makes an optional field a plain `T`. Its zero value then counts
as absent:

- `Validate` skips its checks.
- Parameters, multipart forms and typed headers leave it out, as JSON does.

A zero value set on purpose is not sent either. That is the price of having no pointer.

### Nullable

A pointer cannot tell `null` from a property that was left out: both are `nil`. With
`models.nullable: true`, a field that may hold no value is a `runtime.Nullable[T]` instead:

```yaml
models:
  nullable: true
```

| Field | Go type |
|---|---|
| required | `T` |
| optional, nullable, or both | `runtime.Nullable[T]` |
| slice or map | `T` |
| `any`, `json.RawMessage` | `T` |
| nullable array item or map value | `runtime.Nullable[T]` |

This covers properties, query, header and cookie parameters, and typed response headers. Request
and response bodies stay pointers.

```go
type PetPatch struct {
	Nickname runtime.Nullable[string] `json:"nickname,omitzero"`
}

p.Nickname = runtime.Some("Rex")    // "nickname": "Rex"
p.Nickname = runtime.Null[string]() // "nickname": null
var p PetPatch                      // nickname is left out

if name, ok := p.Nickname.Get(); ok { // false when absent and when null
	use(name)
}
p.Nickname.IsNull() // sent as null
p.Nickname.IsSet()  // sent at all, as a value or null
p.Nickname.Or("none")
```

- The zero value is absent, and `omitzero` leaves it out.
- A required nullable field has no `omitzero`, so one left unset is written as `null`.
- `Validate` reports `must not be null` for a field set to null that the spec does not let be null.
- A type can hold a `Nullable` of itself, so a field on a loop needs no pointer.

[examples/models/nullable](../examples/models/nullable) applies a PATCH that keeps, clears and
sets fields.

#### x-go-nullable

`x-go-nullable` on a property, a parameter or its schema wins over the config:

| Value | Field |
|---|---|
| `true` | a `Nullable`, a slice or map too |
| `false` | keeps the pointer |

`x-go-nullable` is left out, with a warning, on a required field that is not nullable, or on one
that `x-go-type-skip-optional-pointer` makes a plain value.

### Defaults

An optional parameter or property with a `default` gets a getter. It returns the field, or the
default when the field is `nil`, and works on a nil receiver too. Client, server and models alone
get the same getters.

```go
// GetLimit returns Limit, or 20 when it is nil.
func (l *ListPetsQuery) GetLimit() int {
	if l == nil || l.Limit == nil {
		return 20
	}
	return *l.Limit
}
```

The default has to be something Go writes as a constant:

- a string, number or boolean
- an enum value, which comes back as its constant
- a list of these, which is a new slice on each call

These get no getter:

- A default that is any other value, such as an object, a date or an `x-go-type`. `-v` says why
  (`getter-skipped`).
- A required field.
- A path parameter.
- A field that `x-go-type-skip-optional-pointer` makes a plain value.

A default that does not fit its schema is left out, and generation warns (`default-ignored`).

#### Nullable fields

A nullable field is `nil` when it is absent and when it is `null`, so its getter returns the
default for both. A `runtime.Nullable` field's getter returns `Or(default)`, which does the same.

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
	Children []Node `json:"children,omitzero"`
}
```

Aliases that refer to each other in a loop (`A: {$ref: B}`, `B: {$ref: A}`) cannot be written in Go.
The first one becomes `any`, with a warning.

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

### Properties and types

- A property in several members is merged the same way.
- Inline types that came from the referenced type keep the names they have there.
- Members that disagree on the type keep the first type, with a warning.
- A number and an integer give an integer.

### Limits

Limits keep the strictest value:

- the largest minimum and the smallest maximum, and the same for lengths, items and properties
- on a tie, the exclusive bound wins
- every `pattern` and every `multipleOf` is checked

A member without a type counts too: `name: {maxLength: 5}` limits the `name` its `$ref` brings.

### enum and const

`enum` and `const` keep the values every member allows.

- `enum: [car, bike]` in one member and `const: car` in another give an enum of `car` alone.
- Two enums keep the values both list.
- With no value in common, the first enum stays, with a warning (`allof-conflict`).

### Members with x-go-type

A member with `x-go-type` makes the type an alias of that type. When any other member, before or
after it, adds properties, items, variants or another type, what it adds is not generated, with a
warning.

### A $ref plus extras

An `allOf` of one `$ref` plus members that only add a description, a default or flags is that
`$ref`: no new type is made. `allOf: [{$ref: Pet}, {description: The owner's pet}]` is a `Pet`.

- `nullable: true` in such a member makes the field nullable.
- A `default` in such a member gives the field a getter.

A member that lists `required` makes a new type: `allOf: [{$ref: Pet}, {required: [name]}]` is a
struct whose `name` is required.

### Loops and descriptions

- An `allOf` that includes itself is an error. The loop is left out.
- The description comes from the schema and its inline members, never from a referenced type.

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

### Variant fields

| Member | Field name | Field type |
|---|---|---|
| `$ref` | the referenced type | `*Card` |
| inline object, enum or union | title, else discriminator value, else `Option` + position from 1 | `*PaymentMethodOption2` |
| inline primitive, array or map | its Go type | `String *string`, `Int64 *int64`, `Time *time.Time`, `Strings []string`, `StringMap map[string]string` |
| type list member | its Go type; an object or enum takes the type name | `Object *TagObject` |

### Special members

- A `null` member makes the union nullable and gets no field.
- One member plus `null` is not a union: `oneOf: [{$ref: Pet}, {type: 'null'}]` is a nullable
  `Pet`.
- Members with the same Go type share one field, with an info diagnostic. Each keeps its own
  checks, see [Validate](#validate).
- A member that is the union itself is left out, with a warning.
- A `oneOf` or `anyOf` whose members only list `required` is not a union. The struct checks which
  of those lists it has, see [Required lists](#required-lists).

### oneOf and anyOf

Both become the same struct. A union the spec does not describe gets a doc line that names its
kind:

- `Pet is one of Cat or Dog.`
- `Person is any of Named or Aged.`

The methods follow the kind:

| | `oneOf` | `anyOf` |
|---|---|---|
| `UnmarshalJSON` | sets the variant that matches best; two objects that rank the same are an error | sets every variant that matches |
| `MarshalJSON` | writes the variant that is set; more than one set is an error | writes the variants that are set, objects merged |
| `Validate` | one variant set, or none when nullable; checks that variant | one or more set, or none when nullable; passes when one set variant passes its checks |

#### Decoding

`UnmarshalJSON` works in these steps:

1. With a discriminator, its value picks the variant.
   - A variant is known by the values the mapping lists for it, else by the `const` or
     single-value `enum` of its discriminator property, else by its component name.
   - A 3.2 `defaultMapping` takes any other value. Without one, an unknown value is an error that
     lists the allowed ones.
   - A missing property falls back to step 2.
   - A discriminator whose property is not a string is ignored, with a warning: mapping keys are
     strings.
2. Only variants that take the JSON kind are tried (object, array, string, number, boolean). An
   integer goes to integer variants before float ones.
3. An object must have the required properties of a variant.
   - When it has those of none, decoding fails and says what each variant needs:
     `Cat needs meow, Dog needs bark`.
   - A variant with `additionalProperties: false` is ruled out by an unknown key.
   - A variant that is itself a union matches when one of the objects it can be matches, at any
     depth.
4. Objects are ranked by required properties present less unknown keys.
5. A variant that decodes passes when the value passes the `Validate` of the variant's type: an
   enum, a pattern, a struct with checks.
   - For `oneOf`, the first variant in that order that passes is set, else the first that decodes,
     so `Validate` reports why.
   - For `anyOf`, every variant that passes is set, else every one that decodes.
6. For `oneOf`, two objects of the same rank among those step 5 picks from are an error:
   `{"meow":true,"bark":true}` is a Cat and a Dog alike.
   - Generation warns (`union-ambiguous`) when object variants of a `oneOf` without a
     discriminator require the same properties, or none.
   - An object with only those properties hits this error unless their checks tell them apart.

`null` sets nothing. Decoding resets the union first.

#### Encoding

`MarshalJSON` writes the variant that is set.

- A `oneOf` with more than one variant set is an error.
- An `anyOf` with several set merges objects, a later key replacing an earlier one. Otherwise it
  writes the first set variant.
- Nothing set writes `null`.

With a discriminator, `MarshalJSON` also writes the discriminator value.

- An empty discriminator gets the variant's value when the variant has exactly one:
  `Pet{Cat: &Cat{}}` writes `{"kind":"cat"}`. The variant itself is not changed.
- A value that decoding would not read as the variant set is an error, and `Validate` reports the
  same: `kind: "dog" picks Dog, not Cat`.

#### A variant with several values

Several discriminator values can pick one variant. If that variant has no such property, it cannot
keep the value. The union then gets a field for it, `ServerType *string`, unless the union has the
property already.

Decoding fills the field. When you build the union in code, set it by hand: `Server{Web: &Web{}}`
fails with `server_type: must be set, Web takes nginx or apache`.

#### Text

A union of strings, numbers and booleans with no shared properties also gets `MarshalText` and
`UnmarshalText`, so it works as a parameter, a header or a form field.

- The text is the set variant without JSON quotes.
- Text that reads as a JSON number or boolean is tried as one first, then as a string.
- A union with nothing set has no text: sending it is an error.
- A query, header or cookie parameter whose union has an object or array variant gets no field,
  with a warning. So does a list or map of such unions.

#### Forms

Any other union is one field of a form or multipart body:

- a string, number or boolean variant goes as its text
- an object or array variant goes as its JSON

For example, `vertex=abc` or `vertex={"x":1,"y":2}`. The server reads a field that is JSON as JSON
and other text as a string. An object with additional properties goes as JSON in a url-encoded
form too, so its extra keys arrive.

A union that a form body holds, as the body or as a property at any depth, gets `UnmarshalForm`.

- The server picks the variant of a form as it picks one of a JSON object: by the discriminator,
  then by the required names.
- It reads each field with its type, and a file part as a file.
- A union property written with brackets, `vertex[x]=1&vertex[y]=2`, is read the same way.
- In a multipart body the client writes the fields of the variant that is set.

An object with additional properties that a form holds gets `UnmarshalForm` too. It keeps the other
names of the form.

#### Validate

`Validate` checks the count of variants set:

| | Count | When nullable |
|---|---|---|
| `oneOf` | exactly one | at most one |
| `anyOf` | at least one | anything |

- A `oneOf` checks the variant that is set.
- An `anyOf` passes when one set variant passes its checks. Otherwise it reports the failed checks
  of every set variant.
- With a discriminator, `Validate` also checks the value, as `MarshalJSON` does.

The value `"2026-09-30T00:00:00Z"` sets both variants of this `anyOf`, and passes:

```yaml
anyOf: [{format: date-time}, {maxLength: 5}]
```

Members that share a field are checked one by one. This `anyOf` passes with a uuid or with
`my-slug`:

```yaml
anyOf: [{format: uuid}, {pattern: '^[a-z-]+$'}]
```

In a `oneOf`, exactly one of them must pass. A value that passes both fails with
`exactly one variant must match, found 2`.

### Required lists

A `oneOf` or `anyOf` whose members only list `required` checks which properties are set. It adds
no variant. `Validate` counts the members whose properties are all set:

```yaml
Lookup:
  type: object
  oneOf:
    - required: [id]
    - required: [email]
  properties:
    id: {type: integer}
    email: {type: string}
```

```go
func (l Lookup) Validate() error {
	var errs validation.Errors
	errs.Append("", validation.ExactlyOneOf("id or email", l.ID != nil, l.Email != nil))
	return errs.Err()
}
```

- An `anyOf` uses `AtLeastOneOf`.
- A required property that cannot be nil counts as set. An optional one with no pointer counts as
  set when it is not its zero value.
- The check is left out, with a warning, when a listed name is not a property of the struct.
- A member that only lists `required` next to members with a type or `null` is not checked either,
  with a warning.

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

#### A variant that extends the parent

A type can extend a parent through `allOf`, while the parent lists it in its `oneOf` or `anyOf`.
That type is one of the parent's variants. It gets the parent's properties, not its union.

### Several unions

A value can be several unions at once: an `allOf` of two `oneOf`s, a `oneOf` next to an `anyOf`,
or a `oneOf` next to an `if` with both branches. The variants of all of them are fields of one
struct, numbered on from one union to the next.

```yaml
Reminder:
  allOf:
    - oneOf:
        - $ref: '#/components/schemas/Email'
        - $ref: '#/components/schemas/Phone'
    - oneOf:
        - $ref: '#/components/schemas/Daily'
        - $ref: '#/components/schemas/Weekly'
```

```go
// Reminder is one of Email or Phone, and one of Daily or Weekly.
type Reminder struct {
	Email  *Email  `json:"-"`
	Phone  *Phone  `json:"-"`
	Daily  *Daily  `json:"-"`
	Weekly *Weekly `json:"-"`
}
```

Each union keeps its own kind, discriminator and checks.

- `UnmarshalJSON` reads every union from the same JSON. `{"address":"a@b.c","time":"09:00"}` sets
  `Email` and `Daily`. `{"address":"a@b.c"}` fails with `Daily needs time, Weekly needs weekday`.
- `MarshalJSON` merges the set variants.
- `Validate` counts the variants of each union on its own.
- A type that two unions list is one field.

### if, then, else

With both branches, `then` and `else` are the variants. They are named after their `$ref` type,
else `Then` and `Else`. With one branch, its properties join the type as optional fields.

Which branch a value picks:

- When `if` tests one property against a `const` or a single-value `enum` of a string, number or
  boolean, that value picks `then` and any other picks `else`. Otherwise the branches are matched
  by shape.
- As in JSON Schema, an object without the property picks `then`, unless the `if` lists it in
  `required`. So a value with `else` set needs the property, or `Validate` and `MarshalJSON`
  report it.
- A number or boolean is read and written as JSON: `true` does not match `"true"`, and `3` matches
  `3.0`.
- A value with `then` set and no property is written with the tested value.

## Enums

An enum is a named type over its base type with one constant per value, in spec order, and a
func that returns them:

```go
type Status string

const (
	StatusActive     Status = "active"
	StatusInProgress Status = "in_progress"
)

// StatusValues returns the values of Status.
func StatusValues() []Status {
	return []Status{
		StatusActive,
		StatusInProgress,
	}
}
```

### Values

- The base type comes from `type`, or from the values when there is none.
- `null` among the values makes the type nullable and gets no constant.
- A value that does not fit the type is left out, with a warning. In a string enum, numbers and
  booleans become strings.
- Repeated values get one constant.
- A `date-time` string with an enum stays a `string`, since constants cannot be `time.Time`.

### Enums without a type of their own

An enum on an object, an array or a union, or with values of different kinds, gets no type of its
own. `Validate` compares the value with its values as JSON, see [validation](validation.md#checks).

### Constant names

- `<Enum>Values` belongs to the enum: a constant or a type that wants the name gets a number. The
  value `values` of `Status` gives the constant `StatusValues2`.
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

The `AdditionalProperties` field has the JSON name `-`. Generated `MarshalJSON` and `UnmarshalJSON`
methods write its keys next to the properties, and `Get` and `Set` read and write single keys. The
helpers live in the runtime package.

`MarshalJSON` writes the properties first, then the additional keys sorted. A key that has the name
of a property is left out.

### Empty names

A property named `""` gets no field, since no JSON tag can spell an empty key. Generation warns
(`name-empty`). When the object takes additional properties, the `""` key lands in their map.

## readOnly and writeOnly

A `readOnly` or `writeOnly` field keeps its `required` flag but gets `omitempty`, since one struct
serves both requests and responses. A required `readOnly` id is a plain `string` that a request
leaves out.

## const

A `const` sets the type of a field and is checked by validation. It makes no constant.
