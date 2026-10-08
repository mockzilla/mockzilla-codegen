# Validation

Generated types check the constraints of the spec with a plain Go `Validate() error` method. There
is no reflection and no validation library: each check is a call into the runtime package.

A schema and the method generated for it:

```yaml
Pet:
  type: object
  required: [name, tags]
  properties:
    name: {type: string, minLength: 1, maxLength: 20}
    age: {type: integer, minimum: 0}
    tags: {type: array, minItems: 1, items: {type: string, pattern: '^[a-z]+$'}}
    owner: {$ref: '#/components/schemas/Owner'}
```

```go
var patternPetTagsItem = regexp.MustCompile(`^[a-z]+$`)

func (p Pet) Validate() error {
	var errs validation.Errors
	errs.Append("name", validation.MinLength(p.Name, 1))
	errs.Append("name", validation.MaxLength(p.Name, 20))
	if p.Age != nil {
		errs.Append("age", validation.Minimum(*p.Age, 0, false))
	}
	if p.Tags == nil {
		errs.Required("tags")
	}
	if p.Tags != nil {
		errs.Append("tags", validation.MinItems(p.Tags, 1))
		for idx, item := range p.Tags {
			errs.Append(validation.Index("tags", idx), validation.Pattern(item, patternPetTagsItem, `^[a-z]+$`))
		}
	}
	if p.Owner != nil {
		errs.Append("owner", p.Owner.Validate())
	}
	return errs.Err()
}
```

## Which types get Validate

A struct, union, enum, or named slice or map type gets `Validate` when it checks something, itself
or through a type it holds. A type that checks nothing has no `Validate`.

Aliases never get one: the checks of an alias apply where it is used.

`models.validation.skip: true` turns validation off.

## Checks

A value of a declared type is checked by calling its `Validate`. This holds for fields, array
items, map values and union variants alike. An optional value that is `nil` is absent and not
checked.

### Strings

