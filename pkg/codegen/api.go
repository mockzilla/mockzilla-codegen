// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

// The data the template of an extra file runs on: the generated code as the layout places it.

package codegen

import (
	"fmt"
	"go/token"
	"slices"
	"strings"

	"github.com/mockzilla/mockzilla-codegen/internal/gen/client"
	"github.com/mockzilla/mockzilla-codegen/internal/gen/operation"
	"github.com/mockzilla/mockzilla-codegen/internal/gen/server"
	"github.com/mockzilla/mockzilla-codegen/internal/gocode"
	"github.com/mockzilla/mockzilla-codegen/internal/gomodel"
	"github.com/mockzilla/mockzilla-codegen/internal/layout"
	"github.com/mockzilla/mockzilla-codegen/internal/naming"
)

// API is what the template of an extra file sees of the generated code once names are resolved:
// the package of the default output file, the service interface, which is empty without a
// server, every operation, every declared type and the config's user-context. It only ever gains
// fields.
type API struct {
	Package     string
	Service     TypeRef
	Operations  []Operation
	Types       []TypeRef
	UserContext map[string]any
}

// Operation is one operation or webhook of the spec. ID is its Go name, which the handlers report
// as the operation ID; HasOptions says whether it takes parameters or a body, IsRouted whether the
// router registers it. RequestOptions and ResponseData are the types of the service contract,
// empty without a server; ClientRequestOptions is what the client method takes and ClientResponse
// the envelope its WithResponse method returns, empty without a client or without envelopes.
// Responses come in the order of the generated code: codes, ranges, other keys, then default.
// Success points to the first of them with a Code from 200 to 299, nil without one.
type Operation struct {
	ID                   string
	Method               string
	Path                 string
	Summary              string
	Tags                 []string
	HasOptions           bool
	IsRouted             bool
	RequestOptions       TypeRef
	ResponseData         TypeRef
	ClientRequestOptions TypeRef
	ClientResponse       TypeRef
	Responses            []Response
	Success              *Response
}

// Response is one response of an operation. Status is its key as the spec writes it, such as 200,
// 2XX or default. Code is the status code the generated code reads from that key: the key when it
// is a number, the first digit times 100 when it has three characters and starts with a digit,
// else 0. ContentType and Body are those of its JSON body, else of its first one; Body is empty
// without one. IsRaw is set when the body has no schema: any, a string or bytes. Constructor is the
// function that makes the response data of this status, empty without a server: it takes the
// status first when HasStatusArg is set, which is when the key is no number, then the body when
// there is one.
type Response struct {
	Status       string
	Code         int
	ContentType  string
	Body         TypeRef
	IsRaw        bool
	Constructor  TypeRef
	HasStatusArg bool
}

// TypeRef is a Go type, or a function such as a response constructor. Name is the type as the
// package that declares it writes it, Package and ImportPath are those of the identifier in it.
// With an ImportPath, Name is an identifier, or a pointer, slice, array, map or channel around
// one, such as []Pet, and Package, when set, is the package's name; without one the type needs no
// import, Name is written as it is, such as func() any, and Package is empty. A type the
// generator declares has neither when the output is one package outside a module.
type TypeRef struct {
	Name       string
	Package    string
	ImportPath string
}

// Elem is the type t points to, in the package of t: Pet for *Pet. It is empty when t is no
// pointer.
func (t TypeRef) Elem() TypeRef {
	elem, ok := strings.CutPrefix(t.Name, "*")
	if !ok {
		return TypeRef{}
	}
	return TypeRef{Name: elem, Package: t.Package, ImportPath: t.ImportPath}
}

// check returns an error when Package has no ImportPath or is no name to qualify with, or Name
// cannot be qualified with the package of ImportPath.
func (t TypeRef) check() error {
	switch {
	case t.ImportPath == "" && t.Package != "":
		return fmt.Errorf("%w %q of package %s has no import path", errTypeRef, t.Name, t.Package)
	case t.ImportPath == "":
		return nil
	case t.Package != "" && (t.Package == "_" || !token.IsIdentifier(t.Package)):
		return fmt.Errorf("%w %q of %s: %q is no package name", errTypeRef, t.Name, t.ImportPath, t.Package)
	case !gocode.CanQualify(t.Name):
		return fmt.Errorf("%w %q of %s is no identifier, nor a pointer, slice, array, map or channel around one", errTypeRef, t.Name, t.ImportPath)
	}
	return nil
}

