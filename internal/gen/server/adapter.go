// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package server

import (
	"slices"
	"strconv"
	"strings"

	"github.com/mockzilla/mockzilla-codegen/internal/gocode"
	"github.com/mockzilla/mockzilla-codegen/internal/gomodel"
	"github.com/mockzilla/mockzilla-codegen/internal/spec"
	"github.com/mockzilla/mockzilla-codegen/pkg/runtime"
)

// Body kinds, by the decoder a body goes through.
const (
	bodyJSON      = "json"
	bodyForm      = "form"
	bodyMultipart = "multipart"
	bodyText      = "text"
	bodyBytes     = "bytes"
	bodyNone      = "none"
)

var (
	stringType = gomodel.Builtin{Name: "string"}
	bytesType  = gomodel.Slice{Elem: gomodel.Builtin{Name: "byte"}}
)

// handlerLocals are the variables a generated handler declares; a typed error variable never
// takes one of them.
var handlerLocals = []string{"a", "w", "r", "opts", "query", "res", "err", "ok", "text", "data", "contentType"}

// decoders are the runtime functions that read a parameter, by location.
var decoders = map[string]string{
	spec.InPath:   "DecodePath",
	spec.InQuery:  "DecodeQuery",
	spec.InHeader: "DecodeHeader",
	spec.InCookie: "DecodeCookie",
}

// styleNames are the runtime constants of the parameter styles.
var styleNames = map[string]string{
	"simple":         "StyleSimple",
	"label":          "StyleLabel",
	"matrix":         "StyleMatrix",
	"form":           "StyleForm",
	"spaceDelimited": "StyleSpaceDelimited",
	"pipeDelimited":  "StylePipeDelimited",
	"deepObject":     "StyleDeepObject",
}

// defaultStyles are the styles of parameters that name none, by location.
var defaultStyles = map[string]string{spec.InPath: "simple", spec.InQuery: "form", spec.InHeader: "simple", spec.InCookie: "form"}

// ErrorsView is the data of the errors part: the runtime types it names.
type ErrorsView struct {
	Runtime string
}

// AdapterView is the data of the adapter part. Service is the interface, as the file writes it.
type AdapterView struct {
	Service             string
	Runtime             string
	HTTP                string
	IO                  string
	MaxMemory           int64
	IsRequestValidated  bool
	IsResponseValidated bool
	Operations          []HandlerView
}

// HandlerView is one handler method. ID is the operation name as a string literal. Bodies are
// the media types the handler tells apart, each once and never an empty one; Wildcards holds the
// body that takes any other media type, when the operation documents one such as */*.
type HandlerView struct {
	Name           string
	ID             string
	Method         string
	Path           string
	Options        string
	HasQuery       bool
	Groups         []GroupView
	HasBody        bool
	Bodies         []BodyView
	Wildcards      []BodyView
	IsBodyRequired bool
	Errors         []TypedErrorView
}

// GroupView is the parameters of one location: the options field that holds them, its struct
// type, and how each is decoded.
type GroupView struct {
	Field  string
	Type   string
	Params []ParamView
}

// ParamView is one parameter: the runtime decoder, what it reads from, the options field it
// writes to, and its runtime.Param. Name and Location are quoted, Style a runtime constant.
type ParamView struct {
	Decoder    string
	Source     string
	Target     string
	Name       string
	Location   string
	Style      string
	IsExplode  bool
	IsRequired bool
	IsJSON     bool
}

// BodyView is one media type of the request body. MediaType is quoted, in lower case and without
// parameters; OperationID is quoted; Target is the address of the options field; Type is the
// struct a multipart form fills; Assign is the expression that turns text or data, the decoded
// body, into the field's type.
type BodyView struct {
	Kind        string
	MediaType   string
	Runtime     string
	OperationID string
	IsJSON      bool
	IsForm      bool
	IsMultipart bool
	IsText      bool
	IsBytes     bool
	IsRequired  bool
	Field       string
	Target      string
	Type        string
	Assign      string
}

// bodyAt is what the bodies of one operation share.
type bodyAt struct {
	id         string
	isRequired bool
	scope      *gocode.Scope
}

