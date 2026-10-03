// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

// The data of service.tmpl: the service interface, the request options and response data of each
// operation, and the functions that make the response data.

package server

import (
	"strconv"

	"github.com/mockzilla/mockzilla-codegen/internal/gen/operation"
	"github.com/mockzilla/mockzilla-codegen/internal/gocode"
	"github.com/mockzilla/mockzilla-codegen/internal/gomodel"
	"github.com/mockzilla/mockzilla-codegen/internal/naming"
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

// OperationView is one operation: its method, options type and response data type. Fields are
// the parameter groups, then the bodies.
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

// Constructor names the function that makes the response data of r, a response of op, and says
// whether it takes the status first: the key of r is no number, such as a range or default.
func Constructor(n *naming.Namer, op *gomodel.Operation, r gomodel.Response) (name string, hasStatusArg bool) {
	_, err := strconv.Atoi(r.Status)
	return n.ResponseConstructor(op.Name, r.Status, len(op.Responses) > 1), err != nil
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
		Doc:     operation.Doc(op.Spec),
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
	fields := operation.BodyFields(op.Bodies, n)
	for i, c := range op.Bodies {
		t := operation.BodyType(c)
		v.Fields = append(v.Fields, FieldView{Name: fields[i], Type: s.Expr(t), Doc: "Body sent as " + c.MediaType + "."})
		if gomodel.Validates(t) {
			v.Checks = append(v.Checks, CheckView{Field: fields[i], Path: gocode.Quote("body")})
		}
	}

	for _, r := range op.Responses {
		v.Constructors = append(v.Constructors, constructorView(n, op, r, s))
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
func constructorView(n *naming.Namer, op *gomodel.Operation, r gomodel.Response, s *gocode.Scope) ConstructorView {
	name, hasStatusArg := Constructor(n, op, r)
	v := ConstructorView{Name: name, Status: r.Status, HasStatusArg: hasStatusArg, ContentType: gocode.Quote("")}
	if hasStatusArg {
		v.Status = "status"
	}

	doc := "returns the response data of status " + r.Status
	if c, ok := operation.FirstBody(r.Contents); ok {
		v.ContentType = gocode.Quote(c.MediaType)
		v.Body = s.Expr(operation.BodyType(c))
		doc += " with body as " + c.MediaType
	}
	v.Doc = doc + "."
	return v
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
