// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

// The data of adapter.tmpl and errors.tmpl: a handler per operation that decodes the request,
// calls the service and writes its answer, and the error types it answers with.

package server

import (
	"slices"
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
	bodyValue     = "value"
	bodyBytes     = "bytes"
	bodyFile      = "file"
	bodyNone      = "none"
)

// contentTypeVar is the variable a handler holds the media type of the request body in.
const contentTypeVar = "contentType"

var (
	stringType = gomodel.Builtin{Name: "string"}
	bytesType  = gomodel.Slice{Elem: gomodel.Builtin{Name: "byte"}}
	fileType   = gomodel.Qualified{Import: gomodel.Import{Path: gomodel.RuntimePath}, Name: "File"}
	anyType    = gomodel.Builtin{Name: "any"}
)

// decoders are the runtime functions that read a parameter, by location.
var decoders = map[string]string{
	spec.InPath:   "DecodePath",
	spec.InQuery:  "DecodeQuery",
	spec.InHeader: "DecodeHeader",
	spec.InCookie: "DecodeCookie",
}

// ErrorsView is the data of the errors part: the runtime types it names.
type ErrorsView struct {
	HTTPServer string
}

// AdapterView is the data of the adapter part. Service is the interface, as the file writes it;
// Handler is the shape of the handlers the framework takes. Presence is the table the handlers
// check bodies against, nil when none does. Rejects are the error types of the spec that answer
// the requests the handlers turn away.
type AdapterView struct {
	Service             string
	Runtime             string
	HTTPServer          string
	Validation          string
	HTTP                string
	IO                  string
	Errors              string
	MaxMemory           int64
	IsRequestValidated  bool
	IsResponseValidated bool
	Handler             framework.Handler
	Operations          []HandlerView
	Presence            *PresenceView
	Rejects             []RejectView
}

// HandlerView is one handler method. ID is the operation name as a string literal. Bodies are
// the media types the handler tells apart, each once and never an empty one; Patterns are the
// keys that take the media types they match, application/*+json any JSON and a range such as
// text/* its type; Wildcards holds the body that takes any other media type, when the operation
// documents one such as */*. Tag is what the body switch is on, contentType, or empty when
// Patterns make it a switch of conditions; Empty is the case of a body without a media type.
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
	Tag            string
	Empty          string
	Bodies         []BodyView
	Patterns       []BodyView
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
// parameters; Case is the case of the body switch that takes it; OperationID is quoted; Target is the address of the options field; Type is the
// struct a multipart form fills; Assign is the expression that turns text, data or file, the
// decoded body, into the field's type; Return is the statement that leaves the handler. Check is
// what the body is checked against before it is decoded, nil when it is not. Encoding is nil or the
// variable encoding, which EncodingLiteral sets.
type BodyView struct {
	Kind            string
	MediaType       string
	Case            string
	Runtime         string
	OperationID     string
	IsJSON          bool
	IsForm          bool
	IsMultipart     bool
	IsText          bool
	IsValue         bool
	IsBytes         bool
	IsFile          bool
	IsRequired      bool
	Field           string
	Target          string
	Type            string
	Assign          string
	Return          string
	Check           *PropView
	Encoding        string
	EncodingLiteral string
}

// bodyAt is what the bodies of one operation share: the operation, the statement that leaves its
// handler, and the table its bodies are checked against.
type bodyAt struct {
	id         string
	isRequired bool
	ret        string
	scope      *gocode.Scope
	table      *presenceTable
}

// conversion is how a decoded body of the raw type lands in a field of the target type.
type conversion struct {
	raw       gomodel.Type
	target    gomodel.Type
	isPointer bool
}

// TypedErrorView answers an error type of the spec with its status and quoted media type.
type TypedErrorView struct {
	Type      string
	Status    int
	MediaType string
}

func errorsView(s *gocode.Scope) *ErrorsView {
	return &ErrorsView{HTTPServer: s.Import(gomodel.Import{Path: gomodel.HTTPServerPath})}
}

