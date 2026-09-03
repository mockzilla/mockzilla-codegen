// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package server

import (
	"cmp"
	"slices"
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
// packages are imported under; User is the config's user-context.
type ServiceView struct {
	Name       string
	Context    string
	HTTP       string
	Runtime    string
	User       map[string]any
	Operations []OperationView
}

// OperationView is one operation: its method, options type and response data type. Fields are
// the parameter groups, the bodies, then what plugins add.
type OperationView struct {
	Name         string
	Doc          string
	Options      string
	Data         string
	User         map[string]any
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

// SuccessResponse is the first 2xx response of an operation: the status code the handlers answer
// with, the start of a range such as 2XX, and the body its constructor takes, nil without one.
type SuccessResponse struct {
	Status int
	Body   *gomodel.Content
}

// Success returns the first 2xx response of op, if it has one.
func Success(op *gomodel.Operation) (SuccessResponse, bool) {
	for _, r := range op.Responses {
		status := statusOf(r.Status)
		if status < 200 || status > 299 {
			continue
		}

		s := SuccessResponse{Status: status}
		if c, ok := firstBody(r.Contents); ok {
			s.Body = &c
		}
		return s, true
	}
	return SuccessResponse{}, false
}

func serviceView(g *Generator, s *gocode.Scope) *ServiceView {
	v := &ServiceView{Name: g.opts.Name + "Interface", User: g.opts.User}
	if len(g.ops) == 0 {
		return v
	}

	v.Context = s.Import(gomodel.Import{Path: "context"})
	v.HTTP = s.Import(gomodel.Import{Path: "net/http"})
	v.Runtime = s.Import(gomodel.Import{Path: gomodel.RuntimePath})
	for _, op := range g.ops {
		v.Operations = append(v.Operations, operationView(g, op, s))
	}
	return v
}

func operationView(g *Generator, op *gomodel.Operation, s *gocode.Scope) OperationView {
	n := g.opts.Namer
	v := OperationView{
		Name:    op.Name,
		Doc:     operationDoc(op.Spec),
		Options: n.ServiceRequestOptions(op.Name),
		Data:    n.ResponseData(op.Name),
		User:    g.opts.User,
	}

	for _, p := range op.Params {
		field := cmp.Or(optionFields[p.In], n.Exported(p.In))
		v.Fields = append(v.Fields, FieldView{Name: field, Type: s.Expr(gomodel.Pointer{Elem: gomodel.DeclRef{Decl: p.Decl}})})
		if p.Decl.Validation != nil {
			v.Checks = append(v.Checks, CheckView{Field: field, Path: gocode.Quote(p.In)})
		}
	}
	fields := bodyFields(op.Bodies, n)
	for i, c := range op.Bodies {
		t := BodyType(c)
		v.Fields = append(v.Fields, FieldView{Name: fields[i], Type: s.Expr(t), Doc: "Body sent as " + c.MediaType + "."})
		if gomodel.Validates(t) {
			v.Checks = append(v.Checks, CheckView{Field: fields[i], Path: gocode.Quote("body")})
		}
	}
	for _, f := range g.opts.ExtraFields {
		v.Fields = append(v.Fields, FieldView{Name: f.Name, Type: s.Qualified(f.Type, f.Import), Doc: f.Doc})
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
		v.Body = at.scope.Expr(BodyType(c))
		doc += " with body as " + c.MediaType
	}
	v.Doc = doc + "."
	return v
}

// bodyFields names the options field of each body: Body for one, else Body and the tag of its
// media type. Two media types with one tag, such as application/xml and text/xml, are told apart
// by the type, then by a number.
func bodyFields(bodies []gomodel.Content, n *naming.Namer) []string {
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
