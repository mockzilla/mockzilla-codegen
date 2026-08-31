// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

// Package gomodel turns the spec IR into a typed Go model: named declarations, fields, pointers,
// enums, maps and merged allOf schemas. No Go type is text until rendering.
package gomodel

import (
	"cmp"
	"maps"
	"slices"

	"github.com/mockzilla/mockzilla-codegen/internal/diag"
	"github.com/mockzilla/mockzilla-codegen/internal/naming"
	"github.com/mockzilla/mockzilla-codegen/internal/spec"
	"github.com/mockzilla/mockzilla-codegen/pkg/config"
)

// Model is the Go side of a spec. Decls come in walk order: components first, each followed by
// the types inside it, then what operations declare.
type Model struct {
	Decls      []*Decl
	Patterns   []*Pattern
	Operations []*Operation
}

// Operation is an operation or webhook with its Go name and the types it uses.
type Operation struct {
	Name      string
	Spec      *spec.Operation
	Params    []ParamGroup
	Bodies    []Content
	Responses []Response
}

// ParamGroup is the struct that holds the parameters of one location.
type ParamGroup struct {
	In   string
	Decl *Decl
}

// Content is one media type; Type is nil when it has no schema.
type Content struct {
	MediaType string
	Type      Type
}

// Response is one status of an operation. Headers is the struct of its typed headers, nil when it
// declares none or no server is generated.
type Response struct {
	Status   string
	Contents []Content
	Headers  *Decl
}

// readers read the spec once for every step of Build. isServer says whether the server contract
// is generated.
type readers struct {
	flat     *flattener
	unions   *unionReader
	ext      *extReader
	isServer bool
}

// Options are the settings Build uses. Reserved names are declared by the generator elsewhere in
// the package; each operation also declares its name plus every OperationSuffixes entry.
// IsValidated adds Validate methods, ValidateResponse where responses differ. ErrorMapping maps
// error type names to the path of their message. IsServer declares what the service contract
// needs: response header structs and the names of its constructors.
type Options struct {
	IntType           string
	Descriptions      bool
	ExtraTags         []string
	EnumPrefix        bool
	Namer             *naming.Namer
	Reserved          []string
	OperationSuffixes []string
	IsValidated       bool
	ValidateResponse  bool
	IsServer          bool
	ErrorMapping      map[string]string
}

// OptionsFrom reads Options from a config. Blocks left out get their defaults.
func OptionsFrom(cfg *config.Config) Options {
	n := naming.New(cfg.Naming.Initialisms)
	models := cmp.Or(cfg.Models, &config.Models{})
	opts := Options{
		IntType:          cmp.Or(models.IntType, "int"),
		Descriptions:     models.Descriptions == nil || *models.Descriptions,
		ExtraTags:        models.ExtraTags,
		EnumPrefix:       cfg.Naming.EnumPrefix == nil || *cfg.Naming.EnumPrefix,
		Namer:            n,
		IsValidated:      !models.Validation.Skip,
		ValidateResponse: !models.Validation.Skip && models.Validation.Response,
		ErrorMapping:     models.ErrorMapping,
	}
	for _, name := range slices.Sorted(maps.Keys(models.ErrorMapping)) {
		opts.Reserved = append(opts.Reserved, "New"+name)
	}

	if s := cfg.Server; s != nil {
		name := cmp.Or(s.Name, "Service")
		opts.IsServer = true
		opts.Reserved = append(opts.Reserved, name+"Interface", "NewRouter", "ErrorKind", "HandlerError", "ErrorHandler", "DefaultErrorHandler")
		opts.OperationSuffixes = []string{n.ServiceRequestOptions(""), n.ResponseData("")}
	}
	if c := cfg.Client; c != nil {
		name := cmp.Or(c.Name, "Client")
		opts.Reserved = append(opts.Reserved, name, "New"+name, name+"Option", name+"Interface", "HTTPDoer", "RequestEditor", "WithHTTPClient", "WithRequestEditor")
	}
	return opts
}

// Build turns a parsed spec into the Go model. Problems in the spec come back as diagnostics.
func Build(doc *spec.Document, opts Options) (*Model, []diag.Diagnostic) {
	var diags diag.Collector
	ops := resolveOperations(doc, opts.Namer, &diags)

	reserved := slices.Clone(opts.Reserved)
	for _, op := range ops {
		for _, suffix := range opts.OperationSuffixes {
			reserved = append(reserved, op.Name+suffix)
		}
		if opts.IsServer {
			isMultiple := len(op.Spec.Responses) > 1
			for _, r := range op.Spec.Responses {
				reserved = append(reserved, opts.Namer.ResponseConstructor(op.Name, r.Status, isMultiple))
			}
		}
	}

	flat := newFlattener(&diags)
	r := readers{flat: flat, unions: newUnionReader(opts.Namer, flat), ext: newExtReader(opts.Namer, &diags), isServer: opts.IsServer}
	c := newCollector(doc, r, &diags)
	c.run(ops)
	types := resolveTypes(c.pending, reserved, &diags)

	b := newBuilder(opts, r, &diags)
	decls := b.build(c.pending, ops, c.headers)
	resolveConstants(decls, slices.Concat(reserved, types), opts, &diags)

	patterns := newPatternSet(opts.Namer, &diags)
	if opts.IsValidated {
		newValidator(opts, flat, b.decls, patterns).plan(decls)
	}
	planMasks(decls, patterns)
	resolveErrors(decls, opts.ErrorMapping, &diags)
	return &Model{Decls: decls, Patterns: patterns.named(), Operations: ops}, diags.List()
}
