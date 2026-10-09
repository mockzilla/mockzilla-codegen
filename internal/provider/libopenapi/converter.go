// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

// The libopenapi model turned into the spec IR, one method per OpenAPI object.

package libopenapi

import (
	"cmp"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/pb33f/libopenapi/datamodel/high/base"
	v3 "github.com/pb33f/libopenapi/datamodel/high/v3"
	lowmodel "github.com/pb33f/libopenapi/datamodel/low"
	lowbase "github.com/pb33f/libopenapi/datamodel/low/base"
	"github.com/pb33f/libopenapi/index"
	"github.com/pb33f/libopenapi/orderedmap"
	"github.com/pb33f/libopenapi/utils"
	"go.yaml.in/yaml/v4"

	"github.com/mockzilla/mockzilla-codegen/internal/diag"
	"github.com/mockzilla/mockzilla-codegen/internal/oasdoc"
	"github.com/mockzilla/mockzilla-codegen/internal/provider"
	"github.com/mockzilla/mockzilla-codegen/internal/spec"
)

var lowerMethods = []string{"get", "put", "post", "delete", "options", "head", "patch", "trace", "query"}

var locations = []string{spec.InPath, spec.InQuery, spec.InHeader, spec.InCookie, spec.InQueryString}

// libraryLine is the line libopenapi names in a message: one of the spec as prepared, which a
// pruned or bundled spec does not share with the file.
var libraryLine = regexp.MustCompile(`,? (at )?line \d+, col \d+`)

const (
	styleSimple = "simple"
	styleForm   = "form"
	styleCookie = "cookie"
)

// site is where an operation sits; path is a webhook name or callback expression for those.
type site struct {
	method     string
	path       string
	ptr        string
	isWebhook  bool
	isCallback bool
	shared     []*spec.Parameter
}

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

