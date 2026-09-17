# Naming

Every Go name the generator writes comes from the same rules, so the same spec gives the same names
on every run. When two names clash, the generator picks a winner the same way every time and reports
each rename.

## Turning a spec name into a Go name

| Spec name | Go name | Rule |
|---|---|---|
| `client_address`, `client-address`, `client.address` | `ClientAddress` | Any character that is not a letter or a digit splits words, apart from the symbols below. |
| `fooBar`, `HTTPServer` | `FooBar`, `HTTPServer` | A capital starts a word. An upper-case run ends before its last capital when a lower-case letter follows. |
| `v2api`, `404NotFound` | `V2API`, `N404NotFound` | Digits stay with the word before them. A letter after them starts a new word. |
| `CLIENT_ADDRESS`, `SSN` | `ClientAddress`, `Ssn` | Every word is written with one capital, unless it is an initialism. |
| `userId`, `urls`, `userIDs`, `id2` | `UserID`, `URLs`, `UserIDs`, `ID2` | Initialisms stay upper case, also with a plural `s` or trailing digits. |
| `1st`, `200` | `N1st`, `N200` | A name that would start with a digit gets `N` in front. |
| `+1`, `-1`, `-created_at`, `@type` | `Plus1`, `Minus1`, `MinusCreatedAt`, `AtType` | `+` and `@` become words anywhere, `-` only at the start. |
| `1.5`, `v1.2` | `N1Dot5`, `V1Dot2` | A dot between two digits becomes `Dot`. |
| `$`, `>=`, `!=`, `_` | `Dollar`, `GreaterThanEqual`, `NotEqual`, `Underscore` | A name with no letters or digits spells out its symbols. |
| `naïve`, `Straße` | `Naive`, `Strasse` | Latin letters with accents lose them. |
| `日本語Name`, `名前` | `Name`, `X540D` | Other letters are dropped. When nothing is left, the name is `X` and the first character's code in hex. |
| empty string | `Empty` | |

Unexported names (function arguments, local variables) follow the same rules with the first word in
lower case: `HTTPServer` gives `httpServer`. A Go keyword or predeclared name gets `Val`: `type`
gives `typeVal`, `string` gives `stringVal`.

### Initialisms

The default initialisms are API, ASCII, CPU, CSS, DNS, EOF, HTML, HTTP, HTTPS, ID, IP, JSON, OAS,
RPC, SQL, SSH, TCP, TLS, TTL, UDP, URI, URL, UTF8, UUID and XML. Add your own in the config:

```yaml
naming:
  initialisms: [PSP, SSN]
```

An initialism matches a whole word in any case: `Id`, `id` and `ID` all give `ID`. It does not
match inside a word, so `userid` stays `Userid`.

## Names for types without a name

Schemas under `components` keep their own name. Everything else is named after where it sits.

| Origin | Name | Example |
|---|---|---|
| Inline object under a property | parent + property | `OrderClientAddress` |
| Inline array item | parent + `Item` | `OrderItem` |
| Inline `additionalProperties` value | parent + `Value` | `LabelsValue` |
| Inline union variant | parent + title, else discriminator value, else `Option` + position from 1 | `PaymentMethodCard`, `PaymentMethodOption2` |
| Request body | operation + `RequestBody` | `CreatePetRequestBody` |
| Request body, one of several media types | operation + media type + `RequestBody` | `CreatePetJSONRequestBody` |
| Response | operation + `Response` + status | `GetPetResponse200`, `GetPetResponse4XX`, `GetPetResponseDefault` |
| Response, one of several media types | operation + media type + `Response` + status | `GetPetJSONResponse200` |
| Frame of a streamed response (`itemSchema`, or the schema of `text/event-stream` and the line-delimited JSON types) | operation + `ResponseItem` | `ListEventsResponseItem` |
| Item of a component or a request body with `itemSchema` | parent + `Item` | `LinesItem`, `ChatRequestBodyItem` |
| Path parameters | operation + `PathParams` | `GetPetPathParams` |
| Query parameters | operation + `Query` | `GetPetQuery` |
| Header parameters | operation + `Headers` | `GetPetHeaders` |
| Cookie parameters | operation + `Cookies` | `GetPetCookies` |
| Service method input | operation + `ServiceRequestOptions` | `GetPetServiceRequestOptions` |
| Service method output | operation + `ResponseData` | `GetPetResponseData` |
| Enum constant | type + value | `StatusActive`, `LevelMinus1`, `StatusEmpty` |

Media types in a name: `application/json` gives `JSON`, `application/x-www-form-urlencoded` gives
`Form`, `multipart/form-data` gives `Multipart`, `text/plain` gives `Text`. Any other media type
gives its subtype without an `x-` prefix: `application/problem+json` gives `ProblemJSON`,
`application/x-ndjson` gives `Ndjson`. A wildcard subtype gives the type (`image/*` gives `Image`),
and `*/*` gives `Any`.

## Clashes

Two names clash when they are equal ignoring case: `PetId` and `PetID` clash. Names the generator
itself declares, such as `NewRouter`, are taken before anything from the spec.

When several things want the same name, it goes to the first in this order:

1. a name set with `x-go-name`
2. a schema under `components/schemas`
3. anything else under `components`
4. a type named after an operation
5. an inline type

Within one group the spec order decides. The others try a name that says where they come from, such
as `PetResponse` for a response called `Pet`. When that is taken too, they get the first free
number from 2: `PetResponse2`, `PetResponse3`.

Struct fields clash with the methods the generator puts on the struct: `Validate`, `MarshalJSON`
and `UnmarshalJSON` on every struct, plus `Get`, `Set` and the `AdditionalProperties` field on a
struct with additional properties. A property called `validate` gives the field `Validate2`. Enum
constants clash with every other name in the package.

Names are given in rounds: first every type named directly (components, operation types), then the
types inside them, one level at a time. An inline type is named after the final name of its parent,
so when `Client` is renamed to `ClientSchema`, its inline `address` becomes `ClientSchemaAddress`.

Every rename is reported as a `name-clash` diagnostic with the spec location of the renamed item.
Losing an `x-go-name` is a warning. Set `x-go-name` to choose a name yourself.