func adapterView(g *Generator, s *gocode.Scope) *AdapterView {
	v := &AdapterView{
		Service:             s.Symbol(PartService, g.Interface()),
		Runtime:             s.Import(gomodel.Import{Path: gomodel.RuntimePath}),
		HTTPServer:          s.Import(gomodel.Import{Path: gomodel.HTTPServerPath}),
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
	table := newPresenceTable(g.ops, g.opts.ValidateRequest, v.Runtime)
	for _, op := range g.ops {
		v.Operations = append(v.Operations, handlerView(g, op, s, table))
	}
	v.Presence = table.view()
	if v.Presence != nil {
		v.Validation = s.Import(gomodel.Import{Path: gomodel.ValidationPath})
	}

	v.Rejects = rejectViews(g.ops, s)
	return v
}

func handlerView(g *Generator, op *gomodel.Operation, s *gocode.Scope, table *presenceTable) HandlerView {
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
	at := bodyAt{id: v.ID, isRequired: v.IsBodyRequired, ret: g.opts.Framework.Handler(s).Return, scope: s, table: table}
	fields := operation.BodyFields(op.Bodies, g.opts.Namer)
	var jsonPatterns, ranges []BodyView
	seen := []string{""}
	for i, c := range op.Bodies {
		key := operation.BaseMediaType(c.MediaType)
		if isJSONPattern(key) {
			// Every JSON pattern takes the same media types, so the first one wins.
			key = "+json"
		}
		switch {
		case slices.Contains(seen, key):
		case key == "+json":
			jsonPatterns = append(jsonPatterns, bodyView(c, fields[i], at))
		case isTypeRange(key):
			ranges = append(ranges, bodyView(c, fields[i], at))
		case !strings.Contains(key, "*"):
			v.Bodies = append(v.Bodies, bodyView(c, fields[i], at))
		case len(v.Wildcards) == 0:
			v.Wildcards = append(v.Wildcards, bodyView(c, fields[i], at))
		}
		seen = append(seen, key)
	}
	v.Patterns = slices.Concat(jsonPatterns, ranges)
	switchOn(&v)
	v.HasBody = len(op.Bodies) > 0
	v.Errors = typedErrors(op, s)
	return v
}

// switchOn sets what the body switch is on: contentType, or conditions when there are patterns.
func switchOn(v *HandlerView) {
	v.Tag, v.Empty = contentTypeVar, gocode.Quote("")
	if len(v.Patterns) == 0 {
		return
	}

	v.Tag, v.Empty = "", gocode.Equal(contentTypeVar, v.Empty)
	for i := range v.Bodies {
		v.Bodies[i].Case = gocode.Equal(contentTypeVar, v.Bodies[i].Case)
	}
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
	v.Case = bodyCase(operation.BaseMediaType(c.MediaType), v.Runtime)
	t := operation.BodyType(c)
	base := gomodel.Elem(t)
	_, isPointer := t.(gomodel.Pointer)

	switch v.Kind {
	case bodyJSON:
		v.IsJSON, v.Check = true, at.table.root(c, v.Kind)
	case bodyForm:
		v.IsForm, v.Check = true, at.table.root(c, v.Kind)
		v.Encoding, v.EncodingLiteral = encodingOf(c, v.Runtime)
	case bodyMultipart:
		v.IsMultipart, v.Type, v.Check = true, s.Expr(base), at.table.root(c, v.Kind)
		v.Encoding, v.EncodingLiteral = encodingOf(c, v.Runtime)
	case bodyFile:
		v.IsFile, v.Assign = true, convert("file", conversion{raw: fileType, target: base, isPointer: isPointer}, s)
	case bodyText:
		v.IsText, v.Assign = true, convert("text", conversion{raw: stringType, target: base, isPointer: isPointer}, s)
	case bodyValue:
		v.IsValue = true
	case bodyBytes:
		v.IsBytes, v.Assign = true, convert("data", conversion{raw: bytesType, target: base, isPointer: isPointer}, s)
	}
	return v
}

// encodingOf is what a form body is read with, nil or encoding, and the literal of encoding.
func encodingOf(c gomodel.Content, runtimePkg string) (string, string) {
	if len(c.Encoding) == 0 {
		return "nil", ""
	}
	return "encoding", operation.Encoding(c, runtimePkg)
}

// bodyKind picks the decoder of a media type by the type of its field: JSON and forms decode into
// anything, multipart into a struct or a union that reads forms, any other media type into a file,
// a string or bytes, and text into a value such as a number. A range reads as its member, text/*
// as text/plain; another wildcard media type decodes as JSON. Other pairs are taken in but not
// decoded: bodyNone.
func bodyKind(c gomodel.Content) string {
	mediaType := operation.Concrete(c.MediaType)
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
	case strings.HasPrefix(mediaType, "text/") && operation.IsTextValue(under):
		return bodyValue
	}
	return bodyNone
}

// bodyCase is the case of the body switch that takes a media type: a pattern's condition, else the media type quoted.
func bodyCase(mediaType, runtimePkg string) string {
	switch {
	case isJSONPattern(mediaType):
		return gocode.Call(gocode.Selector(runtimePkg, "IsJSON"), contentTypeVar)
	case isTypeRange(mediaType):
		return gocode.Equal(gocode.Call(gocode.Selector(runtimePkg, "MediaRange"), contentTypeVar), gocode.Quote(mediaType))
	}
	return gocode.Quote(mediaType)
}

// isJSONPattern reports a key such as application/*+json, which takes any JSON media type.
func isJSONPattern(mediaType string) bool {
	return strings.Contains(mediaType, "*") && runtime.IsJSON(mediaType)
}

// isTypeRange reports a media range of one type, such as text/*, which takes the subtypes of its type.
func isTypeRange(mediaType string) bool {
	typ, subtype, _ := strings.Cut(mediaType, "/")
	return subtype == "*" && typ != "*"
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
				Type:      s.Expr(gomodel.DeclRef{Decl: d}),
				Status:    operation.StatusOf(r.Status),
				MediaType: gocode.Quote(errorMediaType(c.MediaType)),
			})
		}
	}
	return out
}

// errorMediaType is the media type an error type answers with: the documented one, JSON under a range.
func errorMediaType(mediaType string) string {
	if strings.Contains(mediaType, "*") {
		return "application/json"
	}
	return mediaType
}
