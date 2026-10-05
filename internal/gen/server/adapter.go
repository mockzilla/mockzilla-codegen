// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

// The data of adapter.tmpl and errors.tmpl: a handler per operation that decodes the request,
// calls the service and writes its answer, and the error types it answers with.

package server

import (
	"slices"
	"strconv"
	"strings"

	"github.com/mockzilla/mockzilla-codegen/internal/gen/operation"
	"github.com/mockzilla/mockzilla-codegen/internal/gen/server/framework"
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
	bodyFile      = "file"
	bodyNone      = "none"
)

var (
	stringType = gomodel.Builtin{Name: "string"}
	bytesType  = gomodel.Slice{Elem: gomodel.Builtin{Name: "byte"}}
	fileType   = gomodel.Qualified{Import: gomodel.Import{Path: gomodel.RuntimePath}, Name: "File"}
	anyType    = gomodel.Builtin{Name: "any"}
)

// handlerLocals are the variables a generated handler declares, c being the context of a Native
// framework; a typed error variable never takes one of them.
var handlerLocals = []string{"a", "c", "w", "r", "opts", "query", "res", "err", "ok", "text", "data", "file", "contentType"}

// decoders are the runtime functions that read a parameter, by location.
var decoders = map[string]string{
	spec.InPath:   "DecodePath",
	spec.InQuery:  "DecodeQuery",
	spec.InHeader: "DecodeHeader",
	spec.InCookie: "DecodeCookie",
}

// ErrorsView is the data of the errors part: the runtime types it names.
type ErrorsView struct {
	Runtime string
}

// AdapterView is the data of the adapter part. Service is the interface, as the file writes it;
// Handler is the shape of the handlers the framework takes.
type AdapterView struct {
	Service             string
	Runtime             string
	HTTP                string
	IO                  string
	Errors              string
	MaxMemory           int64
	IsRequestValidated  bool
	IsResponseValidated bool
	Handler             framework.Handler
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
	QueryString    *ParamView
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
	Default    string
}

// BodyView is one media type of the request body. MediaType is quoted, in lower case and without
// parameters; OperationID is quoted; Target is the address of the options field; Type is the
// struct a multipart form fills; Assign is the expression that turns text, data or file, the
// decoded body, into the field's type; Return is the statement that leaves the handler.
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
	IsFile      bool
	IsRequired  bool
	Field       string
	Target      string
	Type        string
	Assign      string
	Return      string
}

// bodyAt is what the bodies of one operation share: the operation, and the statement that leaves
// its handler.
type bodyAt struct {
	id         string
	isRequired bool
	ret        string
	scope      *gocode.Scope
}

// conversion is how a decoded body of the raw type lands in a field of the target type.
type conversion struct {
	raw       gomodel.Type
	target    gomodel.Type
	isPointer bool
}

// TypedErrorView answers an error type of the spec with its status and quoted media type.
type TypedErrorView struct {
	Var       string
	Type      string
	Status    int
	MediaType string
}

func errorsView(s *gocode.Scope) *ErrorsView {
	return &ErrorsView{Runtime: s.Import(gomodel.Import{Path: gomodel.RuntimePath})}
}

func adapterView(g *Generator, s *gocode.Scope) *AdapterView {
	v := &AdapterView{
		Service:             s.Symbol(PartService, g.Interface()),
		Runtime:             s.Import(gomodel.Import{Path: gomodel.RuntimePath}),
		HTTP:                s.Import(gomodel.Import{Path: "net/http"}),
		IO:                  s.Import(gomodel.Import{Path: "io"}),
		Errors:              s.Import(gomodel.Import{Path: "errors"}),
		MaxMemory:           g.opts.MultipartMaxMemory,
		IsRequestValidated:  g.opts.ValidateRequest,
		IsResponseValidated: g.opts.ValidateResponse,
		Handler:             g.opts.Framework.Handler(s),
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
		v.HasQuery = v.HasQuery || p.In == spec.InQuery
		v.Groups = append(v.Groups, groupView(g, p, s))
	}
	if qs := op.QueryString; qs != nil {
		v.QueryString = &ParamView{
			Source:     gocode.Selector(gocode.Selector("r", "URL"), "RawQuery"),
			Target:     gocode.AddressOf(gocode.Selector("opts", operation.QueryStringField(op, g.opts.Namer))),
			Name:       gocode.Quote(qs.Param.Name),
			Location:   gocode.Quote(spec.InQueryString),
			IsRequired: qs.Param.Required,
			IsJSON:     runtime.IsJSON(qs.Content.MediaType),
			Default:    quoteDefault(qs.Default),
		}
	}

	v.IsBodyRequired = op.Spec.Body != nil && op.Spec.Body.Required
	at := bodyAt{id: v.ID, isRequired: v.IsBodyRequired, ret: g.opts.Framework.Handler(s).Return, scope: s}
	fields := operation.BodyFields(op.Bodies, g.opts.Namer)
	seen := []string{""}
	for i, c := range op.Bodies {
		mediaType := operation.BaseMediaType(c.MediaType)
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
	v := GroupView{Field: operation.GroupField(p.In, g.opts.Namer), Type: s.Expr(gomodel.DeclRef{Decl: p.Decl})}
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
		v.Params = append(v.Params, ParamView{
			Decoder:    decoders[p.In],
			Source:     source,
			Target:     gocode.AddressOf(gocode.Selector(gocode.Selector("opts", v.Field), f.Name)),
			Name:       gocode.Quote(param.Name),
			Location:   gocode.Quote(p.In),
			Style:      operation.Style(param),
			IsExplode:  param.Explode,
			IsRequired: param.Required,
			IsJSON:     operation.IsJSONParam(param),
			Default:    quoteDefault(p.Defaults[param.Name]),
		})
	}
	return v
}

