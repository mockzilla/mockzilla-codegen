// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package server

import (
	"cmp"
	"strconv"
	"strings"

	"github.com/mockzilla/mockzilla-codegen/internal/gocode"
	"github.com/mockzilla/mockzilla-codegen/internal/gomodel"
	"github.com/mockzilla/mockzilla-codegen/internal/naming"
	"github.com/mockzilla/mockzilla-codegen/internal/spec"
	"github.com/mockzilla/mockzilla-codegen/pkg/runtime"
)

const deprecatedNote = "Deprecated: the spec marks it deprecated."

// optionFields name the fields that hold the parameters of each location.
var optionFields = map[string]string{
	spec.InPath:        "PathParams",
	spec.InQuery:       "Query",
	spec.InQueryString: "QueryString",
	spec.InHeader:      "Headers",
	spec.InCookie:      "Cookies",
}

// ServiceView is the data of the service part. Context, HTTP and Runtime are the names the
// packages are imported under.
type ServiceView struct {
	Name       string
	Context    string
	HTTP       string
	Runtime    string
	Operations []OperationView
}

// OperationView is one operation: its method, options type and response data type.
type OperationView struct {
	Name         string
	Doc          string
	Options      string
	Data         string
	Fields       []FieldView
	Checks       []CheckView
	Constructors []ConstructorView
	Headers      []HeadersView
}

// FieldView is one field of the options: a parameter group or a body.
type FieldView struct {
	Name string
	Type string
	Doc  string
}

// CheckView validates the Field of the options under Path, a quoted name, when it is set.
type CheckView struct {
	Field string
	Path  string
}

// ConstructorView makes the response data of one status. Status is the status literal, or the
// name of the status argument when HasStatusArg; Body is the type of the body argument, empty for
// none; ContentType is quoted.
type ConstructorView struct {
	Name         string
	Doc          string
	Status       string
	HasStatusArg bool
	Body         string
	ContentType  string
}

// HeadersView is a method that sets the typed headers of one status.
type HeadersView struct {
	Method string
	Type   string
	Doc    string
}

// constructorAt is what the constructors of an operation share.
type constructorAt struct {
	namer     *naming.Namer
	isSeveral bool
	scope     *gocode.Scope
}

func serviceView(g *Generator, s *gocode.Scope) *ServiceView {
	v := &ServiceView{
		Name:    g.name + "Interface",
		Context: s.Import(gomodel.Import{Path: "context"}),
		HTTP:    s.Import(gomodel.Import{Path: "net/http"}),
		Runtime: s.Import(gomodel.Import{Path: gomodel.RuntimePath}),
	}
	for _, op := range g.ops {
		v.Operations = append(v.Operations, operationView(op, g.namer, s))
	}
	return v
}

func operationView(op *gomodel.Operation, n *naming.Namer, s *gocode.Scope) OperationView {
	v := OperationView{
		Name:    op.Name,
		Doc:     operationDoc(op.Spec),
		Options: n.ServiceRequestOptions(op.Name),
		Data:    n.ResponseData(op.Name),
	}

	for _, p := range op.Params {
		field := cmp.Or(optionFields[p.In], n.Exported(p.In))
		v.Fields = append(v.Fields, FieldView{Name: field, Type: s.Expr(gomodel.Pointer{Elem: gomodel.DeclRef{Decl: p.Decl}})})
		if p.Decl.Validation != nil {
			v.Checks = append(v.Checks, CheckView{Field: field, Path: gocode.Quote(p.In)})
		}
	}
	isMultiple := len(op.Bodies) > 1
	for _, c := range op.Bodies {
		field := "Body"
		if isMultiple {
			field += n.MediaTag(c.MediaType)
		}
		t := bodyType(c)
		v.Fields = append(v.Fields, FieldView{Name: field, Type: s.Expr(t), Doc: "Body sent as " + c.MediaType + "."})
		if gomodel.Validates(t) {
			v.Checks = append(v.Checks, CheckView{Field: field, Path: gocode.Quote("body")})
		}
	}

	isSeveral := len(op.Responses) > 1
	for _, r := range op.Responses {
		v.Constructors = append(v.Constructors, constructorView(op, r, constructorAt{namer: n, isSeveral: isSeveral, scope: s}))
		if r.Headers != nil {
			v.Headers = append(v.Headers, HeadersView{
				Method: "WithTypedHeaders" + headerSuffix(n, r.Status, op),
				Type:   s.Expr(gomodel.DeclRef{Decl: r.Headers}),
				Doc:    "adds the headers the spec declares for status " + r.Status + ".",
			})
		}
	}
	return v
}

// constructorView makes the response data for the first body of r, a JSON one when there is one.
func constructorView(op *gomodel.Operation, r gomodel.Response, at constructorAt) ConstructorView {
	v := ConstructorView{Name: at.namer.ResponseConstructor(op.Name, r.Status, at.isSeveral), Status: r.Status, ContentType: gocode.Quote("")}
	if _, err := strconv.Atoi(r.Status); err != nil {
		v.Status, v.HasStatusArg = "status", true
	}

	doc := "returns the response data of status " + r.Status
	if c, ok := firstBody(r.Contents); ok {
		v.ContentType = gocode.Quote(c.MediaType)
		v.Body = at.scope.Expr(bodyType(c))
		doc += " with body as " + c.MediaType
	}
	v.Doc = doc + "."
	return v
}

// bodyType is the Go type a body field holds: the schema's type, held by pointer unless it can
// be nil, or any for JSON without a schema, a string for text and bytes for anything else.
func bodyType(c gomodel.Content) gomodel.Type {
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

// firstBody is the JSON body of a response, else its first one.
func firstBody(contents []gomodel.Content) (gomodel.Content, bool) {
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

// headerSuffix tells the typed header methods apart when several statuses declare headers.
func headerSuffix(n *naming.Namer, status string, op *gomodel.Operation) string {
	count := 0
	for _, r := range op.Responses {
		if r.Headers != nil {
			count++
		}
	}
	if count > 1 {
		return n.Status(status)
	}
	return ""
}

func operationDoc(op *spec.Operation) string {
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
