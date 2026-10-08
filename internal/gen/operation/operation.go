// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

// Package operation holds what the server, the client and the MCP generators share about an
// operation: the fields its parameters and bodies go in, the Go type of a body, the status code a
// response key names, how the runtime names a parameter's style, the encoding of a form body,
// and the parts the types of an operation are declared in.
package operation

import (
	"cmp"
	"maps"
	"slices"
	"strconv"
	"strings"

	"github.com/mockzilla/mockzilla-codegen/internal/gocode"
	"github.com/mockzilla/mockzilla-codegen/internal/gomodel"
	"github.com/mockzilla/mockzilla-codegen/internal/layout"
	"github.com/mockzilla/mockzilla-codegen/internal/naming"
	"github.com/mockzilla/mockzilla-codegen/internal/spec"
	"github.com/mockzilla/mockzilla-codegen/pkg/runtime"
)

const deprecatedNote = "Deprecated: the spec marks it deprecated."

// groupFields name the fields that hold the parameters of each location.
var groupFields = map[string]string{
	spec.InPath:   "PathParams",
	spec.InQuery:  "Query",
	spec.InHeader: "Headers",
	spec.InCookie: "Cookies",
}

// styleNames are the runtime constants of the parameter styles.
var styleNames = map[string]string{
	"simple":         "StyleSimple",
	"label":          "StyleLabel",
	"matrix":         "StyleMatrix",
	"form":           "StyleForm",
	"spaceDelimited": "StyleSpaceDelimited",
	"pipeDelimited":  "StylePipeDelimited",
	"deepObject":     "StyleDeepObject",
	"cookie":         "StyleCookie",
}

var bytesType = gomodel.Slice{Elem: gomodel.Builtin{Name: "byte"}}

// rangeMembers are the media types a body under a range is sent as, for the ranges that have one.
var rangeMembers = map[string]string{"text/*": "text/plain", "multipart/*": "multipart/form-data"}

// defaultStyles are the styles of parameters that name none, by location.
var defaultStyles = map[string]string{spec.InPath: "simple", spec.InQuery: "form", spec.InHeader: "simple", spec.InCookie: "form"}

// GroupField names the options field that holds the parameters of location in: PathParams,
// Query, Headers or Cookies.
func GroupField(in string, n *naming.Namer) string {
	return cmp.Or(groupFields[in], n.Exported(in))
}

// QueryStringField names the options field of the querystring of op, QueryString added on a clash.
func QueryStringField(op *gomodel.Operation, n *naming.Namer) string {
	name := n.Exported(op.QueryString.Param.Name)
	taken := append(BodyFields(op.Bodies, n), "RawRequest")
	for _, p := range op.Params {
		taken = append(taken, GroupField(p.In, n))
	}
	if slices.Contains(taken, name) {
		return name + "QueryString"
	}
	return name
}

// BodyFields names the options field of each body: Body for one, else Body and the tag of its
// media type. Two media types with one tag, such as application/xml and text/xml, are told apart
// by the type, then by a number.
func BodyFields(bodies []gomodel.Content, n *naming.Namer) []string {
	if len(bodies) == 1 {
		return []string{"Body"}
	}

	fields := make([]string, 0, len(bodies))
	for _, c := range bodies {
		field := "Body" + n.MediaTag(c.MediaType)
		if slices.Contains(fields, field) {
			typ, _, _ := strings.Cut(c.MediaType, "/")
			field = "Body" + n.Exported(typ) + n.MediaTag(c.MediaType)
		}
		base := field
		for i := 2; slices.Contains(fields, field); i++ {
			field = base + strconv.Itoa(i)
		}
		fields = append(fields, field)
	}
	return fields
}

// BodyType is the Go type a body field holds: the schema's type, held by pointer unless it can
// be nil, or any for JSON without a schema, a string for text and bytes for anything else.
func BodyType(c gomodel.Content) gomodel.Type {
	switch {
	case c.Type != nil:
		return gomodel.Held(c.Type)
	case runtime.IsJSON(c.MediaType):
		return gomodel.Builtin{Name: "any"}
	case strings.HasPrefix(BaseMediaType(c.MediaType), "text/"):
		return gomodel.Builtin{Name: "string"}
	}
	return bytesType
}

// QueryStringType is the Go type of the querystring field, held as a body is, any without a schema.
func QueryStringType(qs *gomodel.QueryString) gomodel.Type {
	if qs.Content.Type == nil {
		return gomodel.Builtin{Name: "any"}
	}
	return gomodel.Held(qs.Content.Type)
}

// FrameType is the type of one frame: the item type, or bytes for none and for text.
func FrameType(c gomodel.Content) gomodel.Type {
	if c.Item == nil || gomodel.JSONKinds(c.Item) == gomodel.JSONString {
		return bytesType
	}
	return c.Item
}

