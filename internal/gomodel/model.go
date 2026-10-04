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

// serverNames are what the server parts declare next to the models.
var serverNames = []string{
	"NewRouter", "HTTPAdapter", "NewHTTPAdapter", "ServerOption", "ServerOptions", "NewServerOptions",
	"WithRouter", "WithMiddleware", "WithErrorHandler", "WithJSONDecoder", "WithMultipartMaxMemory",
	"ErrorKind", "HandlerError", "ErrorHandler", "ErrorHandlerFunc", "DefaultErrorHandler",
	"ErrorParse", "ErrorDecode", "ErrorValidation", "ErrorService", "ErrorResponse",
}

// mcpNames are what the MCP parts declare next to the models; mcpMethods are the methods of the
// tools type that are no operation.
var (
	mcpNames   = []string{"MCPTools", "NewMCPTools", "ErrMCPStreaming"}
	mcpMethods = []string{"Register"}
)

// middlewareNames are what the middleware scaffold declares.
var middlewareNames = []string{"RequestIDMiddleware", "RecoverMiddleware", "LoggingMiddleware", "CORSMiddleware", "TimeoutMiddleware"}

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

// ParamGroup is the struct that holds the parameters of one location; Params are the parameters
// in the order of the struct's fields.
type ParamGroup struct {
	In     string
	Decl   *Decl
	Params []*spec.Parameter
}

// Content is one media type; Type is nil when it has no schema. Item is the type of one frame of
// a sequential media type such as text/event-stream: its itemSchema, else its schema, which
// describes one event in specs before 3.2; nil for a media type with neither.
type Content struct {
	MediaType string
	Type      Type
	Item      Type
}

// Response is one status of an operation. Headers is the struct of its typed headers, nil when it
// declares none or no server is generated.
type Response struct {
	Status   string
	Contents []Content
	Headers  *Decl
}

// readers read the spec once for every step of Build. hasHeaders says whether responses get
// structs of their typed headers.
type readers struct {
	flat       *flattener
	unions     *unionReader
	ext        *extReader
	hasHeaders bool
}

// Options are the settings Build uses. Reserved names are declared by the generator elsewhere in
// the package; each operation also declares its name plus every OperationSuffixes entry, and no
// operation takes a ReservedOperations name, which the generator declares as a method next to
// the operations, or the name of one of the Methods of another operation.
// IsValidated adds Validate methods, ValidateResponse where responses differ. ErrorMapping maps
// error type names to the path of their message. IsServer reserves the names of the response
// constructors of the service contract; HasResponseHeaders declares a struct of the typed headers
// of every response that has some, which the server and the client envelopes use. Imports are
// the packages an x-go-type may name without an x-go-type-import.
type Options struct {
	IntType            string
	Descriptions       bool
	ExtraTags          []string
	EnumPrefix         bool
	Namer              *naming.Namer
	Reserved           []string
	ReservedOperations []string
	Methods            Methods
	OperationSuffixes  []string
	IsValidated        bool
	ValidateResponse   bool
	IsServer           bool
	HasResponseHeaders bool
	ErrorMapping       map[string]string
	Imports            []config.Import
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
		ValidateResponse: !models.Validation.Skip && (models.Validation.Response || cfg.Server != nil && cfg.Server.Validation.Response),
		ErrorMapping:     models.ErrorMapping,
		Imports:          cfg.Imports,
	}
	for _, name := range slices.Sorted(maps.Keys(models.ErrorMapping)) {
		opts.Reserved = append(opts.Reserved, "New"+name)
	}

	if s := cfg.Server; s != nil {
		name := cmp.Or(s.Name, "Service")
		opts.IsServer, opts.HasResponseHeaders = true, true
		opts.Reserved = append(opts.Reserved, n.Interface(name))
		opts.Reserved = append(opts.Reserved, serverNames...)
		if s.Scaffold.Service != "" {
			opts.Reserved = append(opts.Reserved, name, "New"+name, "ErrNotImplemented")
		}
		if s.Scaffold.Middleware != "" {
			opts.Reserved = append(opts.Reserved, middlewareNames...)
		}
		opts.OperationSuffixes = []string{n.ServiceRequestOptions(""), n.ResponseData("")}
	}
	if c := cfg.Client; c != nil {
		name := cmp.Or(c.Name, "Client")
		opts.Reserved = append(opts.Reserved, name, "New"+name, n.ClientOption(name), n.Interface(name), "HTTPDoer", "RequestEditor", "WithHTTPClient", "WithTimeout", "WithRequestEditor")
		opts.OperationSuffixes = append(opts.OperationSuffixes, n.ClientRequestOptions(""))
		opts.Methods.Client = []string{"Request"}
		stream := []string{"Stream"}
		if c.WithResponse {
			opts.HasResponseHeaders = true
			opts.OperationSuffixes = append(opts.OperationSuffixes, n.ClientResponse(""))
			opts.Methods.Client = append(opts.Methods.Client, "WithResponse")
			stream = append(stream, "StreamWithResponse")
		}

		if c.Streaming {
			opts.Methods.Stream = stream
		}
	}
	if m := cfg.MCP; m != nil {
		opts.Reserved = append(opts.Reserved, mcpNames...)
		opts.ReservedOperations = mcpMethods
		opts.OperationSuffixes = append(opts.OperationSuffixes, n.ToolInput(""))
		opts.Methods.Tool, opts.Methods.IsToolSkipped = []string{"Tool"}, m.DefaultSkip
	}
	return opts
}

// Build turns a parsed spec into the Go model. Problems in the spec come back as diagnostics.
func Build(doc *spec.Document, opts Options) (*Model, []diag.Diagnostic) {
	var diags diag.Collector
	ops := resolveOperations(doc, opts, &diags)

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
	r := readers{flat: flat, unions: newUnionReader(opts.Namer, flat), ext: newExtReader(opts.Namer, &diags), hasHeaders: opts.HasResponseHeaders}
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
