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
	"time"

	"github.com/mockzilla/mockzilla-codegen/internal/diag"
	"github.com/mockzilla/mockzilla-codegen/internal/gen/operation"
	"github.com/mockzilla/mockzilla-codegen/internal/gocode"
	"github.com/mockzilla/mockzilla-codegen/internal/gomodel"
	"github.com/mockzilla/mockzilla-codegen/internal/layout"
	"github.com/mockzilla/mockzilla-codegen/internal/naming"
	"github.com/mockzilla/mockzilla-codegen/internal/render"
)

// The client parts.
const (
	PartCore       layout.PartID = "client.core"
	PartOptions    layout.PartID = "client.options"
	PartOperations layout.PartID = "client.operations"
	PartResponses  layout.PartID = "client.responses"
)

//go:embed *.tmpl
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

// New returns the generator of m's operations. Without HasStreams, it warns about every operation
// whose 2xx responses come only in sequential media types, which the plain method reads whole and
// so never returns from while the server keeps sending.
func New(m *gomodel.Model, opts Options) (*Generator, []diag.Diagnostic) {
	g := &Generator{opts: opts}
	var diags []diag.Diagnostic
	for _, op := range m.Operations {
		if op.Spec.IsWebhook {
			continue
		}
		g.ops = append(g.ops, op)
		if opts.HasStreams || !IsStreamOnly(op) {
			continue
		}

		_, c, _ := streamBody(op)
		diags = append(diags, diag.Diagnostic{
			Severity: diag.Warning,
			Code:     diag.CodeStreamOnly,
			Pointer:  op.Spec.Origin.Pointer,
			Origin:   diag.Origin{File: op.Spec.Origin.File, Line: op.Spec.Origin.Line, Col: op.Spec.Origin.Col},
			Message:  op.Name + " answers only as " + c.MediaType + ", which " + op.Name + " reads whole; set client.streaming to read it as it arrives",
		})
	}
	return g, diags
}

// Templates is the client template set. No block can be overridden yet.
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
	}
}

// Parts returns the client parts with the parts each refers to. The operations add methods to
// the client type, so they share the folder of the core.
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

	parts := []layout.Part{
		{ID: PartCore},
		{ID: PartOptions, Uses: operation.PartsOf(requests)},
		{ID: PartOperations, Uses: slices.Concat([]layout.PartID{PartCore, PartOptions}, operation.PartsOf(responses)), Owner: PartCore},
	}
	if g.opts.HasEnvelopes {
		parts[2].Uses = slices.Concat([]layout.PartID{PartCore, PartOptions, PartResponses}, operation.PartsOf(responses))
		parts = append(parts, layout.Part{ID: PartResponses, Uses: operation.PartsOf(responses)})
	}
	return parts
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
