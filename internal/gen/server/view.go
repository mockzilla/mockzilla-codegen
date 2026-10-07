// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

// The data of service.tmpl: the service interface, the request options and response data of each
// operation, and the functions that make the response data.

package server

import (
	"slices"
	"strconv"
	"strings"

	"github.com/mockzilla/mockzilla-codegen/internal/gen/operation"
	"github.com/mockzilla/mockzilla-codegen/internal/gocode"
	"github.com/mockzilla/mockzilla-codegen/internal/gomodel"
	"github.com/mockzilla/mockzilla-codegen/internal/naming"
	"github.com/mockzilla/mockzilla-codegen/internal/spec"
	"github.com/mockzilla/mockzilla-codegen/pkg/runtime"
)

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

// OperationView is one operation: its method, options type and response data type. Method and
// Path are as the spec writes them. Fields are the parameter groups, then the bodies; Validation
// is set when there are checks.
type OperationView struct {
	Name         string
	Method       string
	Path         string
	Doc          string
	Options      string
	Data         string
	Validation   string
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

// ConstructorView is one response constructor; Status is a literal or the status argument.
type ConstructorView struct {
	Name         string
	Doc          string
	Status       string
	HasStatusArg bool
	Arg          string
	Body         string
	ContentType  string
}

// Constructor is one function that makes the response data of a status.
type Constructor struct {
	Name         string
	HasStatusArg bool
	Body         gomodel.Content
	HasBody      bool
	IsStream     bool
}

// HeadersView is a method that sets the typed headers of one status.
type HeadersView struct {
	Method string
	Type   string
	Doc    string
}

// Constructors lists the constructors of r: one for its body read whole, one for its frames.
func Constructors(n *naming.Namer, op *gomodel.Operation, r gomodel.Response) []Constructor {
	name := n.ResponseConstructor(op.Name, r.Status, len(op.Responses) > 1)
	_, err := strconv.Atoi(r.Status)
	hasStatusArg := err != nil

	var out []Constructor
	whole := slices.DeleteFunc(slices.Clone(r.Contents), isSequential)
	if c, ok := operation.FirstBody(whole); ok || len(whole) == len(r.Contents) {
		out = append(out, Constructor{Name: name, HasStatusArg: hasStatusArg, Body: c, HasBody: ok})
	}
	if i := slices.IndexFunc(r.Contents, isSequential); i >= 0 {
		if len(out) > 0 {
			name += "Stream"
		}
		out = append(out, Constructor{Name: name, HasStatusArg: hasStatusArg, Body: r.Contents[i], HasBody: true, IsStream: true})
	}
	return out
}

func serviceView(g *Generator, s *gocode.Scope) *ServiceView {
	v := &ServiceView{Name: g.Interface(), User: g.opts.User}
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
		Method:  op.Spec.Method,
		Path:    op.Spec.Path,
		Doc:     operation.Doc(op.Name+" handles "+op.Spec.Method+" "+op.Spec.Path+".", op.Spec, g.opts.Descriptions),
		Options: n.ServiceRequestOptions(op.Name),
		Data:    n.ResponseData(op.Name),
		User:    g.opts.User,
	}

	for _, p := range op.Params {
		field := operation.GroupField(p.In, n)
		v.Fields = append(v.Fields, FieldView{Name: field, Type: s.Expr(gomodel.Pointer{Elem: gomodel.DeclRef{Decl: p.Decl}})})
		if p.Decl.Validation != nil {
			v.Checks = append(v.Checks, CheckView{Field: field, Path: gocode.Quote(p.In)})
		}
	}
	if qs := op.QueryString; qs != nil {
		field, t := operation.QueryStringField(op, n), operation.QueryStringType(qs)
		v.Fields = append(v.Fields, FieldView{Name: field, Type: s.Expr(t), Doc: "Query sent as " + qs.Content.MediaType + "."})
		if gomodel.Validates(t) {
			v.Checks = append(v.Checks, CheckView{Field: field, Path: gocode.Quote(spec.InQueryString)})
		}
	}
	fields := operation.BodyFields(op.Bodies, n)
	for i, c := range op.Bodies {
		t := operation.BodyType(c)
		v.Fields = append(v.Fields, FieldView{Name: fields[i], Type: s.Expr(t), Doc: "Body sent as " + c.MediaType + "."})
		if gomodel.Validates(t) {
			v.Checks = append(v.Checks, CheckView{Field: fields[i], Path: gocode.Quote("body")})
		}
	}

	if len(v.Checks) > 0 {
		v.Validation = s.Import(gomodel.Import{Path: gomodel.ValidationPath})
	}

	for _, r := range op.Responses {
		for _, c := range Constructors(n, op, r) {
			v.Constructors = append(v.Constructors, constructorView(c, r.Status, s))
		}
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

func constructorView(c Constructor, status string, s *gocode.Scope) ConstructorView {
	v := ConstructorView{Name: c.Name, Status: status, HasStatusArg: c.HasStatusArg, ContentType: gocode.Quote("")}
	if c.HasStatusArg {
		v.Status = "status"
	}

	doc := "returns the " + status + " response"
	switch {
	case c.IsStream:
		v.ContentType, v.Arg = gocode.Quote(c.Body.MediaType), "frames"
		v.Body = gocode.Index(gocode.Selector(s.Import(gomodel.Import{Path: "iter"}), "Seq"), s.Expr(operation.FrameType(c.Body)))
		doc += " that streams frames as " + c.Body.MediaType
	case c.HasBody:
		v.Arg, v.Body = "body", s.Expr(operation.BodyType(c.Body))
		// A wildcard is no media type to send; the runtime picks one by the Go type.
		if !strings.Contains(c.Body.MediaType, "*") {
			v.ContentType = gocode.Quote(c.Body.MediaType)
		}
		doc += " with its " + c.Body.MediaType + " body"
	}
	v.Doc = doc + "."
	return v
}

func isSequential(c gomodel.Content) bool {
	return runtime.IsSequential(c.MediaType)
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