// conversion is how a decoded body of the raw type lands in a field of the target type.
type conversion struct {
	raw       gomodel.Type
	target    gomodel.Type
	isPointer bool
}

// TypedErrorView answers an error type of the spec with its status.
type TypedErrorView struct {
	Var    string
	Type   string
	Status int
}

func errorsView(s *gocode.Scope) *ErrorsView {
	return &ErrorsView{Runtime: s.Import(gomodel.Import{Path: gomodel.RuntimePath})}
}

func adapterView(g *Generator, s *gocode.Scope) *AdapterView {
	v := &AdapterView{
		Service:             s.Symbol(PartService, g.opts.Name+"Interface"),
		Runtime:             s.Import(gomodel.Import{Path: gomodel.RuntimePath}),
		HTTP:                s.Import(gomodel.Import{Path: "net/http"}),
		IO:                  s.Import(gomodel.Import{Path: "io"}),
		MaxMemory:           g.opts.MultipartMaxMemory,
		IsRequestValidated:  g.opts.ValidateRequest,
		IsResponseValidated: g.opts.ValidateResponse,
	}
	if v.MaxMemory <= 0 {
		v.MaxMemory = runtime.DefaultMultipartMemory
	}
	for _, op := range g.ops {
		v.Operations = append(v.Operations, handlerView(g, op, s))
	}
	return v
}

func handlerView(g *Generator, op *gomodel.Operation, s *gocode.Scope) HandlerView {
	v := HandlerView{
		Name:    op.Name,
		ID:      gocode.Quote(op.Name),
		Method:  op.Spec.Method,
		Path:    op.Spec.Path,
		Options: s.Symbol(PartService, g.opts.Namer.ServiceRequestOptions(op.Name)),
	}
	for _, p := range op.Params {
		if decoders[p.In] == "" {
			continue
		}
		v.HasQuery = v.HasQuery || p.In == spec.InQuery
		v.Groups = append(v.Groups, groupView(g, p, s))
	}

	v.IsBodyRequired = op.Spec.Body != nil && op.Spec.Body.Required
	at := bodyAt{id: v.ID, isRequired: v.IsBodyRequired, scope: s}
	fields := bodyFields(op.Bodies, g.opts.Namer)
	seen := []string{""}
	for i, c := range op.Bodies {
		mediaType := baseMediaType(c.MediaType)
		isWildcard := strings.Contains(mediaType, "*")
		switch {
		case isWildcard && len(v.Wildcards) == 0:
			v.Wildcards = append(v.Wildcards, bodyView(c, fields[i], at))
		case !isWildcard && !slices.Contains(seen, mediaType):
			seen = append(seen, mediaType)
			v.Bodies = append(v.Bodies, bodyView(c, fields[i], at))
		}
	}
	v.HasBody = len(op.Bodies) > 0
	v.Errors = typedErrors(op, s)
	return v
}

// groupView decodes each parameter of a location into its field of the group's struct.
func groupView(g *Generator, p gomodel.ParamGroup, s *gocode.Scope) GroupView {
	v := GroupView{Field: optionFields[p.In], Type: s.Expr(gomodel.DeclRef{Decl: p.Decl})}
	for i, f := range p.Decl.Struct.Fields {
		param := p.Params[i]
		source := "query"
		switch p.In {
		case spec.InPath:
			source = g.opts.Framework.PathParam(s, param.Name)
		case spec.InHeader:
			source = gocode.Selector("r", "Header")
		case spec.InCookie:
			source = gocode.Call(gocode.Selector("r", "Cookies"))
		}
		style := param.Style
		if styleNames[style] == "" {
			style = defaultStyles[p.In]
		}

		v.Params = append(v.Params, ParamView{
			Decoder:    decoders[p.In],
			Source:     source,
			Target:     gocode.AddressOf(gocode.Selector(gocode.Selector("opts", v.Field), f.Name)),
			Name:       gocode.Quote(param.Name),
			Location:   gocode.Quote(p.In),
			Style:      styleNames[style],
			IsExplode:  param.Explode,
			IsRequired: param.Required,
			IsJSON:     param.Schema == nil && len(param.Contents) > 0 && runtime.IsJSON(param.Contents[0].Name),
		})
	}
	return v
}