// quoteDefault writes the JSON of a default as a Go string, raw when it holds a quote.
func quoteDefault(value string) string {
	switch {
	case value == "":
		return ""
	case strings.Contains(value, `"`):
		return gocode.RawString(value)
	}
	return gocode.Quote(value)
}

// bodyView is the body of one media type, read by its decoder.
func bodyView(c gomodel.Content, field string, at bodyAt) BodyView {
	s := at.scope
	v := BodyView{
		Kind:        bodyKind(c),
		MediaType:   gocode.Quote(operation.BaseMediaType(c.MediaType)),
		Runtime:     s.Import(gomodel.Import{Path: gomodel.RuntimePath}),
		OperationID: at.id,
		IsRequired:  at.isRequired,
		Field:       field,
		Target:      gocode.AddressOf(gocode.Selector("opts", field)),
		Return:      at.ret,
	}
	t := operation.BodyType(c)
	base := gomodel.Elem(t)
	_, isPointer := t.(gomodel.Pointer)

	switch v.Kind {
	case bodyJSON:
		v.IsJSON = true
	case bodyForm:
		v.IsForm = true
	case bodyMultipart:
		v.IsMultipart, v.Type = true, s.Expr(base)
	case bodyFile:
		v.IsFile, v.Assign = true, convert("file", conversion{raw: fileType, target: base, isPointer: isPointer}, s)
	case bodyText:
		v.IsText, v.Assign = true, convert("text", conversion{raw: stringType, target: base, isPointer: isPointer}, s)
	case bodyBytes:
		v.IsBytes, v.Assign = true, convert("data", conversion{raw: bytesType, target: base, isPointer: isPointer}, s)
	}
	return v
}

// bodyKind picks the decoder of a media type by the type of its field: JSON and forms decode into
// anything, multipart into a struct or a union that reads forms, any other media type into a file,
// a string or bytes. A wildcard media type into anything else decodes as JSON. Other pairs are
// taken in but not decoded: bodyNone.
func bodyKind(c gomodel.Content) string {
	mediaType := operation.BaseMediaType(c.MediaType)
	t := operation.BodyType(c)
	_, isPointer := t.(gomodel.Pointer)
	under := gomodel.Underlying(gomodel.Elem(t))
	isString, isBytes, isFile := under == stringType, under == bytesType, under == fileType

	switch {
	case runtime.IsJSON(mediaType), strings.Contains(mediaType, "*") && !isString && !isBytes && !isFile:
		return bodyJSON
	case mediaType == "application/x-www-form-urlencoded":
		return bodyForm
	case mediaType == "multipart/form-data" && isPointer && gomodel.FormDecl(t) != nil:
		return bodyMultipart
	case isFile:
		return bodyFile
	case isString:
		return bodyText
	case isBytes:
		return bodyBytes
	}
	return bodyNone
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

// typedErrors are the error types the operation answers with, by the first response of each.
func typedErrors(op *gomodel.Operation, s *gocode.Scope) []TypedErrorView {
	var out []TypedErrorView
	var seen []*gomodel.Decl
	for _, r := range op.Responses {
		for _, c := range r.Contents {
			d := gomodel.ErrorDecl(c.Type)
			if d == nil || slices.Contains(seen, d) {
				continue
			}
			seen = append(seen, d)
			out = append(out, TypedErrorView{
				Var:       errorVar(d.Name, len(out)),
				Type:      s.Expr(gomodel.DeclRef{Decl: d}),
				Status:    operation.StatusOf(r.Status),
				MediaType: gocode.Quote(c.MediaType),
			})
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