// BaseMediaType is a media type in lower case and without its parameters.
func BaseMediaType(mediaType string) string {
	base, _, _ := strings.Cut(strings.ToLower(mediaType), ";")
	return strings.TrimSpace(base)
}

// Concrete is the media type a body under mediaType is sent as: the member of a range that has one, else mediaType without parameters.
func Concrete(mediaType string) string {
	base := BaseMediaType(mediaType)
	return cmp.Or(rangeMembers[base], base)
}

// IsTextValue reports a type a text body holds as its text: a builtin, any included, or a type from a package such as time.Time.
func IsTextValue(t gomodel.Type) bool {
	switch gomodel.Underlying(t).(type) {
	case gomodel.Builtin, gomodel.Qualified:
		return true
	}
	return false
}

// Encoding writes the runtime.Encoding of a form body, nil when it declares none.
func Encoding(c gomodel.Content, runtimePkg string) string {
	if len(c.Encoding) == 0 {
		return "nil"
	}
	elems := make([]gocode.KeyValue, 0, len(c.Encoding))
	for _, name := range slices.Sorted(maps.Keys(c.Encoding)) {
		elems = append(elems, gocode.KeyValue{Key: gocode.Quote(name), Value: propertyEncoding(c.Encoding[name], runtimePkg)})
	}
	return gocode.Composite(gocode.Selector(runtimePkg, "Encoding"), elems)
}

// propertyEncoding writes e as the value of a runtime.Encoding, its type left out.
func propertyEncoding(e gomodel.Encoding, runtimePkg string) string {
	if e.Style == "" {
		return gocode.Composite("", []gocode.KeyValue{{Key: "ContentType", Value: gocode.Quote(e.ContentType)}})
	}
	fields := []gocode.KeyValue{{Key: "Style", Value: gocode.Selector(runtimePkg, styleNames[e.Style])}}
	if e.IsExplode {
		fields = append(fields, gocode.KeyValue{Key: "IsExplode", Value: "true"})
	}
	if e.IsReserved {
		fields = append(fields, gocode.KeyValue{Key: "IsReserved", Value: "true"})
	}
	return gocode.Composite("", fields)
}

// FirstBody is the JSON body of a response, else its first one.
func FirstBody(contents []gomodel.Content) (gomodel.Content, bool) {
	for _, c := range contents {
		if runtime.IsJSON(c.MediaType) {
			return c, true
		}
	}
	if len(contents) == 0 {
		return gomodel.Content{}, false
	}
	return contents[0], true
}

// Style is the runtime constant of a parameter's style, the default of its location when it names
// none the runtime knows.
func Style(p *spec.Parameter) string {
	if name, ok := styleNames[p.Style]; ok {
		return name
	}
	return styleNames[defaultStyles[p.In]]
}

// IsJSONParam reports a parameter written as JSON: one with content application/json instead
// of a schema.
func IsJSONParam(p *spec.Parameter) bool {
	return p.Schema == nil && len(p.Contents) > 0 && runtime.IsJSON(p.Contents[0].Name)
}

// PartsOf lists the parts the declarations behind types are in, each once and sorted.
func PartsOf(types []gomodel.Type) []layout.PartID {
	var out []layout.PartID
	for _, t := range types {
		for _, d := range decls(t) {
			if id := layout.PartID(d.Part); !slices.Contains(out, id) {
				out = append(out, id)
			}
		}
	}
	slices.Sort(out)
	return out
}

// StatusOf is the status code a response is answered with: the one its key names, else 500.
func StatusOf(status string) int {
	if code, ok := spec.StatusCode(status); ok {
		return code
	}
	return 500
}

// Doc is head, then the summary and description when isDescribed, then any deprecation note.
func Doc(head string, op *spec.Operation, isDescribed bool) string {
	lines := []string{head}
	if isDescribed {
		summary, description := strings.TrimSpace(op.Summary), strings.TrimSpace(op.Description)
		lines = append(lines, summary)
		if description != summary {
			lines = append(lines, description)
		}
	}
	doc := strings.Join(slices.DeleteFunc(lines, func(line string) bool { return line == "" }), "\n")
	switch {
	case !op.Deprecated:
		return doc
	case doc == "":
		return deprecatedNote
	}
	return doc + "\n\n" + deprecatedNote
}

// decls returns the declarations a type refers to.
func decls(t gomodel.Type) []*gomodel.Decl {
	switch t := t.(type) {
	case gomodel.DeclRef:
		return []*gomodel.Decl{t.Decl}
	case gomodel.Pointer:
		return decls(t.Elem)
	case gomodel.Nullable:
		return decls(t.Elem)
	case gomodel.Slice:
		return decls(t.Elem)
	case gomodel.Map:
		return slices.Concat(decls(t.Key), decls(t.Elem))
	}
	return nil
}