// bodyView picks the decoder of a media type by the type of its field: JSON and forms decode into
// anything, multipart into a struct, any other media type into a string or into bytes. A wildcard
// media type into anything else decodes as JSON. Other pairs are taken in but not decoded.
func bodyView(c gomodel.Content, field string, at bodyAt) BodyView {
	s := at.scope
	mediaType := baseMediaType(c.MediaType)
	v := BodyView{
		Kind:        bodyNone,
		MediaType:   gocode.Quote(mediaType),
		Runtime:     s.Import(gomodel.Import{Path: gomodel.RuntimePath}),
		OperationID: at.id,
		IsRequired:  at.isRequired,
		Field:       field,
		Target:      gocode.AddressOf(gocode.Selector("opts", field)),
	}
	t := bodyType(c)
	base, isPointer := t, false
	if p, ok := t.(gomodel.Pointer); ok {
		base, isPointer = p.Elem, true
	}
	isWildcard := strings.Contains(mediaType, "*")
	isString, isBytes := gomodel.Underlying(base) == stringType, gomodel.Underlying(base) == bytesType

	switch {
	case runtime.IsJSON(mediaType), isWildcard && !isString && !isBytes:
		v.Kind, v.IsJSON = bodyJSON, true
	case mediaType == "application/x-www-form-urlencoded":
		v.Kind, v.IsForm = bodyForm, true
	case mediaType == "multipart/form-data" && isPointer && gomodel.StructDecl(base) != nil:
		v.Kind, v.IsMultipart, v.Type = bodyMultipart, true, s.Expr(base)
	case isString:
		v.Kind, v.IsText, v.Assign = bodyText, true, convert("text", conversion{raw: stringType, target: base, isPointer: isPointer}, s)
	case isBytes:
		v.Kind, v.IsBytes, v.Assign = bodyBytes, true, convert("data", conversion{raw: bytesType, target: base, isPointer: isPointer}, s)
	}
	return v
}

// baseMediaType is a media type as a request's Content-Type is compared with it: in lower case
// and without parameters.
func baseMediaType(mediaType string) string {
	base, _, _ := strings.Cut(strings.ToLower(mediaType), ";")
	return strings.TrimSpace(base)
}

// convert writes value, of the raw type, as the target type of a body field: converted when the
// target is another type, and behind a pointer when the field holds one.
func convert(value string, c conversion, s *gocode.Scope) string {
	out := value
	if c.target != c.raw {
		out = gocode.Call(s.Expr(c.target), value)
	}
	if c.isPointer {
		out = gocode.Call(gocode.Selector(s.Import(gomodel.Import{Path: gomodel.RuntimePath}), "Ptr"), out)
	}
	return out
}

// typedErrors are the error types of the spec the operation answers with, each with the status of
// the first response that carries it.
func typedErrors(op *gomodel.Operation, s *gocode.Scope) []TypedErrorView {
	var out []TypedErrorView
	var seen []*gomodel.Decl
	for _, r := range op.Responses {
		for _, c := range r.Contents {
			d := gomodel.StructDecl(c.Type)
			if d == nil || d.Error == nil || slices.Contains(seen, d) {
				continue
			}
			seen = append(seen, d)
			out = append(out, TypedErrorView{Var: errorVar(d.Name, len(out)), Type: s.Expr(gomodel.DeclRef{Decl: d}), Status: statusOf(r.Status)})
		}
	}
	return out
}

// errorVar names the variable a typed error is caught in, clear of the handler's own locals.
func errorVar(typeName string, i int) string {
	name := strings.ToLower(typeName[:1]) + typeName[1:]
	if slices.Contains(handlerLocals, name) {
		name += strconv.Itoa(i + 1)
	}
	return name
}

// statusOf is the status code a response is answered with: its own, the start of its range, or
// 500 for default.
func statusOf(status string) int {
	if code, err := strconv.Atoi(status); err == nil {
		return code
	}
	if code, err := strconv.Atoi(status[:1]); err == nil && len(status) == 3 {
		return code * 100
	}
	return 500
}