// buildIssues takes cycles from the index, which lists them all, not from the build error.
func (c *converter) buildIssues(err error, cycles []*index.CircularReferenceResult) {
	for _, e := range flatten(err) {
		if re := (*index.ResolvingError)(nil); errors.As(e, &re) && re.CircularReference != nil {
			continue
		}
		c.diags.Append(diag.Diagnostic{Severity: diag.Warning, Code: diag.CodeBuildIssue, Origin: diag.Origin{File: c.file}, Message: e.Error()})
	}

	for _, cr := range cycles {
		d := diag.Diagnostic{
			Severity: diag.Info,
			Code:     diag.CodeCircularRef,
			Origin:   diag.Origin{File: c.file},
			Message:  "circular reference: " + cr.GenerateJourneyPath(),
		}
		// libopenapi ignores nullable when it calls a loop infinite, so real specs trip it: warn only.
		if cr.IsInfiniteLoop {
			d.Severity, d.Code = diag.Warning, diag.CodeInfiniteCircularRef
			d.Message = "circular reference with every step required: " + cr.GenerateJourneyPath()
		}
		if at := cr.LoopPoint; at != nil {
			d.Pointer = refPointer(at.Definition)
			d.Origin = c.position(d.Pointer, at.Node)
		}
		c.diags.Append(d)
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

func (c *converter) paths(p *v3.Paths) []*spec.Operation {
	if p == nil {
		return nil
	}

	var out []*spec.Operation
	for path, item := range p.PathItems.FromOldest() {
		out = append(out, c.pathItem(item, site{path: path, ptr: "/paths/" + oasdoc.Escape(path)})...)
	}
	return out
}

func (c *converter) webhooks(m *orderedmap.Map[string, *v3.PathItem]) []*spec.Operation {
	var out []*spec.Operation
	for name, item := range m.FromOldest() {
		out = append(out, c.pathItem(item, site{path: name, ptr: "/webhooks/" + oasdoc.Escape(name), isWebhook: true})...)
	}
	return out
}

// pathItem takes at.ptr as the path item's pointer.
func (c *converter) pathItem(item *v3.PathItem, at site) []*spec.Operation {
	itemPtr := at.ptr
	at.shared = c.parameters(item.Parameters, itemPtr+"/parameters", at)

	fixed := []*v3.Operation{item.Get, item.Put, item.Post, item.Delete, item.Options, item.Head, item.Patch, item.Trace, item.Query}
	var out []*spec.Operation
	for i, op := range fixed {
		if op == nil {
			continue
		}
		at.method = spec.MethodOrder[i]
		at.ptr = itemPtr + "/" + lowerMethods[i]
		out = append(out, c.operation(op, at))
	}
	// libopenapi v0.38.7 leaves the high-level AdditionalOperations empty, so read the low model.
	for k, v := range item.GoLow().AdditionalOperations.Value.FromOldest() {
		at.method = k.Value
		at.ptr = itemPtr + "/additionalOperations/" + oasdoc.Escape(k.Value)
		out = append(out, c.operation(v3.NewOperation(v.Value), at))
	}
	return out
}

func (c *converter) operation(o *v3.Operation, at site) *spec.Operation {
	low := o.GoLow()
	out := &spec.Operation{
		ID:          o.OperationId,
		Method:      at.method,
		Path:        at.path,
		IsWebhook:   at.isWebhook,
		Summary:     o.Summary,
		Description: o.Description,
		Deprecated:  o.Deprecated != nil && *o.Deprecated,
		Tags:        slices.Clone(o.Tags),
		Params:      mergeParameters(at.shared, c.parameters(o.Parameters, at.ptr+"/parameters", at)),
		Body:        c.requestBody(o.RequestBody, at.ptr+"/requestBody"),
		Responses:   c.responses(o.Responses, at.ptr+"/responses"),
		Callbacks:   c.callbacks(o.Callbacks, at.ptr+"/callbacks"),
		Security:    c.security,
		Servers:     servers(o.Servers),
		Extensions:  extensions(o.Extensions),
		Origin:      c.origin(at.ptr, low.GetRootNode()),
	}
	if low.Security.KeyNode != nil {
		out.Security = securityRequirements(o.Security)
	}
	if out.ID == "" {
		out.ID, out.IsIDDerived = spec.DeriveOperationID(at.method, at.path), true
	}
	return out
}

// callbacks converts a referenced callback at its target so its schemas are shared.
func (c *converter) callbacks(m *orderedmap.Map[string, *v3.Callback], ptr string) []*spec.Callback {
	var out []*spec.Callback
	for name, cb := range m.FromOldest() {
		cbPtr := ptr + "/" + oasdoc.Escape(name)
		item := &spec.Callback{Name: name}
		if low := cb.GoLow(); low.IsReference() {
			cbPtr = refPointer(low.GetReference())
			item.Ref = componentRef(cbPtr)
		}
		for expr, pi := range cb.Expression.FromOldest() {
			item.Operations = append(item.Operations, c.pathItem(pi, site{path: expr, ptr: cbPtr + "/" + oasdoc.Escape(expr), isCallback: true})...)
		}
		out = append(out, item)
	}
	return out
}

// parameters leaves out a parameter of no name or known location, or a path one the path has no {name} for.
func (c *converter) parameters(list []*v3.Parameter, ptr string, at site) []*spec.Parameter {
	var out []*spec.Parameter
	for i, p := range list {
		usage := ptr + "/" + strconv.Itoa(i)
		param := c.parameter(p, usage)
		switch {
		case param.Name == "" || !slices.Contains(locations, param.In):
			continue
		case param.In == spec.InPath && !at.isWebhook && !at.isCallback && !strings.Contains(at.path, "{"+param.Name+"}"):
			msg := fmt.Sprintf("path parameter %q has no {%s} in %s; it is left out", param.Name, param.Name, at.path)
			c.warn(diag.CodePathParamUnused, usage, p.GoLow().GetRootNode(), msg)
			continue
		}
		out = append(out, param)
	}
	return out
}

func (c *converter) parameter(p *v3.Parameter, ptr string) *spec.Parameter {
	out, ref := memoized(c.paramMemo, p.GoLow(), ptr, func(at string) *spec.Parameter { return c.buildParameter(p, at) })
	if ref != nil {
		out.Ref = ref
	}
	return out
}

// buildParameter fills in the default style and explode, and treats a path parameter as required.
func (c *converter) buildParameter(p *v3.Parameter, ptr string) *spec.Parameter {
	out := &spec.Parameter{
		Name:            p.Name,
		In:              p.In,
		Description:     p.Description,
		Required:        p.Required != nil && *p.Required,
		Deprecated:      p.Deprecated,
		AllowEmptyValue: p.AllowEmptyValue,
		Style:           p.Style,
		AllowReserved:   p.AllowReserved,
		Schema:          c.schema(p.Schema, ptr+"/schema"),
		Contents:        c.contents(p.Content, ptr+"/content"),
		Extensions:      extensions(p.Extensions),
		Origin:          c.origin(ptr, p.GoLow().GetRootNode()),
	}

	switch {
	case out.Name == "":
		c.warn(diag.CodeNameEmpty, ptr, p.GoLow().GetRootNode(), "parameter has no name; it is left out")
	case !slices.Contains(locations, out.In):
		in := "no in"
		if out.In != "" {
			in = "in " + strconv.Quote(out.In) + ", which is not one of " + strings.Join(locations, ", ")
		}
		c.warn(diag.CodeParamIn, ptr, p.GoLow().GetRootNode(), fmt.Sprintf("parameter %q has %s; it is left out", out.Name, in))
	}

	if out.Style == "" {
		switch out.In {
		case spec.InQuery, spec.InCookie:
			out.Style = styleForm
		case spec.InPath, spec.InHeader:
			out.Style = styleSimple
		}
	}
	out.Explode = out.Style == styleForm || out.Style == styleCookie
	if p.Explode != nil {
		out.Explode = *p.Explode
	}

	if out.In == spec.InPath && !out.Required {
		out.Required = true
		c.diags.Append(diag.Diagnostic{
			Severity: diag.Warning,
			Code:     diag.CodeOptionalPathParam,
			Pointer:  ptr,
			Origin:   c.position(ptr, p.GoLow().GetRootNode()),
			Message:  "path parameter " + out.Name + " is not marked required; treating it as required",
		})
	}
	return out
}

func (c *converter) requestBody(b *v3.RequestBody, ptr string) *spec.RequestBody {
	if b == nil {
		return nil
	}

	out, ref := memoized(c.bodyMemo, b.GoLow(), ptr, func(at string) *spec.RequestBody {
		return &spec.RequestBody{
			Description: b.Description,
			Required:    b.Required != nil && *b.Required,
			Contents:    c.contents(b.Content, at+"/content"),
			Extensions:  extensions(b.Extensions),
			Origin:      c.origin(at, b.GoLow().GetRootNode()),
		}
	})
	if ref != nil {
		out.Ref = ref
	}
	return out
}

func (c *converter) contents(m *orderedmap.Map[string, *v3.MediaType], ptr string) []*spec.MediaType {
	var out []*spec.MediaType
	for name, mt := range m.FromOldest() {
		at := ptr + "/" + oasdoc.Escape(name)
		if name == "" {
			c.diags.Append(diag.Diagnostic{
				Severity: diag.Warning,
				Code:     diag.CodeMediaTypeEmpty,
				Pointer:  at,
				Origin:   c.position(at, mt.GoLow().GetRootNode()),
				Message:  "media type key is empty; the content is left out",
			})
			continue
		}
		out = append(out, &spec.MediaType{
			Name:       name,
			Schema:     c.schema(mt.Schema, at+"/schema"),
			ItemSchema: c.schema(mt.ItemSchema, at+"/itemSchema"),
			Encodings:  c.encodings(mt.Encoding, at+"/encoding"),
			Extensions: extensions(mt.Extensions),
			Origin:     c.origin(at, mt.GoLow().GetRootNode()),
		})
	}
	return out
}

func (c *converter) encodings(m *orderedmap.Map[string, *v3.Encoding], ptr string) []*spec.Encoding {
	var out []*spec.Encoding
	for name, e := range m.FromOldest() {
		enc := &spec.Encoding{
			Name:          name,
			ContentType:   e.ContentType,
			Headers:       c.headers(e.Headers, ptr+"/"+oasdoc.Escape(name)+"/headers"),
			Style:         e.Style,
			AllowReserved: e.AllowReserved,
		}
		if e.Explode != nil {
			enc.Explode = new(*e.Explode)
		}
		out = append(out, enc)
	}
	return out
}

func (c *converter) responses(r *v3.Responses, ptr string) []*spec.Response {
	if r == nil {
		return nil
	}

	var out []*spec.Response
	for code, resp := range r.Codes.FromOldest() {
		at := ptr + "/" + oasdoc.Escape(code)
		if statusRank(code) == rankOther {
			c.diags.Append(diag.Diagnostic{
				Severity: diag.Warning,
				Code:     diag.CodeInvalidStatus,
				Pointer:  at,
				Origin:   c.position(at, resp.GoLow().GetRootNode()),
				Message:  "response key " + strconv.Quote(code) + " is not a status code, a range or default; the response is left out",
			})
			continue
		}
		out = append(out, c.response(resp, code, at))
	}
	if r.Default != nil {
		out = append(out, c.response(r.Default, statusDefault, ptr+"/"+statusDefault))
	}

	slices.SortStableFunc(out, func(a, b *spec.Response) int {
		return cmp.Or(cmp.Compare(statusRank(a.Status), statusRank(b.Status)), cmp.Compare(a.Status, b.Status))
	})
	return out
}

func (c *converter) response(r *v3.Response, status, ptr string) *spec.Response {
	out, ref := memoized(c.responseMemo, r.GoLow(), ptr, func(at string) *spec.Response {
		return &spec.Response{
			Description: r.Description,
			Headers:     c.headers(r.Headers, at+"/headers"),
			Contents:    c.contents(r.Content, at+"/content"),
			Extensions:  extensions(r.Extensions),
			Origin:      c.origin(at, r.GoLow().GetRootNode()),
		}
	})
	if ref != nil {
		out.Ref = ref
	}
	out.Status = status
	return out
}

func (c *converter) headers(m *orderedmap.Map[string, *v3.Header], ptr string) []*spec.Header {
	var out []*spec.Header
	for name, h := range m.FromOldest() {
		at := ptr + "/" + oasdoc.Escape(name)
		if name == "" {
			c.warn(diag.CodeNameEmpty, at, h.GoLow().GetRootNode(), "header has no name; it is left out")
			continue
		}
		out = append(out, c.header(h, name, at))
	}
	return out
}

// header names a header by its usage key, which can differ from a referenced component's name.
func (c *converter) header(h *v3.Header, name, ptr string) *spec.Header {
	out, ref := memoized(c.headerMemo, h.GoLow(), ptr, func(at string) *spec.Header {
		style := h.Style
		if style == "" {
			style = styleSimple
		}
		return &spec.Header{
			Description: h.Description,
			Required:    h.Required,
			Deprecated:  h.Deprecated,
			Style:       style,
			Explode:     h.Explode,
			Schema:      c.schema(h.Schema, at+"/schema"),
			Contents:    c.contents(h.Content, at+"/content"),
			Extensions:  extensions(h.Extensions),
			Origin:      c.origin(at, h.GoLow().GetRootNode()),
		}
	})
	if ref != nil {
		out.Ref = ref
	}
	out.Name = name
	return out
}

func (c *converter) schemas(list []*base.SchemaProxy, ptr string) []*spec.Schema {
	var out []*spec.Schema
	for i, p := range list {
		out = append(out, c.schema(p, ptr+"/"+strconv.Itoa(i)))
	}
	return out
}

// schema converts each pointer once, which turns recursive specs into pointer cycles.
func (c *converter) schema(p *base.SchemaProxy, ptr string) *spec.Schema {
	if p == nil {
		return nil
	}
	if s, ok := c.schemaMemo[ptr]; ok {
		return s
	}

	s := &spec.Schema{}
	c.schemaMemo[ptr] = s
	low := p.GoLow()
	switch {
	case low != nil && low.IsTransformedRefWithSiblings():
		c.siblingRef(s, p, ptr)
	case p.IsReference():
		s.Ref = c.ref(p)
		s.Origin = c.origin(ptr, p.GetReferenceNode())
	default:
		c.fill(s, p, ptr, p.GetValueNode())
	}
	return s
}

// siblingRef reads libopenapi's allOf [siblings, $ref] rewrite, which it does in every version.
func (c *converter) siblingRef(s *spec.Schema, p *base.SchemaProxy, ptr string) {
	// Building the wrapper cannot fail: its parts are built lazily.
	view := base.NewSchema(p.GoLow().Schema())
	c.fill(s, view.AllOf[0], ptr, p.GetReferenceNode())
	s.Ref = c.ref(view.AllOf[1])
}

// ref converts a component target from its own entry, anything else from the resolved content.
func (c *converter) ref(p *base.SchemaProxy) *spec.Ref {
	target := refPointer(p.GetReference())
	ref := &spec.Ref{Pointer: target, Name: componentName(target)}
	if ref.Target = c.known(target); ref.Target != nil {
		return ref
	}

	s := &spec.Schema{}
	c.schemaMemo[target] = s
	c.fill(s, p, target, p.GetReferenceNode())
	ref.Target = s
	return ref
}

// known returns the schema at ptr when it is already converted or is a component.
func (c *converter) known(ptr string) *spec.Schema {
	if s, ok := c.schemaMemo[ptr]; ok {
		return s
	}
	if p, ok := c.componentSchemas[ptr]; ok {
		return c.schema(p, ptr)
	}
	return nil
}

// fill leaves s empty, so it reads as any, when p cannot be built, and reports that at n.
func (c *converter) fill(s *spec.Schema, p *base.SchemaProxy, ptr string, n *yaml.Node) {
	h, err := p.BuildSchema()
	if h == nil {
		h = c.rebuild(p, ptr)
	}
	if h == nil {
		s.Origin = c.origin(ptr, n)
		c.diags.Append(diag.Diagnostic{
			Severity: diag.Error,
			Code:     diag.CodeSchemaBuild,
			Pointer:  ptr,
			Origin:   c.position(ptr, n),
			Message:  "schema cannot be built: " + libraryLine.ReplaceAllString(err.Error(), ""),
		})
		return
	}

	k := keywords{c: c, node: h.GoLow().RootNode, ptr: ptr}
	if k.node != nil && k.node.Kind != yaml.MappingNode {
		c.notObject(k.node, ptr)
	}
	// libopenapi leaves these out when they are of the wrong kind; read only reports them.
	k.read("type", "a type name or a list of them", isTypeName)
	k.read("required", "a list of property names", isList)
	k.read("properties", "an object", isObject)
	k.read("enum", "a list", isList)

	s.Types, s.Nullable = c.types(h, k.flag("nullable"), ptr)
	s.Format = h.Format
	s.Title = h.Title
	s.Description = h.Description
	s.Pattern = h.Pattern
	s.ContentEncoding = h.ContentEncoding
	s.ContentMediaType = h.ContentMediaType
	s.Required = slices.Clone(h.Required)
	s.Properties = c.properties(h, ptr)
	s.AdditionalProperties = c.additional(h.AdditionalProperties, ptr+"/additionalProperties")
	s.PropertyNames = c.schema(h.PropertyNames, ptr+"/propertyNames")
	if h.Items != nil && h.Items.IsA() {
		s.Items = c.schema(h.Items.A, ptr+"/items")
	}
	s.PrefixItems = c.schemas(h.PrefixItems, ptr+"/prefixItems")
	s.AllOf = c.schemas(h.AllOf, ptr+"/allOf")
	s.OneOf = c.schemas(h.OneOf, ptr+"/oneOf")
	s.AnyOf = c.schemas(h.AnyOf, ptr+"/anyOf")
	s.Not = c.schema(h.Not, ptr+"/not")
	s.If = c.schema(h.If, ptr+"/if")
	s.Then = c.schema(h.Then, ptr+"/then")
	s.Else = c.schema(h.Else, ptr+"/else")
	s.Discriminator = c.discriminator(h.Discriminator, ptr+"/discriminator")
	s.Enum = values(h.Enum)
	s.Const = optionalValue(h.Const)
	s.Default = optionalValue(h.Default)
	s.Examples = examples(h)
	s.Limits = k.limits()
	s.ReadOnly = k.flag("readOnly")
	s.WriteOnly = k.flag("writeOnly")
	s.Deprecated = k.flag("deprecated")
	s.Extensions = extensions(h.Extensions)
	s.Origin = c.origin(ptr, h.GoLow().RootNode)
	k.unsupported()
}

// rebuild builds the schema of p again without the keywords libopenapi refuses it for. It is nil
// when p has none of them or still cannot be built.
func (c *converter) rebuild(p *base.SchemaProxy, ptr string) *base.Schema {
	lp := p.GoLow()
	ctx, node, idx := lp.GetContext(), lp.GetValueNode(), lp.GetIndex()
	if isRef, _, _ := utils.IsNodeRefValue(node); isRef {
		node, idx, _, ctx = lowmodel.LocateRefNodeWithContext(ctx, node, idx)
	}
	if node == nil {
		return nil
	}

	kept := keywords{c: c, node: utils.NodeAlias(node), ptr: ptr}.buildable()
	ls := &lowbase.Schema{}
	if kept == nil || ls.Build(ctx, kept, idx) != nil {
		return nil
	}
	return base.NewSchema(ls)
}

// notObject reports a schema that is no object, which reads as any; since 3.1 true is a schema.
func (c *converter) notObject(n *yaml.Node, ptr string) {
	v := value(n)
	switch {
	case c.version == spec.V30 || !isBool(v):
		want := "an object"
		if c.version != spec.V30 {
			want = "an object or a boolean"
		}
		c.warn(diag.CodeSchemaInvalid, ptr, n, fmt.Sprintf("a schema must be %s, not %s; it is read as any", want, written(v, n)))
	case !v.Bool:
		c.warn(diag.CodeSchemaInvalid, ptr, n, "schema false allows no value, which no Go type can hold; it is read as any")
	}
}

func (c *converter) properties(h *base.Schema, ptr string) []*spec.Property {
	var out []*spec.Property
	for name, p := range h.Properties.FromOldest() {
		at := ptr + "/properties/" + oasdoc.Escape(name)
		if name == "" {
			c.warn(diag.CodeNameEmpty, at, p.GetValueNode(), `property "" cannot be a Go field; it is left out`)
			continue
		}
		out = append(out, &spec.Property{
			Name:     name,
			Schema:   c.schema(p, at),
			Required: slices.Contains(h.Required, name),
		})
	}
	return out
}

func (c *converter) additional(d *base.DynamicValue[*base.SchemaProxy, bool], ptr string) spec.Additional {
	switch {
	case d == nil:
		return spec.Additional{}
	case d.IsA():
		return spec.Additional{Mode: spec.AdditionalSchema, Schema: c.schema(d.A, ptr)}
	case d.B:
		return spec.Additional{Mode: spec.AdditionalAllowed}
	default:
		return spec.Additional{Mode: spec.AdditionalDenied}
	}
}

// types folds "null" in a type list into Nullable, keeping it only when it is the sole type.
func (c *converter) types(h *base.Schema, isNullable bool, ptr string) (spec.TypeSet, bool) {
	var set spec.TypeSet
	for _, name := range h.Type {
		t := typeOf(name)
		if t == 0 {
			c.diags.Append(diag.Diagnostic{
				Severity: diag.Warning,
				Code:     diag.CodeUnknownType,
				Pointer:  ptr,
				Origin:   c.position(ptr, h.GoLow().RootNode),
				Message:  "unknown type " + strconv.Quote(name) + " is ignored",
			})
		}
		set |= t
	}

	if set.Has(spec.TypeNull) {
		isNullable = true
		if set != spec.TypeNull {
			set &^= spec.TypeNull
		}
	}
	return set, isNullable
}

// discriminator resolves every mapping value; a bare name means a component schema.
func (c *converter) discriminator(d *base.Discriminator, ptr string) *spec.Discriminator {
	if d == nil {
		return nil
	}

	low := d.GoLow()
	out := &spec.Discriminator{Property: d.PropertyName}
	for k, v := range low.Mapping.Value.FromOldest() {
		at := ptr + "/mapping/" + oasdoc.Escape(k.Value)
		out.Mapping = append(out.Mapping, spec.Mapping{Value: k.Value, Ref: c.mappingRef(v.Value, at, v.ValueNode)})
	}
	if d.DefaultMapping != "" {
		out.Default = c.mappingRef(d.DefaultMapping, ptr+"/defaultMapping", low.DefaultMapping.ValueNode)
	}
	return out
}

func (c *converter) mappingRef(target, ptr string, n *yaml.Node) *spec.Ref {
	to := refPointer(target)
	if !strings.ContainsAny(target, "#/") && !isFileName(target) {
		to = componentPointer("schemas", target)
	}

	ref := &spec.Ref{Pointer: to, Name: componentName(to), Target: c.known(to)}
	if ref.Target == nil {
		c.diags.Append(diag.Diagnostic{
			Severity: diag.Warning,
			Code:     diag.CodeUnresolvedMapping,
			Pointer:  ptr,
			Origin:   c.position(ptr, n),
			Message:  "discriminator mapping " + strconv.Quote(target) + " names no schema",
		})
	}
	return ref
}

// origin prefers the caller's pre-transform positions, nearest ancestor first, over parsed nodes.
func (c *converter) origin(ptr string, n *yaml.Node) spec.Origin {
	at := c.position(ptr, n)
	return spec.Origin{Pointer: ptr, File: at.File, Line: at.Line, Col: at.Col}
}

func (c *converter) warn(code, ptr string, n *yaml.Node, msg string) {
	c.diags.Append(diag.Diagnostic{Severity: diag.Warning, Code: code, Pointer: ptr, Origin: c.position(ptr, n), Message: msg})
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
