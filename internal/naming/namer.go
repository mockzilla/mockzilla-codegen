// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

// Package naming turns spec names into Go names and settles clashes between them.
package naming

import (
	"go/token"
	"slices"
	"strconv"
	"strings"
)

var defaultInitialisms = []string{
	"API", "ASCII", "CPU", "CSS", "DNS", "EOF", "HTML", "HTTP", "HTTPS", "ID", "IP", "JSON", "OAS",
	"RPC", "SQL", "SSH", "TCP", "TLS", "TTL", "UDP", "URI", "URL", "UTF8", "UUID", "XML",
}

// Namer builds Go names. Suffix methods keep their first argument as is and convert the rest.
type Namer struct {
	initialisms map[string]string
}

// New returns a Namer that writes the default initialisms and extra in upper case.
func New(extra []string) *Namer {
	m := make(map[string]string, len(defaultInitialisms)+len(extra))
	for _, s := range slices.Concat(defaultInitialisms, extra) {
		if s != "" {
			m[strings.ToLower(s)] = strings.ToUpper(s)
		}
	}
	return &Namer{initialisms: m}
}

// Exported joins parts into one exported CamelCase identifier.
func (n *Namer) Exported(parts ...string) string {
	return strings.Join(n.words(parts), "")
}

// Unexported joins parts into one lowerCamel identifier. A keyword or predeclared name gets "Val".
func (n *Namer) Unexported(parts ...string) string {
	ws := n.words(parts)
	ws[0] = strings.ToLower(ws[0])

	name := strings.Join(ws, "")
	if token.IsKeyword(name) || predeclared[name] {
		return name + "Val"
	}
	return name
}

// InlineProperty names the type of an inline schema under a property: OrderClientAddress.
func (n *Namer) InlineProperty(parent, prop string) string {
	return parent + n.camel(prop)
}

// ArrayItem names the type of an inline array item: OrderItem.
func (n *Namer) ArrayItem(parent string) string {
	return parent + "Item"
}

// MapValue names the type of an inline additionalProperties schema: LabelsValue.
func (n *Namer) MapValue(parent string) string {
	return parent + "Value"
}

// UnionVariant uses the title, else the discriminator value, else Option and the position from 1.
func (n *Namer) UnionVariant(parent, title, discValue string, index int) string {
	switch {
	case title != "":
		return parent + n.camel(title)
	case discValue != "":
		return parent + n.camel(discValue)
	}
	return parent + "Option" + strconv.Itoa(index+1)
}

// RequestBody adds the media type only when the operation has more than one body.
func (n *Namer) RequestBody(op, contentType string, multiple bool) string {
	if !multiple {
		return op + "RequestBody"
	}
	return op + n.MediaTag(contentType) + "RequestBody"
}

// Response names an inline response: GetPetResponse200, GetPetResponse4XX, GetPetResponseDefault.
// The media type is added only when the response has more than one: GetPetJSONResponse200.
func (n *Namer) Response(op, status, contentType string, multiple bool) string {
	if multiple {
		op += n.MediaTag(contentType)
	}
	return op + "Response" + n.Status(status)
}

// ResponseItem names the inline schema of one frame of a streamed response: GetEventsResponseItem.
func (n *Namer) ResponseItem(op string) string {
	return op + "ResponseItem"
}

// ResponseHeaders names the typed headers of a response: GetPetResponse200Headers.
func (n *Namer) ResponseHeaders(op, status string) string {
	return n.Response(op, status, "", false) + "Headers"
}

// ResponseConstructor names the function that makes the response data of an operation from the
// body of one status: NewGetPetResponseData, or NewGetPetResponseData404 among several.
func (n *Namer) ResponseConstructor(op, status string, multiple bool) string {
	name := "New" + n.ResponseData(op)
	if multiple {
		name += n.Status(status)
	}
	return name
}

// Status writes a status as a name part: 200, 4XX, Default.
func (n *Namer) Status(status string) string {
	if strings.EqualFold(status, "default") {
		return "Default"
	}
	return strings.ToUpper(n.camel(status))
}

