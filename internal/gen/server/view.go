// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

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
	for _, f := range g.fields[op.Name] {
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
	if c, ok := operation.FirstBody(r.Contents); ok {
		v.ContentType = gocode.Quote(c.MediaType)
		v.Body = at.scope.Expr(operation.BodyType(c))
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
