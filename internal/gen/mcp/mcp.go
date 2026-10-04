// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

// Package mcp turns the operations of the Go model into MCP tools that call the generated client:
// the tools type that registers them on a server of the official Go SDK, one tool definition and
// handler per operation, and the input type of every tool.
package mcp

import (
	"cmp"
	"embed"
	"slices"
	"strconv"
	"strings"

	"github.com/mockzilla/mockzilla-codegen/internal/diag"
	"github.com/mockzilla/mockzilla-codegen/internal/extension"
	"github.com/mockzilla/mockzilla-codegen/internal/gen/client"
	"github.com/mockzilla/mockzilla-codegen/internal/gen/operation"
	"github.com/mockzilla/mockzilla-codegen/internal/gocode"
	"github.com/mockzilla/mockzilla-codegen/internal/gomodel"
	"github.com/mockzilla/mockzilla-codegen/internal/layout"
	"github.com/mockzilla/mockzilla-codegen/internal/naming"
	"github.com/mockzilla/mockzilla-codegen/internal/render"
	"github.com/mockzilla/mockzilla-codegen/internal/spec"
)

// The MCP parts.
const (
	PartTools  layout.PartID = "mcp.tools"
	PartInputs layout.PartID = "mcp.inputs"
)

// SDKPath is the import path of the MCP SDK the tools are written for.
const SDKPath = "github.com/modelcontextprotocol/go-sdk/mcp"

// maxToolName is the longest tool name the MCP SDK takes, maxHostToolName the longest some hosts
// take; bodyID is the naming request of the body next to the parameters, which are numbered.
const (
	maxToolName     = 128
	maxHostToolName = 64
	bodyID          = "body"
)

//go:embed *.tmpl
var templates embed.FS

// safeMethods are the HTTP methods that read only; idempotentMethods can be repeated. OpenAPI
// ignores a header parameter named in ignoredHeaders, which the client sets itself.
var (
	safeMethods       = []string{"GET", "HEAD", "OPTIONS", "TRACE", "QUERY"}
	idempotentMethods = []string{"PUT", "DELETE"}
	ignoredHeaders    = []string{"Accept", "Authorization", "Content-Type"}
)

// Options are the settings of the MCP generator. Client is the client type the tools call;
// DefaultSkip leaves every operation out unless x-mcp turns it on; User is the config's
// user-context.
type Options struct {
	Client      string
	Namer       *naming.Namer
	DefaultSkip bool
	User        map[string]any
}

// Generator builds the template data of the MCP parts.
type Generator struct {
	opts  Options
	tools []*tool
}

// tool is one operation exposed as a tool: its name and description, the parameters and the body
// of its input, and the JSON schema of that input. An operation that answers with a stream only is
// marked, since its tool returns an error.
type tool struct {
	op       *gomodel.Operation
	name     string
	desc     string
	params   []param
	body     *body
	schema   string
	isStream bool
}

// param is one parameter of the input: the field of the parameter group it goes to and the
// parameter it is. Name is its property in the input; GoName its field.
type param struct {
	group  gomodel.ParamGroup
	field  *gomodel.Field
	spec   *spec.Parameter
	name   string
	goName string
}

// body is the request body of the input: the content the client sends and the options field it
// goes to. Name is its property; GoName its field.
type body struct {
	content    gomodel.Content
	field      string
	name       string
	goName     string
	isRequired bool
}

// New returns the generator of the tools of m: one per operation the config and x-mcp keep,
// webhooks left out. Tool names that clash are numbered, with a note; an x-mcp name the SDK would
// reject is replaced with a warning, and a name some hosts reject is kept with one. A default that
// does not fit its schema is left out of every input, with one warning.
func New(m *gomodel.Model, opts Options) (*Generator, []diag.Diagnostic) {
	g := &Generator{opts: opts}
	var diags []diag.Diagnostic
	var reqs []naming.Request
	isWarned := map[diag.Diagnostic]bool{}
	for i, op := range m.Operations {
		if op.Spec.IsWebhook {
			continue
		}
		set, extDiags := extension.Parse(op.Spec.Extensions, op.Spec.Origin)
		diags = append(diags, extDiags...)
		if set.MCP.IsSkipped(opts.DefaultSkip) {
			continue
		}

		t, schemaDiags := newTool(op, opts.Namer)
		for _, d := range schemaDiags {
			if !isWarned[d] {
				isWarned[d] = true
				diags = append(diags, d)
			}
		}
		req := naming.Request{ID: op.Spec.Origin.Pointer, Want: t.name, Rank: naming.RankOperation, Order: i, Origin: origin(op.Spec)}
		if set.MCP != nil {
			t.desc = cmp.Or(set.MCP.Description, t.desc)
			if set.MCP.Name != "" {
				req.Want, req.Rank = set.MCP.Name, naming.RankGoName
				if !isToolName(set.MCP.Name) {
					req.Want, req.Rank = t.name, naming.RankOperation
					diags = append(diags, badName(op, set.MCP.Name, t.name))
				}
			}
		}
		g.tools = append(g.tools, t)
		reqs = append(reqs, req)
	}

	res := naming.Resolve(nil, reqs)
	for _, r := range res.Renames {
		diags = append(diags, r.Diagnostic())
	}
	for i, t := range g.tools {
		t.name = res.Names[reqs[i].ID]
		if len(t.name) > maxHostToolName || strings.Contains(t.name, ".") {
			diags = append(diags, hostName(t, reqs[i].Rank == naming.RankGoName))
		}
	}
	return g, diags
}

// Templates is the MCP template set. No block can be overridden yet.
func Templates() render.Set {
	return render.Set{
		Name: "mcp",
		FS:   templates,
		Parts: map[layout.PartID]string{
			PartTools:  "tools.tmpl",
			PartInputs: "inputs.tmpl",
		},
	}
}

