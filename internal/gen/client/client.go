// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

// Package client turns the operations of the Go model into an HTTP client: the client type with
// its options, the request options of every operation, the methods that call the API, and the
// response envelopes of the HasEnvelopes methods.
package client

import (
	"embed"
	"slices"
	"strings"
	"time"

	"github.com/mockzilla/mockzilla-codegen/internal/diag"
	"github.com/mockzilla/mockzilla-codegen/internal/gen/operation"
	"github.com/mockzilla/mockzilla-codegen/internal/gocode"
	"github.com/mockzilla/mockzilla-codegen/internal/gomodel"
	"github.com/mockzilla/mockzilla-codegen/internal/layout"
	"github.com/mockzilla/mockzilla-codegen/internal/naming"
	"github.com/mockzilla/mockzilla-codegen/internal/render"
	"github.com/mockzilla/mockzilla-codegen/internal/spec"
)

// The client parts.
const (
	PartCore       layout.PartID = "client.core"
	PartOptions    layout.PartID = "client.options"
	PartOperations layout.PartID = "client.operations"
	PartResponses  layout.PartID = "client.responses"
)

// blockInterfaceHeader is the block of the client templates a config may override.
const blockInterfaceHeader = "client.interface-header"

//go:embed templates/*.tmpl
var templates embed.FS

// Options are the settings of the client generator. Name is the client type; Timeout is what the
// default http.Client gives up after; HasEnvelopes adds the envelopes and the WithResponse
// methods; HasStreams adds the Stream methods of the operations that answer with a sequential
// media type; User is the config's user-context.
type Options struct {
	Name         string
	Namer        *naming.Namer
	Timeout      time.Duration
	HasEnvelopes bool
	HasStreams   bool
	User         map[string]any
}

// Generator builds the template data of every client part. It skips webhooks, which are
// incoming.
type Generator struct {
	opts Options
	ops  []*gomodel.Operation
}

// New returns the generator of m's operations, with the warnings of each, see warnings.
func New(m *gomodel.Model, opts Options) (*Generator, []diag.Diagnostic) {
	g := &Generator{opts: opts}
	var diags []diag.Diagnostic
	for _, op := range m.Operations {
		if op.Spec.IsWebhook {
			continue
		}
		g.ops = append(g.ops, op)
		diags = append(diags, warnings(op, opts)...)
	}
	return g, diags
}

// Templates is the client template set.
func Templates() render.Set {
	return render.Set{
		Name: "client",
		FS:   templates,
		Parts: map[layout.PartID]string{
			PartCore:       "core.tmpl",
			PartOptions:    "options.tmpl",
			PartOperations: "operations.tmpl",
			PartResponses:  "responses.tmpl",
		},
		Blocks: Blocks(),
	}
}

// Blocks lists the blocks of the client templates a config may override.
func Blocks() []string {
	return []string{blockInterfaceHeader}
}

// Interface is the name of the interface the client implements.
func (g *Generator) Interface() string {
	return g.opts.Namer.Interface(g.opts.Name)
}

// Parts returns the client parts with the parts each refers to, in the order a file holds them:
// the types the client takes and returns, then the client with its interface, then its methods.
// The operations add methods to the client type, so they share the folder of the core.
func (g *Generator) Parts() []layout.Part {
	var requests, responses []gomodel.Type
	for _, op := range g.ops {
		for _, p := range op.Params {
			requests = append(requests, gomodel.DeclRef{Decl: p.Decl})
		}
		for _, c := range op.Bodies {
			requests = append(requests, c.Type)
		}
		for _, r := range op.Responses {
			for _, c := range r.Contents {
				responses = append(responses, c.Type, c.Item)
			}
			if r.Headers != nil {
				responses = append(responses, gomodel.DeclRef{Decl: r.Headers})
			}
		}
	}

	types := []layout.PartID{PartOptions}
	parts := []layout.Part{{ID: PartOptions, Uses: operation.PartsOf(requests)}}
	if g.opts.HasEnvelopes {
		types = append(types, PartResponses)
		parts = append(parts, layout.Part{ID: PartResponses, Uses: operation.PartsOf(responses)})
	}
	return append(parts,
		layout.Part{ID: PartCore, Uses: slices.Concat(types, operation.PartsOf(responses))},
		layout.Part{
			ID:     PartOperations,
			Uses:   slices.Concat([]layout.PartID{PartCore}, types, operation.PartsOf(responses)),
			Owner:  PartCore,
			Reason: "adds methods to the types of " + string(PartCore),
		},
	)
}

