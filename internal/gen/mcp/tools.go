// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

// The data of tools.tmpl: the definition of each tool and its handler, which fills the request
// options from the input and calls the client.

package mcp

import (
	"slices"
	"strings"

	"github.com/mockzilla/mockzilla-codegen/internal/gen/client"
	"github.com/mockzilla/mockzilla-codegen/internal/gen/operation"
	"github.com/mockzilla/mockzilla-codegen/internal/gocode"
	"github.com/mockzilla/mockzilla-codegen/internal/gomodel"
)

var stringType = gomodel.Builtin{Name: "string"}

// mediaPrefixes are the media types a tool answers as image or audio content.
var mediaPrefixes = []string{"image/", "audio/"}

// ToolsView is the data of the tools part. Client is the client interface the tools call, as the
// file writes it; MCP, JSON, Context, Errors, Runtime and Strings are the names the packages are
// imported under. HasStream says whether a tool answers with the streaming error, HasFile whether
// one answers with a file.
type ToolsView struct {
	Client    string
	MCP       string
	JSON      string
	Context   string
	Errors    string
	Runtime   string
	Strings   string
	HasStream bool
	HasFile   bool
	Tools     []ToolView
	User      map[string]any
}

// ToolView is one tool: its definition and its handler. Name is the operation, Tool the tool
// name. IsRounded says the handler decodes the arguments again, past the SDK's float64. Of the
// result, Text is it as text and File as a file; with neither it is structured content.
// HasOtherSuccess says another 2xx leaves it nil, HasNilCheck that the handler checks for nil.
type ToolView struct {
	Name            string
	Tool            string
	Description     string
	Schema          string
	Input           string
	Options         string
	IsReadOnly      bool
	IsIdempotent    bool
	HasInput        bool
	IsRounded       bool
	Groups          []GroupView
	Body            *AssignView
	IsStream        bool
	HasResult       bool
	Text            string
	File            string
	HasOtherSuccess bool
	HasNilCheck     bool
}

// GroupView is the parameters of one location: the options field that holds them, the type of
// that field, and the field of each parameter with the input field it comes from.
type GroupView struct {
	Field   string
	Type    string
	Assigns []AssignView
}

// AssignView fills Field from the input field From.
type AssignView struct {
	Field string
	From  string
}

func toolsView(g *Generator, s *gocode.Scope) *ToolsView {
	v := &ToolsView{
		Client: s.Symbol(client.PartOperations, g.opts.Client+"Interface"),
		MCP:    s.Import(gomodel.Import{Path: SDKPath}),
		User:   g.opts.User,
	}
	if len(g.tools) == 0 {
		return v
	}

	v.JSON = s.Import(gomodel.Import{Path: "encoding/json"})
	v.Context = s.Import(gomodel.Import{Path: "context"})
	for _, t := range g.tools {
		tv := toolView(g, t, s)
		switch {
		case t.isStream:
			v.HasStream = true
			v.Errors = s.Import(gomodel.Import{Path: "errors"})
		default:
			v.Runtime = s.Import(gomodel.Import{Path: gomodel.RuntimePath})
		}
		if tv.File != "" {
			v.HasFile = true
			v.Strings = s.Import(gomodel.Import{Path: "strings"})
		}
		v.Tools = append(v.Tools, tv)
	}
	return v
}

func toolView(g *Generator, t *tool, s *gocode.Scope) ToolView {
	n := g.opts.Namer
	v := ToolView{
		Name:         t.op.Name,
		Tool:         t.name,
		Description:  t.desc,
		Schema:       gocode.RawString(t.schema),
		Input:        s.Symbol(PartInputs, n.ToolInput(t.op.Name)),
		Options:      s.Symbol(client.PartOptions, n.ClientRequestOptions(t.op.Name)),
		IsReadOnly:   slices.Contains(safeMethods, t.op.Spec.Method),
		IsIdempotent: slices.Contains(safeMethods, t.op.Spec.Method) || slices.Contains(idempotentMethods, t.op.Spec.Method),
		IsStream:     t.isStream,
		HasInput:     len(t.params) > 0 || t.body != nil,
		IsRounded:    isRounded(t),
	}
	for _, p := range t.params {
		field := operation.GroupField(p.group.In, n)
		i := slices.IndexFunc(v.Groups, func(gv GroupView) bool { return gv.Field == field })
		if i < 0 {
			i = len(v.Groups)
			v.Groups = append(v.Groups, GroupView{Field: field, Type: s.Expr(gomodel.DeclRef{Decl: p.group.Decl})})
		}
		v.Groups[i].Assigns = append(v.Groups[i].Assigns, AssignView{Field: p.field.Name, From: p.goName})
	}
	if t.body != nil {
		v.Body = &AssignView{Field: t.body.field, From: t.body.goName}
	}

	if r, c, ok := client.SuccessBody(t.op); ok {
		typ := operation.BodyType(c)
		var isTextPointer, isFilePointer bool
		v.HasResult = true
		v.Text, isTextPointer = textResult(typ, s)
		if v.Text == "" {
			v.File, isFilePointer = fileResult(c.MediaType, typ, s)
		}
		v.HasOtherSuccess = slices.ContainsFunc(t.op.Responses, func(x gomodel.Response) bool { return client.IsOtherSuccess(x, r.Status) })
		v.HasNilCheck = isTextPointer || isFilePointer || v.HasOtherSuccess
	}
	return v
}

// isRounded reports whether a field of the input of t holds a number that loses digits when read
// through float64.
func isRounded(t *tool) bool {
	if t.body != nil && !gomodel.FitsFloat64(operation.BodyType(t.body.content)) {
		return true
	}
	return slices.ContainsFunc(t.params, func(p param) bool { return !gomodel.FitsFloat64(p.field.Type) })
}

// textResult is the expression that reads a result of type t as a string, when its underlying
// type is one: dereferenced when the method returns a pointer and converted when it returns a
// defined type. Any other result comes back empty and is returned as it is.
func textResult(t gomodel.Type, s *gocode.Scope) (string, bool) {
	base, isPointer := t, false
	if p, ok := t.(gomodel.Pointer); ok {
		base, isPointer = p.Elem, true
	}
	if gomodel.Underlying(base) != stringType {
		return "", false
	}

	out := "out"
	if isPointer {
		out = gocode.Deref(out)
	}
	if base != stringType {
		out = gocode.Call(s.Expr(stringType), out)
	}
	return out, isPointer
}

// fileResult is the expression of the file a result of type t is, which goes back as image or
// audio content when its media type is one: a file the client read, dereferenced, or bytes under
// the image or audio media type the response documents. Any other result comes back empty.
func fileResult(mediaType string, t gomodel.Type, s *gocode.Scope) (string, bool) {
	if p, ok := t.(gomodel.Pointer); ok && gomodel.Underlying(p.Elem) == fileType {
		return gocode.Deref("out"), true
	}
	isMedia := slices.ContainsFunc(mediaPrefixes, func(prefix string) bool { return strings.HasPrefix(mediaType, prefix) })
	if t != bytesType || !isMedia || strings.Contains(mediaType, "*") {
		return "", false
	}
	newFile := gomodel.Qualified{Import: fileType.Import, Name: "NewFile"}
	return gocode.Call(s.Expr(newFile), "out", gocode.Quote(""), gocode.Quote(mediaType)), false
}
