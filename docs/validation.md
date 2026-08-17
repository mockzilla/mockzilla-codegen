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
	var errs runtime.ValidationErrors
	errs.Append("name", runtime.MinLength(p.Name, 1))
	errs.Append("name", runtime.MaxLength(p.Name, 20))
	if p.Age != nil {
		errs.Append("age", runtime.Minimum(*p.Age, 0, false))
	}
	if p.Tags == nil {
		errs.Add("tags", "is required")
	}
	if p.Tags != nil {
		errs.Append("tags", runtime.MinItems(p.Tags, 1))
		for idx, item := range p.Tags {
			errs.Append(runtime.Index("tags", idx), runtime.Pattern(item, patternPetTagsItem))
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
| `minLength`, `maxLength` | strings | characters, not bytes |
| `pattern` | strings | Go `regexp` (RE2) |
| `format` | strings | `uuid`, `uri`, `uri-reference`, `ipv4`, `ipv6`, `hostname`, `date`, `date-time`, `email` |
| `minimum`, `maximum`, exclusive forms | numbers | 3.0 boolean and 3.1 numeric forms alike |
| `multipleOf` | numbers | |
| `minItems`, `maxItems`, `uniqueItems` | arrays | |
| `minProperties`, `maxProperties` | maps | |
| `const` | strings, numbers, booleans | |
| `enum` | enum types | the value is one of the constants |
| `required` | pointers, slices, maps | not nil |
| `oneOf`, `anyOf` | unions | how many variants are set, then each set variant |

- A value of a declared type is checked by calling its `Validate`: fields, array items, map values
  and union variants alike.
- An optional value that is `nil` is absent and not checked.
- A format that becomes its own Go type is checked by decoding: `date-time` is a `time.Time`,
  `date` a `runtime.Date`, `email` a `runtime.Email` whose `Validate` checks the address.
- A keyword that does not fit the Go type is left out: `minLength` on a number, a `const` of 2.5 on
  an integer.

### required

`required` on a pointer, slice or map field means it is not `nil`. A required field that is a plain
value, such as `Name string`, gets no presence check: after JSON decoding, a missing name and an
empty one look the same.

### Patterns

Patterns are compiled once, into package-level variables. `\uXXXX` escapes are rewritten to the
RE2 form `\x{XXXX}`. A pattern RE2 cannot compile, such as one with a lookahead or a
backreference, is not checked, with a warning that names it.

## Error paths

`runtime.ValidationErrors` collects every failed check with the path of its value:

```
name: must be at least 1 characters long; tags[1]: must match ^[a-z]+$; labels["team"]: must be at most 5 characters long
```

`errors.As` finds the `runtime.ValidationErrors` and each `runtime.ValidationError` in it. Map values
come out in key order, so the same value gives the same error text on every run.

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