// describe is the code g generates, as its layout places it.
func describe(g *generation) *API {
	out := &API{Package: g.lay.Package, UserContext: g.cfg.UserContext}
	for _, d := range g.m.Decls {
		out.Types = append(out.Types, typeRef(gomodel.DeclRef{Decl: d}, g.lay))
	}

	routed := make(map[string]bool)
	if g.srv != nil {
		if f := g.lay.FileOf(server.PartService); f != nil {
			out.Service = inFile(g.srv.Interface(), f)
		}

		for _, r := range g.srv.Routes() {
			routed[r.Operation] = true
		}
	}
	for _, op := range g.m.Operations {
		out.Operations = append(out.Operations, describeOperation(g.namer, op, g.lay, routed[op.Name]))
	}
	return out
}

// describeOperation leaves empty the types of a part lay does not hold: the config asks for no
// server, no client or no envelopes.
func describeOperation(namer *naming.Namer, op *gomodel.Operation, lay *layout.Layout, isRouted bool) Operation {
	o := Operation{
		ID:         op.Name,
		Method:     op.Spec.Method,
		Path:       op.Spec.Path,
		Summary:    op.Spec.Summary,
		Tags:       op.Spec.Tags,
		HasOptions: len(op.Params)+len(op.Bodies) > 0,
		IsRouted:   isRouted,
		Responses:  describeResponses(namer, op, lay),
	}
	if i := slices.IndexFunc(o.Responses, func(r Response) bool { return r.Code >= 200 && r.Code <= 299 }); i >= 0 {
		o.Success = &o.Responses[i]
	}
	if f := lay.FileOf(server.PartService); f != nil {
		o.RequestOptions = inFile(namer.ServiceRequestOptions(op.Name), f)
		o.ResponseData = inFile(namer.ResponseData(op.Name), f)
	}
	if op.Spec.IsWebhook {
		return o
	}

	if f := lay.FileOf(client.PartOptions); f != nil {
		o.ClientRequestOptions = inFile(namer.ClientRequestOptions(op.Name), f)
	}
	if f := lay.FileOf(client.PartResponses); f != nil {
		o.ClientResponse = inFile(namer.ClientResponse(op.Name), f)
	}
	return o
}

// describeResponses leaves the constructors empty when lay holds no service.
func describeResponses(namer *naming.Namer, op *gomodel.Operation, lay *layout.Layout) []Response {
	service := lay.FileOf(server.PartService)
	out := make([]Response, 0, len(op.Responses))
	for _, r := range op.Responses {
		code, _ := operation.StatusCode(r.Status)
		res := Response{Status: r.Status, Code: code}
		if c, ok := operation.FirstBody(r.Contents); ok {
			res.ContentType = c.MediaType
			res.Body = typeRef(operation.BodyType(c), lay)
			res.IsRaw = c.Type == nil
		}
		if service != nil {
			name, hasStatusArg := server.Constructor(namer, op, r)
			res.Constructor = inFile(name, service)
			res.HasStatusArg = hasStatusArg
		}
		out = append(out, res)
	}
	return out
}

// typeRef describes t: its text as its own package writes it, and the package of the named type
// inside it.
func typeRef(t gomodel.Type, lay *layout.Layout) TypeRef {
	ref := TypeRef{Name: gocode.Text(t)}
	switch leaf := gocode.Leaf(t).(type) {
	case gomodel.DeclRef:
		if f := lay.FileOf(layout.PartID(leaf.Decl.Part)); f != nil {
			ref = inFile(ref.Name, f)
		}
	case gomodel.Qualified:
		ref.Package, ref.ImportPath = gocode.ImportName(leaf.Import.Path, leaf.Import.Alias), leaf.Import.Path
	}
	return ref
}

// inFile is name as f declares it. Without an import path the output is one package, and the
// type needs no import.
func inFile(name string, f *layout.File) TypeRef {
	if f.ImportPath == "" {
		return TypeRef{Name: name}
	}
	return TypeRef{Name: name, Package: f.Package, ImportPath: f.ImportPath}
}
