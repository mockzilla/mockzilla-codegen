# Validation

Generated types check the constraints of the spec with a plain Go `Validate() error` method. There
is no reflection and no validation library: each check is a call into the runtime package.

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
or through a type it holds. Aliases never do: the checks of an alias apply where it is used. A type
that checks nothing has no `Validate`.

`models.validation.skip: true` turns validation off.

## Checks

| Keyword | Applies to | Check |
|---|---|---|
| `minLength`, `maxLength` | strings | characters, not bytes; for `format: byte` the base64 text |
| `pattern` | strings | ECMA-262, see [patterns](#patterns); for `format: byte` the base64 text |
| `format` | strings | `uuid`, `uri`, `uri-reference`, `ipv4`, `ipv6`, `hostname`, `date`, `date-time`, `email` |
| `minimum`, `maximum`, exclusive forms | numbers | the boolean and the numeric form in every version |
| `multipleOf` | numbers | exact, on the decimal the Go type writes: a `float32` 0.07 is a multiple of 0.01 |
| `minItems`, `maxItems`, `uniqueItems` | arrays | |
| `minProperties`, `maxProperties` | maps | |
| `propertyNames` | maps | each key: length, `pattern`, `format`, `const` and `enum` |
| `const` | strings, numbers, booleans | |
| `enum` | enum types | the value is one of the constants |
| `enum` | objects, arrays, unions, `any` | the value written as JSON is one of the values, each read into the Go type and written back |
| `required` | pointers, slices, maps | not nil |
| `oneOf`, `anyOf` | unions | how many variants are set, the discriminator value, then each set variant |

- A value of a declared type is checked by calling its `Validate`: fields, array items, map values
  and union variants alike.
- An optional value that is `nil` is absent and not checked.
- An enum compared as JSON matches a value as Go holds it: the value `{"x": 0, "y": null}` matches
  `Corner{X: new(0)}`, since a nil `Y` is how Go holds `null`. Key order and number form do not
  count. A value that does not fit the schema is left out, with a warning (`enum-value`).
- A format that becomes its own Go type is checked by decoding: `date-time` is a `time.Time`,
  `date` a `runtime.Date`, `email` a `runtime.Email` whose `Validate` checks the address. So is a
  format that `models.format-types` maps to a type of your own.
- An email address is checked as RFC 5321 writes it: `a@example.com`, `"a b"@example.com`,
  `a@[10.0.0.1]`. Only ASCII: `josé@example.com` is no `email`.
- A keyword that does not fit the Go type is left out: `minLength` on a number, a `const` of 2.5 on
  an integer.
- Under `allOf`, every member's limits hold: the strictest of each is checked, and every `pattern`
  and `multipleOf`. An `enum` or `const` keeps the values all members allow.
- `exclusiveMinimum` and `exclusiveMaximum` are read by their value: a number is the bound, `true`
  or `false` makes `minimum` or `maximum` exclusive. The form of the other version, a number in a
  3.0 spec or a boolean in a 3.1 one, still counts, with a warning (`keyword-version`).
- A keyword whose value is of the wrong kind, such as `minLength: abc` or `required: true` on a
  property, is left out, with a warning (`keyword-invalid`). A schema that is no object, such as
  `name: string`, reads as any, with a warning (`schema-invalid`).
- `patternProperties`, `prefixItems`, `not`, `contains`, `minContains`, `maxContains`,
  `dependentRequired`, `dependentSchemas`, `unevaluatedProperties` and `unevaluatedItems` are not
  checked, with a warning (`keyword-unsupported`). So is `propertyNames` on a struct without
  additional properties, which keeps no other keys to check.

### required

`required` on a pointer, slice or map field means it is not `nil`. A required field that is a plain
value, such as `Name string`, gets no presence check: after JSON decoding, a missing name and an
empty one look the same. A server with `validation.request` checks that the key is in the request
body before it decodes it, see [request bodies](server.md#request-bodies).

### Patterns

A pattern is ECMA-262, the dialect of JSON Schema. It is compiled once, into a package-level
variable, written in Go's syntax with the meaning ECMA-262 gives it:

| Spec | Go | Why |
|---|---|---|
| `\s`, `\S` | `[\t-\r \xA0\x{1680}...]`, `[^...]` | Go's `\s` knows ASCII spaces only |
| `.` | `[^\n\r\x{2028}\x{2029}]` | Go's `.` matches `\r` and U+2028 |
| `[^]`, `[]` | `[\s\S]`, `[^\s\S]` | any character, none |
| `\cJ` | `\n` | a control character |
| `\uXXXX` | `\xHH`, `\x{XXXX}` | Go has no `\u` |
| `[\b]` | `\x08` | a backspace |
| `[[:alpha:]]` | `[\[:alpha:]]` | a `[` in a class is a character, not a POSIX class |

An escape ECMA-262 does not have, such as `\A` or `\z`, keeps Go's meaning. A pattern Go cannot
compile, such as one with a lookahead or a backreference, is not checked, with a warning that
names it. An error quotes the pattern as the spec writes it: `tags[1]: must match ^[a-z]+$`.

## Error paths

`validation.Errors`, of the package `pkg/runtime/validation`, collects every failed check with the
path of its value:

```
name: must be at least 1 characters long; tags[1]: must match ^[a-z]+$; labels["team"]: must be at most 5 characters long
```

`errors.As` finds the `*validation.Errors` and each `validation.Error` in it. Map values
come out in key order, so the same value gives the same error text on every run. A key that fails
`propertyNames` is reported under the path of its value: `labels["Bad Key"]: must match ^[a-z]+$`.

Each error also names the keyword that failed, `Rule`, and its value in the spec, `Limit`, so a
service can write messages of its own:

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

`type` and `additionalProperties` come from the server's check of a request body: a `null` the
schema does not take, a key a closed object does not take. A key that fails `propertyNames` has
the rule of the check it failed. An error added with `Add` has no rule.

## readOnly and writeOnly

A `readOnly` value only comes in responses, a `writeOnly` one only in requests. `Validate` checks a
value as a request carries it: it leaves out `readOnly` fields. With
`models.validation.response: true`, types whose response checks differ also get
`ValidateResponse`, which leaves out `writeOnly` fields and calls `ValidateResponse` on the types it
holds.

```go
user.Validate()         // name and password
user.ValidateResponse() // id, name and roles
```