| Keyword | Check |
|---|---|
| `minLength`, `maxLength` | length in characters, not bytes. For `format: byte`, the length of the base64 text |
| `pattern` | ECMA-262, see [patterns](#patterns). For `format: byte`, matched against the base64 text |
| `format` | one of the [formats](#formats) below |

### Numbers

| Keyword | Check |
|---|---|
| `minimum`, `maximum`, exclusive forms | the boolean and the numeric form, in every version. See [exclusive bounds](#exclusive-bounds) |
| `multipleOf` | exact, on the decimal the Go type writes: a `float32` 0.07 is a multiple of 0.01 |

### Arrays

| Keyword | Check |
|---|---|
| `minItems`, `maxItems` | the number of items |
| `uniqueItems` | no item repeats |

### Maps

| Keyword | Check |
|---|---|
| `minProperties`, `maxProperties` | the number of properties |
| `propertyNames` | each key: length, `pattern`, `format`, `const` and `enum` |

### Any value

| Keyword | Applies to | Check |
|---|---|---|
| `const` | strings, numbers, booleans | the value is the constant |
| `enum` | enum types | the value is one of the constants |
| `enum` | objects, arrays, unions, `any` | the value written as JSON is one of the values, each read into the Go type and written back. See [enums compared as JSON](#enums-compared-as-json) |
| `required` | pointers, slices, maps | not nil. See [required](#required) |
| `oneOf`, `anyOf` | unions | how many variants are set, the discriminator value, then each set variant |

### Formats

`format` checks a string against one of these: `uuid`, `uri`, `uri-reference`, `ipv4`, `ipv6`,
`hostname`, `date`, `date-time` and `email`.

A format that becomes its own Go type is checked by decoding:

| Format | Go type | Check |
|---|---|---|
| `date-time` | `time.Time` | decoding |
| `date` | `runtime.Date` | decoding |
| `email` | `runtime.Email` | decoding, then its `Validate` checks the address |

A format that `models.format-types` maps to a type of your own works the same way.

An email address is checked as RFC 5321 writes it: `a@example.com`, `"a b"@example.com`,
`a@[10.0.0.1]`. Only ASCII is valid: `josé@example.com` is not an `email`.

### Enums compared as JSON

An enum of objects, arrays, unions or `any` matches a value as Go holds it. The value
`{"x": 0, "y": null}` matches `Corner{X: new(0)}`, since a nil `Y` is how Go holds `null`.

Key order and number form do not count.

A value that does not fit the schema is left out, with a warning (`enum-value`).

### allOf

Under `allOf`, every member's limits hold:

- The strictest of each limit is checked.
- Every `pattern` and `multipleOf` is checked.
- An `enum` or `const` keeps the values all members allow.

### Exclusive bounds

`exclusiveMinimum` and `exclusiveMaximum` are read by their value:

| Value | Meaning |
|---|---|
| a number | the bound |
| `true` or `false` | makes `minimum` or `maximum` exclusive |

The form of the other version still counts, with a warning (`keyword-version`). That is a number
in a 3.0 spec, or a boolean in a 3.1 one.

### Keywords that cannot be checked

A keyword that does not fit the Go type is left out: `minLength` on a number, a `const` of 2.5 on
an integer.

A keyword with a value of the wrong kind is left out, with a warning (`keyword-invalid`). Examples
are `minLength: abc`, `required: true` on a property, and `items` holding a list. The rest of the
schema still counts.

A schema that is not an object, such as `name: string`, reads as any, with a warning
(`schema-invalid`).

These keywords are not checked, with a warning (`keyword-unsupported`):

- `patternProperties`
- `prefixItems`
- `not`
- `contains`, `minContains`, `maxContains`
- `dependentRequired`, `dependentSchemas`
- `unevaluatedProperties`, `unevaluatedItems`
- `propertyNames` on a struct without additional properties, which keeps no other keys to check

### required

`required` on a pointer, slice or map field means it is not `nil`.

A required field that is a plain value, such as `Name string`, gets no presence check. After JSON
decoding, a missing name and an empty one look the same. A server with `validation.request` checks
that the key is in the request body before it decodes it, see
[request bodies](server.md#request-bodies).

### Patterns

A pattern is ECMA-262, the dialect of JSON Schema. It is compiled once, into a package-level
variable. Generation writes it in Go's syntax with the meaning ECMA-262 gives it:

| Spec | Go | Why |
|---|---|---|
| `\s`, `\S` | `[\t-\r \xA0\x{1680}...]`, `[^...]` | Go's `\s` knows ASCII spaces only |
| `.` | `[^\n\r\x{2028}\x{2029}]` | Go's `.` matches `\r` and U+2028 |
| `[^]`, `[]` | `[\s\S]`, `[^\s\S]` | any character, none |
| `\cJ` | `\n` | a control character |
| `\uXXXX` | `\xHH`, `\x{XXXX}` | Go has no `\u` |
| `[\b]` | `\x08` | a backspace |
| `[[:alpha:]]` | `[\[:alpha:]]` | a `[` in a class is a character, not a POSIX class |

An escape ECMA-262 does not have, such as `\A` or `\z`, keeps Go's meaning.

A pattern Go cannot compile, such as one with a lookahead or a backreference, is not checked, with
a warning that names it.

An error quotes the pattern as the spec writes it: `tags[1]: must match ^[a-z]+$`.

## Error paths

`validation.Errors`, of the package `pkg/runtime/validation`, collects every failed check with the
path of its value:

```
name: must be at least 1 characters long; tags[1]: must match ^[a-z]+$; labels["team"]: must be at most 5 characters long
```

`errors.As` finds the `*validation.Errors` and each `validation.Error` in it.

Map values come out in key order, so the same value gives the same error text on every run. A key
that fails `propertyNames` is reported under the path of its value:
`labels["Bad Key"]: must match ^[a-z]+$`.

### Rule and Limit

Each error also names the keyword that failed, `Rule`, and its value in the spec, `Limit`. A
service can use them to write messages of its own:

```go
var errs *validation.Errors
if errors.As(err, &errs) {
	for _, e := range *errs {
		// e.Field "name", e.Rule validation.RuleMinLength, e.Limit 2
	}
}
```

| Rule | Limit |
|---|---|
| `minLength`, `maxLength`, `minItems`, `maxItems`, `minProperties`, `maxProperties` | the number, an `int` |
| `minimum`, `maximum`, `exclusiveMinimum`, `exclusiveMaximum`, `multipleOf` | the number, a `float64` |
| `pattern` | the pattern as the spec writes it |
| `format` | the name of the format |
| `const` | the value |
| `enum` | the values, a slice of the enum type, or of the Go type for an enum compared as JSON |
| `uniqueItems` | `true` |
| `additionalProperties` | `false` |
| `required`, `type`, `oneOf`, `anyOf`, `discriminator` | nil |

Two rules come from the server's check of a request body:

- `type`: a `null` the schema does not take.
- `additionalProperties`: a key a closed object does not take.

A key that fails `propertyNames` has the rule of the check it failed. An error added with `Add`
has no rule.

## readOnly and writeOnly

A `readOnly` value only comes in responses, a `writeOnly` one only in requests.

| Method | Checks a value as | Leaves out |
|---|---|---|
| `Validate` | a request carries it | `readOnly` fields |
| `ValidateResponse` | a response carries it | `writeOnly` fields |

`ValidateResponse` also calls `ValidateResponse` on the types it holds. It is generated with
`models.validation.response: true`, for the types whose response checks differ.

```go
user.Validate()         // name and password
user.ValidateResponse() // id, name and roles
```