// Parts returns the MCP parts with the parts each refers to. The tools build the request options
// of the client from the inputs, so they refer to the client, the inputs and the parameter types.
func (g *Generator) Parts() []layout.Part {
	var params, inputs []gomodel.Type
	for _, t := range g.tools {
		for _, p := range t.params {
			params = append(params, gomodel.DeclRef{Decl: p.group.Decl})
			inputs = append(inputs, p.field.Type)
		}
		if t.body != nil {
			inputs = append(inputs, operation.BodyType(t.body.content))
		}
	}
	return []layout.Part{
		{ID: PartInputs, Uses: operation.PartsOf(inputs)},
		{ID: PartTools, Uses: slices.Concat([]layout.PartID{client.PartOperations, client.PartOptions, PartInputs}, operation.PartsOf(params))},
	}
}

// View returns the data of part, with names written as the file of s spells them.
func (g *Generator) View(part layout.PartID, s *gocode.Scope) any {
	if part == PartInputs {
		return inputsView(g, s)
	}
	return toolsView(g, s)
}

// newTool reads the parameters and the body of op into a tool named after the operation ID in
// snake case, described by its summary and description or else by its method and path, and
// builds the schema of its input. A property or field name taken twice, by parameters of two
// locations, gets the location in front.
func newTool(op *gomodel.Operation, n *naming.Namer) (*tool, []diag.Diagnostic) {
	t := &tool{op: op, name: n.Snake(n.Exported(op.Spec.ID)), desc: description(op.Spec), isStream: client.IsStreamOnly(op)}
	var goReqs, jsonReqs []naming.Request
	for _, p := range op.Params {
		for i, f := range p.Decl.Struct.Fields {
			sp := p.Params[i]
			if isIgnoredHeader(sp) {
				continue
			}
			id := strconv.Itoa(len(t.params))
			t.params = append(t.params, param{group: p, field: f, spec: sp})
			goReqs = append(goReqs, naming.Request{ID: id, Want: f.Name, Fallback: operation.GroupField(p.In, n) + f.Name, Order: len(goReqs)})
			jsonReqs = append(jsonReqs, naming.Request{ID: id, Want: sp.Name, Fallback: sp.In + "_" + sp.Name, Order: len(jsonReqs)})
		}
	}
	if c, field, ok := inputBody(op, n); ok {
		t.body = &body{content: c, field: field, isRequired: op.Spec.Body != nil && op.Spec.Body.Required}
		goReqs = append(goReqs, naming.Request{ID: bodyID, Want: "Body", Fallback: "RequestBody", Order: len(goReqs)})
		jsonReqs = append(jsonReqs, naming.Request{ID: bodyID, Want: "body", Fallback: "request_body", Order: len(jsonReqs)})
	}

	goNames, jsonNames := naming.Resolve(nil, goReqs), naming.Resolve(nil, jsonReqs)
	for i := range t.params {
		id := strconv.Itoa(i)
		t.params[i].goName, t.params[i].name = goNames.Names[id], jsonNames.Names[id]
	}
	if t.body != nil {
		t.body.goName, t.body.name = goNames.Names[bodyID], jsonNames.Names[bodyID]
	}
	var diags []diag.Diagnostic
	t.schema, diags = inputSchema(t)
	return t, diags
}

// isToolName reports a name the MCP SDK takes: letters, digits, _ - and . up to 128 characters.
func isToolName(name string) bool {
	if name == "" || len(name) > maxToolName {
		return false
	}
	return !strings.ContainsFunc(name, func(r rune) bool {
		isLetter := r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z'
		return !isLetter && (r < '0' || r > '9') && r != '_' && r != '-' && r != '.'
	})
}

// description is the operation's summary and description, or its method and path when it has
// neither, with the deprecation note.
func description(op *spec.Operation) string {
	if op.Summary != "" || op.Description != "" {
		return operation.Doc(op)
	}
	named := *op
	named.Summary = op.Method + " " + op.Path
	return operation.Doc(&named)
}

func isIgnoredHeader(p *spec.Parameter) bool {
	return p.In == spec.InHeader && slices.ContainsFunc(ignoredHeaders, func(h string) bool { return strings.EqualFold(h, p.Name) })
}

// hostName warns about the name of t, which the SDK takes and some hosts do not. The pointer is
// x-mcp.name when isExtension says the name comes from there.
func hostName(t *tool, isExtension bool) diag.Diagnostic {
	pointer := t.op.Spec.Origin.Pointer
	if isExtension {
		pointer += "/" + extension.MCPName + "/name"
	}
	return diag.Diagnostic{
		Severity: diag.Warning,
		Code:     diag.CodeMCPToolName,
		Pointer:  pointer,
		Origin:   origin(t.op.Spec),
		Message:  "tool name " + strconv.Quote(t.name) + " may be turned down by hosts that take only letters, digits, _ and - up to 64 characters; " + extension.MCPName + ".name sets another",
	}
}

func badName(op *gomodel.Operation, name, fallback string) diag.Diagnostic {
	return diag.Diagnostic{
		Severity: diag.Warning,
		Code:     diag.CodeMCPToolName,
		Pointer:  op.Spec.Origin.Pointer + "/" + extension.MCPName + "/name",
		Origin:   origin(op.Spec),
		Message:  extension.MCPName + ".name " + strconv.Quote(name) + " is no tool name, which has letters, digits, _ - and . up to 128 characters; the tool is named " + strconv.Quote(fallback),
	}
}

func origin(op *spec.Operation) diag.Origin {
	return diag.Origin{File: op.Origin.File, Line: op.Origin.Line, Col: op.Origin.Col}
}
