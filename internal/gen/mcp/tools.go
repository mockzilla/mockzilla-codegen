// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

// The data of tools.tmpl: the definition of each tool and its handler, which fills the request
// options from the input and calls the client.

package mcp

import (
	"slices"

	"github.com/mockzilla/mockzilla-codegen/internal/gen/client"
	"github.com/mockzilla/mockzilla-codegen/internal/gen/operation"
	"github.com/mockzilla/mockzilla-codegen/internal/gocode"
	"github.com/mockzilla/mockzilla-codegen/internal/gomodel"
)

var stringType = gomodel.Builtin{Name: "string"}

// ToolsView is the data of the tools part. Client is the client interface the tools call, as the
// file writes it; MCP, JSON, Context, Errors and Runtime are the names the packages are imported
// under. HasStream says whether a tool answers with the streaming error.
type ToolsView struct {
	Client    string
	MCP       string
	JSON      string
	Context   string
	Errors    string
	Runtime   string
	HasStream bool
	Tools     []ToolView
	User      map[string]any
}

// ToolView is one tool: its definition and its handler. Name is the operation; Tool is the tool
// name and Schema the input schema as a Go literal. Input and Options are the input type and the
// request options type as the file writes them; Groups and Body say how the handler fills the
// options from the input, and HasInput whether it reads any. HasResult says whether the client
// method returns a body; Text is the expression of that body as text, empty when it is returned as
// it is, and IsTextPointer says whether Text dereferences a pointer. IsStream marks a tool that
// answers with the streaming error.
type ToolView struct {
	Name          string
	Tool          string
	Description   string
	Schema        string
	Input         string
	Options       string
	IsReadOnly    bool
	IsIdempotent  bool
	HasInput      bool
	Groups        []GroupView
	Body          *AssignView
	IsStream      bool
	HasResult     bool
	Text          string
	IsTextPointer bool
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

	if _, c, ok := client.SuccessBody(t.op); ok {
		v.HasResult = true
		v.Text, v.IsTextPointer = textResult(operation.BodyType(c), s)
	}
	return v
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
