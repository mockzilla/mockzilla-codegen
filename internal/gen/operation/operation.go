// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

// Package operation holds what the server and the client generators share about an operation:
// the fields its parameters and bodies go in, the Go type of a body, the response the plain
// methods answer with, and how the runtime names a parameter's style.
package operation

import (
	"cmp"
	"maps"
	"slices"
	"strconv"
	"strings"

	"github.com/mockzilla/mockzilla-codegen/internal/gomodel"
	"github.com/mockzilla/mockzilla-codegen/internal/naming"
	"github.com/mockzilla/mockzilla-codegen/internal/spec"
	"github.com/mockzilla/mockzilla-codegen/pkg/runtime"
)

const deprecatedNote = "Deprecated: the spec marks it deprecated."

// groupFields name the fields that hold the parameters of each location.
var groupFields = map[string]string{
	spec.InPath:        "PathParams",
	spec.InQuery:       "Query",
	spec.InQueryString: "QueryString",
	spec.InHeader:      "Headers",
	spec.InCookie:      "Cookies",
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
}

// defaultStyles are the styles of parameters that name none, by location.
var defaultStyles = map[string]string{spec.InPath: "simple", spec.InQuery: "form", spec.InHeader: "simple", spec.InCookie: "form"}

// SuccessResponse is the first 2xx response of an operation: the status code the plain methods
// answer with, the start of a range such as 2XX, and its body, nil without one.
type SuccessResponse struct {
	Status int
	Body   *gomodel.Content
}

// Success returns the first 2xx response of op, if it has one.
func Success(op *gomodel.Operation) (SuccessResponse, bool) {
	for _, r := range op.Responses {
		status := StatusOf(r.Status)
		if status < 200 || status > 299 {
			continue
		}

		s := SuccessResponse{Status: status}
		if c, ok := FirstBody(r.Contents); ok {
			s.Body = &c
		}
		return s, true
	}
	return SuccessResponse{}, false
}

// GroupField names the options field that holds the parameters of location in: PathParams,
// Query, QueryString, Headers or Cookies.
func GroupField(in string, n *naming.Namer) string {
	return cmp.Or(groupFields[in], n.Exported(in))
}

// IsGroupField reports a name GroupField gives to one of the locations of the spec.
func IsGroupField(name string) bool {
	return slices.Contains(slices.Collect(maps.Values(groupFields)), name)
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
	case strings.HasPrefix(c.MediaType, "text/"):
		return gomodel.Builtin{Name: "string"}
	}
	return gomodel.Slice{Elem: gomodel.Builtin{Name: "byte"}}
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

// StatusOf is the status code a response is answered with: its own, the start of its range, or
// 500 for default.
func StatusOf(status string) int {
	if code, err := strconv.Atoi(status); err == nil {
		return code
	}
	if code, err := strconv.Atoi(status[:1]); err == nil && len(status) == 3 {
		return code * 100
	}
	return 500
}

// Doc is the comment of an operation's method: its summary and description, and the deprecation
// note when the spec marks it deprecated.
func Doc(op *spec.Operation) string {
	doc := cmp.Or(op.Summary, op.Description)
	if op.Summary != "" && op.Description != "" && op.Summary != op.Description {
		doc += "\n\n" + op.Description
	}
	switch {
	case !op.Deprecated:
		return doc
	case doc == "":
		return deprecatedNote
	}
	return doc + "\n\n" + deprecatedNote
}
