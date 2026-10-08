// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

// The data of responses.tmpl: the envelope of each operation, a field per body the client decodes.

package client

import (
	"slices"
	"strconv"
	"strings"

	"github.com/mockzilla/mockzilla-codegen/internal/gen/operation"
	"github.com/mockzilla/mockzilla-codegen/internal/gocode"
	"github.com/mockzilla/mockzilla-codegen/internal/gomodel"
	"github.com/mockzilla/mockzilla-codegen/pkg/runtime"
)

// ResponsesView is the data of the responses part: the envelope of every operation.
type ResponsesView struct {
	HTTP       string
	Operations []EnvelopeView
}

// EnvelopeView is the envelope of one operation: a field per documented body the client decodes
// and per struct of typed headers, after the response and its raw body.
type EnvelopeView struct {
	Name   string
	Type   string
	Fields []FieldView
}

// envelopeField is one field of an envelope with the response it decodes: the status as the spec
// writes it and, for a body, its media type; a headers field has none. The stream field holds the
// stream of the response the Stream method reads, and typ is the type of one frame.
type envelopeField struct {
	name      string
	typ       gomodel.Type
	doc       string
	status    string
	mediaType string
	isHeaders bool
	isStream  bool
}

// view is the field as the envelope's file s declares it, the only file that spells its type.
func (f envelopeField) view(s *gocode.Scope) FieldView {
	t := s.Expr(f.typ)
	if f.isStream {
		t = streamType(t, s)
	}
	return FieldView{Name: f.name, Type: t, Doc: f.doc}
}

func responsesView(g *Generator, s *gocode.Scope) *ResponsesView {
	v := &ResponsesView{}
	if len(g.ops) == 0 {
		return v
	}

	v.HTTP = s.Import(gomodel.Import{Path: "net/http"})
	for _, op := range g.ops {
		e := EnvelopeView{Name: op.Name, Type: g.opts.Namer.ClientResponse(op.Name)}
		for _, f := range envelopeFields(g, op) {
			e.Fields = append(e.Fields, f.view(s))
		}
		v.Operations = append(v.Operations, e)
	}
	return v
}

// envelopeFields lists the fields of an operation's envelope: one per documented body the client
// can decode, named after its media type and status, with HasStreams the stream of the response
// the Stream method reads, then one per struct of typed headers. Two media types with one tag at
// a status are told apart by the type, then by a number.
func envelopeFields(g *Generator, op *gomodel.Operation) []envelopeField {
	n := g.opts.Namer
	var out []envelopeField
	names := []string{"HTTPResponse", "Body", "StatusCode"}
	var stream *envelopeField
	if r, c, ok := streamBody(op); ok && g.opts.HasStreams {
		name := "Stream" + n.Status(r.Status)
		names = append(names, name)
		stream = &envelopeField{
			name:      name,
			typ:       operation.FrameType(c),
			doc:       name + " is the stream of a " + r.Status + " response as " + c.MediaType + ".",
			status:    r.Status,
			mediaType: c.MediaType,
			isStream:  true,
		}
	}

	for _, r := range op.Responses {
		for _, c := range r.Contents {
			if !isDecodable(c) {
				continue
			}
			name := n.MediaTag(c.MediaType) + n.Status(r.Status)
			if slices.Contains(names, name) {
				typ, _, _ := strings.Cut(c.MediaType, "/")
				name = n.Exported(typ) + n.MediaTag(c.MediaType) + n.Status(r.Status)
			}
			base := name
			for i := 2; slices.Contains(names, name); i++ {
				name = base + strconv.Itoa(i)
			}
			names = append(names, name)

			out = append(out, envelopeField{
				name:      name,
				typ:       gomodel.Held(operation.BodyType(c)),
				doc:       name + " is the body of a " + r.Status + " response as " + c.MediaType + ".",
				status:    r.Status,
				mediaType: c.MediaType,
			})
		}
	}
	if stream != nil {
		out = append(out, *stream)
	}
	for _, r := range op.Responses {
		if r.Headers == nil {
			continue
		}
		name := "Headers" + n.Status(r.Status)
		out = append(out, envelopeField{
			name:      name,
			typ:       gomodel.Pointer{Elem: gomodel.DeclRef{Decl: r.Headers}},
			doc:       name + " holds the headers the spec declares for a " + r.Status + " response.",
			status:    r.Status,
			isHeaders: true,
		})
	}
	return out
}

// isDecodable reports JSON, a form or a wildcard into anything, multipart into a struct, text into a value, else a string, bytes or a file.
func isDecodable(c gomodel.Content) bool {
	mediaType := operation.Concrete(c.MediaType)
	t := gomodel.Underlying(gomodel.Elem(operation.BodyType(c)))
	switch {
	case runtime.IsJSON(mediaType), mediaType == "application/x-www-form-urlencoded", strings.Contains(mediaType, "*"):
		return true
	case mediaType == "multipart/form-data" && gomodel.StructDecl(operation.BodyType(c)) != nil:
		return true
	case strings.HasPrefix(mediaType, "text/") && operation.IsTextValue(t):
		return true
	}
	return t == stringType || t == bytesType || t == anyType || t == fileType
}
