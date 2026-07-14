// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package libopenapi

import (
	"slices"
	"strings"

	"github.com/pb33f/libopenapi/datamodel/high/base"
	v3 "github.com/pb33f/libopenapi/datamodel/high/v3"
	"go.yaml.in/yaml/v4"

	"github.com/mockzilla/codegen/internal/diag"
	"github.com/mockzilla/codegen/internal/oasdoc"
	"github.com/mockzilla/codegen/internal/provider"
	"github.com/mockzilla/codegen/internal/spec"
)

// converter memoizes by JSON pointer, so a $ref and its target share IR values and cycles end.
type converter struct {
	version   spec.Version
	file      string
	positions map[string]diag.Origin
	diags     diag.Collector
	security  []spec.SecurityRequirement

	componentSchemas map[string]*base.SchemaProxy
	schemaMemo       map[string]*spec.Schema
	paramMemo        map[string]*spec.Parameter
	bodyMemo         map[string]*spec.RequestBody
	responseMemo     map[string]*spec.Response
	headerMemo       map[string]*spec.Header
}

func newConverter(version spec.Version, opts provider.ParseOptions) *converter {
	return &converter{
		version:          version,
		file:             opts.File,
		positions:        opts.Positions,
		componentSchemas: map[string]*base.SchemaProxy{},
		schemaMemo:       map[string]*spec.Schema{},
		paramMemo:        map[string]*spec.Parameter{},
		bodyMemo:         map[string]*spec.RequestBody{},
		responseMemo:     map[string]*spec.Response{},
		headerMemo:       map[string]*spec.Header{},
	}
}

func (c *converter) document(m *v3.Document) *spec.Document {
	doc := &spec.Document{
		Version:    c.version,
		Servers:    servers(m.Servers),
		Security:   securityRequirements(m.Security),
		Extensions: extensions(m.Extensions),
	}
	if m.Info != nil {
		doc.Info = spec.Info{
			Title:       m.Info.Title,
			Summary:     m.Info.Summary,
			Description: m.Info.Description,
			Version:     m.Info.Version,
			Extensions:  extensions(m.Info.Extensions),
		}
	}

	c.security = doc.Security
	doc.Components = c.components(m.Components)
	doc.Operations = c.paths(m.Paths)
	doc.Webhooks = c.webhooks(m.Webhooks)
	return doc
}

func (c *converter) components(m *v3.Components) spec.Components {
	var out spec.Components
	if m == nil {
		return out
	}

	for name, p := range m.Schemas.FromOldest() {
		c.componentSchemas[componentPointer("schemas", name)] = p
	}
	for name, p := range m.Schemas.FromOldest() {
		out.Schemas = append(out.Schemas, spec.Named[*spec.Schema]{Name: name, Value: c.schema(p, componentPointer("schemas", name))})
	}
	for name, p := range m.Parameters.FromOldest() {
		out.Parameters = append(out.Parameters, spec.Named[*spec.Parameter]{Name: name, Value: c.parameter(p, componentPointer("parameters", name))})
	}
	for name, h := range m.Headers.FromOldest() {
		out.Headers = append(out.Headers, spec.Named[*spec.Header]{Name: name, Value: c.header(h, name, componentPointer("headers", name))})
	}
	for name, b := range m.RequestBodies.FromOldest() {
		out.RequestBodies = append(out.RequestBodies, spec.Named[*spec.RequestBody]{Name: name, Value: c.requestBody(b, componentPointer("requestBodies", name))})
	}
	for name, r := range m.Responses.FromOldest() {
		out.Responses = append(out.Responses, spec.Named[*spec.Response]{Name: name, Value: c.response(r, "", componentPointer("responses", name))})
	}
	for name, s := range m.SecuritySchemes.FromOldest() {
		out.SecuritySchemes = append(out.SecuritySchemes, c.securityScheme(s, name))
	}
	return out
}

func (c *converter) securityScheme(s *v3.SecurityScheme, name string) *spec.SecurityScheme {
	out := &spec.SecurityScheme{
		Name:              name,
		Type:              s.Type,
		Description:       s.Description,
		In:                s.In,
		ParamName:         s.Name,
		Scheme:            s.Scheme,
		BearerFormat:      s.BearerFormat,
		OpenIDConnectURL:  s.OpenIdConnectUrl,
		OAuth2MetadataURL: s.OAuth2MetadataUrl,
		Deprecated:        s.Deprecated,
		Extensions:        extensions(s.Extensions),
		Origin:            c.origin(componentPointer("securitySchemes", name), s.GoLow().GetRootNode()),
	}
	if f := s.Flows; f != nil {
		for _, flow := range []struct {
			kind string
			flow *v3.OAuthFlow
		}{
			{"implicit", f.Implicit},
			{"password", f.Password},
			{"clientCredentials", f.ClientCredentials},
			{"authorizationCode", f.AuthorizationCode},
			{"device", f.Device},
		} {
			if flow.flow != nil {
				out.Flows = append(out.Flows, oauthFlow(flow.kind, flow.flow))
			}
		}
	}
	return out
}

// origin prefers the caller's pre-transform positions, nearest ancestor first, over parsed nodes.
func (c *converter) origin(ptr string, n *yaml.Node) spec.Origin {
	at := c.position(ptr, n)
	return spec.Origin{Pointer: ptr, File: at.File, Line: at.Line, Col: at.Col}
}

func (c *converter) position(ptr string, n *yaml.Node) diag.Origin {
	for p := ptr; c.positions != nil; {
		if at, ok := c.positions[p]; ok {
			return at
		}
		i := strings.LastIndex(p, "/")
		if i < 0 {
			break
		}
		p = p[:i]
	}

	at := diag.Origin{File: c.file}
	if n != nil {
		at.Line, at.Col = n.Line, n.Column
	}
	return at
}

func oauthFlow(kind string, f *v3.OAuthFlow) spec.OAuthFlow {
	out := spec.OAuthFlow{
		Kind:             kind,
		AuthorizationURL: f.AuthorizationUrl,
		TokenURL:         f.TokenUrl,
		RefreshURL:       f.RefreshUrl,
	}
	for name, desc := range f.Scopes.FromOldest() {
		out.Scopes = append(out.Scopes, spec.Scope{Name: name, Description: desc})
	}
	return out
}

func servers(list []*v3.Server) []spec.Server {
	var out []spec.Server
	for _, s := range list {
		srv := spec.Server{URL: s.URL, Name: s.Name, Description: s.Description}
		for name, v := range s.Variables.FromOldest() {
			srv.Variables = append(srv.Variables, spec.ServerVariable{
				Name:        name,
				Default:     v.Default,
				Description: v.Description,
				Enum:        slices.Clone(v.Enum),
			})
		}
		out = append(out, srv)
	}
	return out
}

// securityRequirements keeps an explicit empty list non-nil: it switches security off.
func securityRequirements(list []*base.SecurityRequirement) []spec.SecurityRequirement {
	if list == nil {
		return nil
	}

	out := make([]spec.SecurityRequirement, 0, len(list))
	for _, r := range list {
		var req spec.SecurityRequirement
		for name, scopes := range r.Requirements.FromOldest() {
			req.Schemes = append(req.Schemes, spec.RequiredScheme{Name: name, Scopes: slices.Clone(scopes)})
		}
		out = append(out, req)
	}
	return out
}

func componentPointer(kind, name string) string {
	return "/components/" + kind + "/" + oasdoc.Escape(name)
}
