// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package codegen

import (
	"fmt"
	"go/token"
	"text/template"

	"github.com/mockzilla/mockzilla-codegen/internal/gocode"
)

// Plugin adds generated code next to the built-in parts. Name matches [a-z][a-z0-9]* and names
// the parts: plugin.<name>.<part>. Reserve runs before names are resolved, Contribute after.
type Plugin interface {
	Name() string
	Reserve() Reservations
	Contribute(api *API) (*Contribution, error)
}

// Reservations is what a plugin declares before naming. Idents are the package-level names its
// parts declare, Go identifiers which no model may take; RequestOptionFields are added to the
// request options of every operation, before RawRequest.
type Reservations struct {
	Idents              []string
	RequestOptionFields []FieldSpec
}

// FieldSpec is one field a plugin adds. Name is an exported identifier; Type has an ImportPath
// when it has a Package; Doc is its comment, empty for none.
type FieldSpec struct {
	Name string
	Type TypeRef
	Doc  string
}

// Contribution is what a plugin generates. Parts are placed through output.files as
// plugin.<name>.<part>; Scaffolds replace the templates of the scaffold files the config writes;
// Funcs are available to this plugin's templates, next to the generator's, and replace those of
// the same name.
type Contribution struct {
	Parts     []PartSource
	Scaffolds map[ScaffoldKind]string
	Funcs     template.FuncMap
}

// PartSource is one part: its template, the data the template runs on and the imports its code
// needs. Name matches [a-z][a-z0-9]*. The template may also call expr, which writes a TypeRef as
// the file spells it, and import, which imports a path and returns the name to qualify with.
// Imports is for the packages the code does not name, those under _ or .: only import tells the
// name a package got in the file.
type PartSource struct {
	Name     string
	Template string
	Data     any
	Imports  []Import
}

// Import is a package a generated file imports. Alias is empty, _, . or the identifier to import
// it under, which gets a number when another import of the file holds that name.
type Import struct {
	Path  string
	Alias string
}

// check returns an error for an import without a path, or under an alias Go does not take.
func (imp Import) check() error {
	switch {
	case imp.Path == "":
		return fmt.Errorf("%w without a path", errImport)
	case imp.Alias != "" && imp.Alias != "." && !token.IsIdentifier(imp.Alias):
		return fmt.Errorf("%w of %s: the alias %q is not _, . or an identifier", errImport, imp.Path, imp.Alias)
	}
	return nil
}

// ScaffoldKind names a scaffold file.
type ScaffoldKind int

const (
	ScaffoldService ScaffoldKind = iota
	ScaffoldMiddleware
	ScaffoldMain
)

func (k ScaffoldKind) String() string {
	switch k {
	case ScaffoldService:
		return "service"
	case ScaffoldMiddleware:
		return "middleware"
	case ScaffoldMain:
		return "main"
	default:
		return "unknown"
	}
}

// API is what a plugin sees of the generated code once names are resolved: the package of the
// default output file, every operation, every declared type and the config's user-context. It
// only ever gains fields. Each plugin gets a copy of its own, so a change to it reaches nothing
// else.
type API struct {
	Package     string
	Operations  []Operation
	Types       []TypeRef
	UserContext map[string]any
}

// Operation is one operation or webhook of the spec. ID is its Go name, which the handlers report
// as the operation ID; HasOptions says whether it takes parameters or a body, IsRouted whether the
// router registers it. RequestOptions and ResponseData are the types of the service contract,
// empty without a server; ClientRequestOptions is what the client method takes and ClientResponse
// the envelope its WithResponse method returns, empty without a client or without envelopes;
// Success is the first 2xx response, nil without one.
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
	Success              *Success
}

// Success is a 2xx response. Status is its code, or the start of a range such as 2XX. ContentType
// and Body are those of its JSON body, else of its first one; Body is empty without one. IsRaw is
// set when the body has no schema: any, a string or bytes.
type Success struct {
	Status      int
	ContentType string
	Body        TypeRef
	IsRaw       bool
}

// TypeRef is a Go type. Name is the type as the package that declares it writes it, Package and
// ImportPath are those of the identifier in it. With an ImportPath, Name is an identifier, or a
// pointer, slice, array, map or channel around one, such as []Pet, and Package, when set, is the
// package's name; without one the type needs no import and Name is written as it is, such as
// func() any.
type TypeRef struct {
	Name       string
	Package    string
	ImportPath string
}

// Expr writes t as the package with import path from spells it: qualified with Package unless t
// is declared there or needs no import.
func (t TypeRef) Expr(from string) string {
	if t.ImportPath == "" || t.ImportPath == from {
		return t.Name
	}
	return gocode.Qualify(t.Name, t.Package)
}

// check returns an error when Name cannot be qualified with the package of ImportPath, or
// Package is no name to qualify with.
func (t TypeRef) check() error {
	switch {
	case t.ImportPath == "":
		return nil
	case t.Package != "" && (t.Package == "_" || !token.IsIdentifier(t.Package)):
		return fmt.Errorf("%w %q of %s: %q is no package name", errTypeRef, t.Name, t.ImportPath, t.Package)
	case !gocode.CanQualify(t.Name):
		return fmt.Errorf("%w %q of %s is no identifier, nor a pointer, slice, array, map or channel around one", errTypeRef, t.Name, t.ImportPath)
	}
	return nil
}