// View returns the data of part, with names written as the file of s spells them.
func (g *Generator) View(part layout.PartID, s *gocode.Scope) any {
	switch part {
	case PartCore:
		return coreView(g, s)
	case PartOptions:
		return optionsView(g, s)
	case PartResponses:
		return responsesView(g, s)
	default:
		return operationsView(g, s)
	}
}

// warnings are what the client cannot do for op: fill a placeholder of its path, write a request
// body, decode a 2xx body, stream a body documented under default alone, and, without HasStreams,
// return from a method whose 2xx responses only stream, which it reads whole while the server
// keeps sending.
func warnings(op *gomodel.Operation, opts Options) []diag.Diagnostic {
	var out []diag.Diagnostic
	if names := unfilled(op); len(names) > 0 {
		out = append(out, warning(op, diag.CodePathParamMissing, "no path parameter fills {"+strings.Join(names, "}, {")+"} in "+op.Spec.Path+", so "+op.Name+" always fails"))
	}
	fields := operation.BodyFields(op.Bodies, opts.Namer)
	for i, c := range op.Bodies {
		if encoderOf(c) == "" {
			out = append(out, warning(op, diag.CodeClientBodyUnwritable, op.Name+" sends "+c.MediaType+", which the client cannot write "+
				gocode.Text(gomodel.Elem(operation.BodyType(c)))+" as, so the call fails when "+fields[i]+" is set"))
		}
	}
	for _, r := range op.Responses {
		if c, ok := unreadBody(r); ok {
			out = append(out, warning(op, diag.CodeClientBodyUnread, op.Name+" answers "+r.Status+" as "+c.MediaType+", which the client cannot decode, so "+op.Name+" returns no body for it"))
		}
	}

	_, c, hasStream := streamBody(op)
	switch {
	case !hasStream:
		if body, ok := defaultStream(op); ok {
			out = append(out, warning(op, diag.CodeStreamUnread, op.Name+" documents "+body.MediaType+" under default alone, which never covers a 2xx, so it has no Stream method; document it under 200 or 2XX"))
		}
	case !opts.HasStreams && IsStreamOnly(op):
		out = append(out, warning(op, diag.CodeStreamOnly, op.Name+" answers only as "+c.MediaType+", which "+op.Name+" reads whole; set client.streaming to read it as it arrives"))
	}
	return out
}

func warning(op *gomodel.Operation, code, message string) diag.Diagnostic {
	return diag.Diagnostic{
		Severity: diag.Warning,
		Code:     code,
		Pointer:  op.Spec.Origin.Pointer,
		Origin:   diag.Origin{File: op.Spec.Origin.File, Line: op.Spec.Origin.Line, Col: op.Spec.Origin.Col},
		Message:  message,
	}
}

// unreadBody is the first body of a 2xx response that the client decodes none of, unless the
// body is sequential, which the Stream method reads.
func unreadBody(r gomodel.Response) (gomodel.Content, bool) {
	status := operation.StatusOf(r.Status)
	if status < 200 || status > 299 || slices.ContainsFunc(r.Contents, isDecodable) {
		return gomodel.Content{}, false
	}
	i := slices.IndexFunc(r.Contents, func(c gomodel.Content) bool { return !isSequential(c) })
	if i < 0 {
		return gomodel.Content{}, false
	}
	return r.Contents[i], true
}

// defaultStream is the first sequential body of the default response of op.
func defaultStream(op *gomodel.Operation) (gomodel.Content, bool) {
	for _, r := range op.Responses {
		if !strings.EqualFold(r.Status, "default") {
			continue
		}
		if i := slices.IndexFunc(r.Contents, isSequential); i >= 0 {
			return r.Contents[i], true
		}
	}
	return gomodel.Content{}, false
}

// unfilled lists, each once, the placeholders in the path of op, its query included, that no path
// parameter fills. The runtime sends no fragment, so a placeholder after a # needs none.
func unfilled(op *gomodel.Operation) []string {
	filled := map[string]bool{}
	for _, group := range op.Params {
		if group.In != spec.InPath {
			continue
		}
		for _, p := range group.Params {
			filled[p.Name] = true
		}
	}

	var names []string
	path, _, _ := strings.Cut(op.Spec.Path, "#")
	for {
		_, rest, ok := strings.Cut(path, "{")
		if !ok {
			return names
		}
		name, after, _ := strings.Cut(rest, "}")
		if !filled[name] && !slices.Contains(names, name) {
			names = append(names, name)
		}
		path = after
	}
}
