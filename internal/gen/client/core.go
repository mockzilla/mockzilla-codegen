// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

// The data of core.tmpl: the interface of the client, the client type, its options and the
// default timeout.

package client

import (
	"slices"

	"github.com/mockzilla/mockzilla-codegen/internal/gen/operation"
	"github.com/mockzilla/mockzilla-codegen/internal/gocode"
	"github.com/mockzilla/mockzilla-codegen/internal/gomodel"
)

// CoreView is the data of the core part: the client type, its option type, the interface it
// implements with a signature per operation, the names the packages are imported under, the
// default timeout as a duration expression, empty for none, and whether the client has a Stream
// method. Header is the data of the client.interface-header block.
type CoreView struct {
	Name         string
	Option       string
	Interface    string
	Header       HeaderView
	Context      string
	HTTP         string
	Time         string
	URL          string
	HTTPClient   string
	Timeout      string
	HasStreams   bool
	HasEnvelopes bool
	Signatures   []SignatureView
	User         map[string]any
}

// HeaderView is the data of the client.interface-header block: Name is the client interface.
type HeaderView struct {
	Name string
	User map[string]any
}

// SignatureView is what the interface and the methods of one operation share. Route is the method
// and the path as the spec writes them; Doc is the spec's text of the operation. Result is the
// type of the body the plain method returns, empty for none; Response is the envelope, empty
// without HasEnvelopes; StreamType is what the Stream method returns, empty for no Stream method.
type SignatureView struct {
	Name       string
	Route      string
	Doc        string
	Options    string
	Result     string
	Response   string
	StreamType string
}

func coreView(g *Generator, s *gocode.Scope) *CoreView {
	v := &CoreView{
		Name:         g.opts.Name,
		Option:       g.opts.Namer.ClientOption(g.opts.Name),
		Interface:    g.Interface(),
		Header:       HeaderView{Name: g.Interface(), User: g.opts.User},
		Context:      s.Import(gomodel.Import{Path: "context"}),
		HTTP:         s.Import(gomodel.Import{Path: "net/http"}),
		Time:         s.Import(gomodel.Import{Path: "time"}),
		URL:          s.Import(gomodel.Import{Path: "net/url"}),
		HTTPClient:   s.Import(gomodel.Import{Path: gomodel.HTTPClientPath}),
		HasStreams:   g.opts.HasStreams && slices.ContainsFunc(g.ops, hasStream),
		HasEnvelopes: g.opts.HasEnvelopes,
		User:         g.opts.User,
	}
	if g.opts.Timeout > 0 {
		v.Timeout = gocode.Duration(g.opts.Timeout, v.Time)
	}
	for _, op := range g.ops {
		v.Signatures = append(v.Signatures, signatureView(g, op, s))
	}
	return v
}

// signatureView writes only the types the signatures name, so that the file of the core imports
// nothing it does not use.
func signatureView(g *Generator, op *gomodel.Operation, s *gocode.Scope) SignatureView {
	n := g.opts.Namer
	v := SignatureView{
		Name:    op.Name,
		Route:   op.Spec.Method + " " + op.Spec.Path,
		Doc:     operation.Doc(op.Spec),
		Options: s.Symbol(PartOptions, n.ClientRequestOptions(op.Name)),
	}
	if _, c, ok := SuccessBody(op); ok {
		v.Result = s.Expr(operation.BodyType(c))
	}
	if g.opts.HasEnvelopes {
		v.Response = s.Symbol(PartResponses, n.ClientResponse(op.Name))
	}
	if _, c, ok := streamBody(op); ok && g.opts.HasStreams {
		v.StreamType = streamType(s.Expr(operation.FrameType(c)), s)
	}
	return v
}

func hasStream(op *gomodel.Operation) bool {
	_, _, ok := streamBody(op)
	return ok
}