// Params names the struct for one parameter location: GetPetPathParams, GetPetQuery.
func (n *Namer) Params(op, in string) string {
	switch in {
	case "path":
		return op + "PathParams"
	case "query":
		return op + "Query"
	case "header":
		return op + "Headers"
	case "cookie":
		return op + "Cookies"
	}
	return op + n.camel(in) + "Params"
}

// ServiceRequestOptions names what the service method of an operation receives.
func (n *Namer) ServiceRequestOptions(op string) string {
	return op + "ServiceRequestOptions"
}

// ResponseData names what the service method of an operation returns.
func (n *Namer) ResponseData(op string) string {
	return op + "ResponseData"
}

// ClientRequestOptions names what the client method of an operation sends.
func (n *Namer) ClientRequestOptions(op string) string {
	return op + "RequestOptions"
}

// ClientResponse names what the WithResponse client method of an operation returns.
func (n *Namer) ClientResponse(op string) string {
	return op + "Response"
}

// ClientOption names the option type of a client: ClientOption.
func (n *Namer) ClientOption(client string) string {
	return client + "Option"
}

// Interface names the interface of a service or a client: ServiceInterface, ClientInterface.
func (n *Namer) Interface(name string) string {
	return name + "Interface"
}

// ToolInput names what the MCP tool of an operation receives.
func (n *Namer) ToolInput(op string) string {
	return op + "ToolInput"
}

// EnumConst names an enum constant prefixed with its type: StatusActive.
func (n *Namer) EnumConst(typ, value string) string {
	return typ + n.camel(value)
}

// Snake writes a Go name in snake case: GetPetByID gives get_pet_by_id, HTTP2Stats http2_stats.
func (n *Namer) Snake(name string) string {
	ws := rawWords(name, n.initialisms)
	for i, w := range ws {
		ws[i] = strings.ToLower(w)
	}
	return strings.Join(ws, "_")
}

// MediaTag shortens a media type: application/problem+json gives ProblemJSON.
func (n *Namer) MediaTag(contentType string) string {
	mt, _, _ := strings.Cut(contentType, ";")
	mt = strings.ToLower(strings.TrimSpace(mt))
	switch mt {
	case "application/x-www-form-urlencoded":
		return "Form"
	case "multipart/form-data":
		return "Multipart"
	case "text/plain":
		return "Text"
	}

	typ, sub, _ := strings.Cut(mt, "/")
	sub = strings.TrimPrefix(sub, "x-")
	switch {
	case sub != "" && sub != "*":
		return n.camel(strings.ReplaceAll(sub, "+", "."))
	case typ != "" && typ != "*":
		return n.camel(typ)
	}
	return "Any"
}

// words converts parts into cased words; the first word never starts with a digit.
func (n *Namer) words(parts []string) []string {
	var ws []string
	for _, p := range parts {
		for _, w := range rawWords(p, n.initialisms) {
			ws = append(ws, n.caseWord(w))
		}
	}

	switch {
	case len(ws) == 0:
		return []string{"Empty"}
	case isDigit(rune(ws[0][0])):
		return slices.Insert(ws, 0, "N")
	}
	return ws
}

func (n *Namer) camel(part string) string {
	var b strings.Builder
	for _, w := range rawWords(part, n.initialisms) {
		b.WriteString(n.caseWord(w))
	}
	return b.String()
}

// caseWord keeps initialisms canonical, also with a plural "s" or trailing digits: IDs, HTTP2.
func (n *Namer) caseWord(w string) string {
	lw := strings.ToLower(w)
	if c, ok := n.initialisms[lw]; ok {
		return c
	}

	letters := strings.TrimRight(lw, "0123456789")
	digits := lw[len(letters):]
	if c, ok := n.initialisms[letters]; ok {
		return c + digits
	}
	if base, ok := strings.CutSuffix(letters, "s"); ok {
		if c, found := n.initialisms[base]; found {
			return c + "s" + digits
		}
	}
	return strings.ToUpper(lw[:1]) + lw[1:]
}
