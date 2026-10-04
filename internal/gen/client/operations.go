// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

// The data of operations.tmpl: the methods of each operation, how they encode the request and
// which response body they return.

package client

import (
	"slices"
	"strings"

	"github.com/mockzilla/mockzilla-codegen/internal/gen/operation"
	"github.com/mockzilla/mockzilla-codegen/internal/gocode"
	"github.com/mockzilla/mockzilla-codegen/internal/gomodel"
	"github.com/mockzilla/mockzilla-codegen/internal/spec"
	"github.com/mockzilla/mockzilla-codegen/pkg/runtime"
)

// Body encoders of the request builder, by what a body field holds.
const (
	encodeJSON      = "JSONBody"
	encodeForm      = "FormBody"
	encodeMultipart = "MultipartBody"
	encodeText      = "TextBody"
	encodeBytes     = "BytesBody"
	encodeFile      = "FileBody"
)

var (
	stringType = gomodel.Builtin{Name: "string"}
	bytesType  = gomodel.Slice{Elem: gomodel.Builtin{Name: "byte"}}
	anyType    = gomodel.Builtin{Name: "any"}
	fileType   = gomodel.Qualified{Import: gomodel.Import{Path: gomodel.RuntimePath}, Name: "File"}
)

// encoders are the request builder methods that add a parameter, by location.
var encoders = map[string]string{
	spec.InPath:   "PathParam",
	spec.InQuery:  "QueryParam",
	spec.InHeader: "HeaderParam",
	spec.InCookie: "CookieParam",
}

// methodConsts are the constants of net/http for the methods it names.
var methodConsts = map[string]string{
	"GET":     "MethodGet",
	"HEAD":    "MethodHead",
	"POST":    "MethodPost",
	"PUT":     "MethodPut",
	"PATCH":   "MethodPatch",
	"DELETE":  "MethodDelete",
	"CONNECT": "MethodConnect",
	"OPTIONS": "MethodOptions",
	"TRACE":   "MethodTrace",
}

// wildcardMediaTypes are what a body under a wildcard media type is sent as, by encoder.
var wildcardMediaTypes = map[string]string{encodeJSON: "application/json", encodeText: "text/plain", encodeBytes: "application/octet-stream"}

// OperationsView is the data of the operations part. Client is the client type as the file writes
// it.
type OperationsView struct {
	Client       string
	Context      string
	HTTP         string
	Runtime      string
	Slices       string
	HasEnvelopes bool
	User         map[string]any
	Operations   []OperationView
}

// OperationView is one operation: the request it builds, the method that returns the body of its
// success response, and with HasEnvelopes the method that returns its envelope. Method is the
// net/http constant or a quoted method; Path is quoted. IsSendable is false when the body is
// required and the client can send none of its media types, so the request is never built.
// Zero is the value the plain method returns on an error. Targets are what the plain method
// decodes, with the other documented 2xx statuses when it has a Result, EnvelopeTargets what the
// HasEnvelopes method decodes into the envelope Response. Stream is the Stream method of an
// operation that answers in a sequential media type, nil without HasStreams.
type OperationView struct {
	SignatureView
	Method          string
	Path            string
	Groups          []GroupView
	Bodies          []BodyView
	IsBodyRequired  bool
	IsSendable      bool
	Zero            string
	Targets         []TargetView
	EnvelopeTargets []TargetView
	Stream          *StreamView
}

// StreamView is the Stream method of an operation. MediaType is the sequential media type it asks
// for; Frame is the type of one frame; Targets are the error bodies it decodes; Field is the
// envelope field that holds the stream.
type StreamView struct {
	MediaType string
	Frame     string
	Targets   []TargetView
	Field     string
}

// GroupView is the parameters of one location: the options field that holds them and how each
// is added to the request.
type GroupView struct {
	Field  string
	Params []ParamView
}

// ParamView is one parameter: the builder method that adds it, the expression of its value and
// its runtime.Param. Name is quoted, Style a runtime constant.
type ParamView struct {
	Encoder    string
	Value      string
	Name       string
	Style      string
	IsExplode  bool
	IsRequired bool
	IsJSON     bool
}

// BodyView is one body field: the expression that says it is set, the builder method that sends
// it with the expression of its value, and the quoted media type the method takes, empty for one
// that needs none. An empty Encoder is a body the client cannot send, whose media type it reports.
type BodyView struct {
	IsSet     string
	Encoder   string
	Value     string
	MediaType string
}

// TargetView is one runtime.Target: the quoted status and media type, and the address of what
// the body is decoded into, empty for a status whose body is not read; IsHeaders marks the typed
// headers of the status.
type TargetView struct {
	Status    string
	MediaType string
	Dst       string
	IsHeaders bool
}

func operationsView(g *Generator, s *gocode.Scope) *OperationsView {
	v := &OperationsView{
		Client:       s.Symbol(PartCore, g.opts.Name),
		HasEnvelopes: g.opts.HasEnvelopes,
		User:         g.opts.User,
	}
	if len(g.ops) == 0 {
		return v
	}

	v.Context = s.Import(gomodel.Import{Path: "context"})
	v.HTTP = s.Import(gomodel.Import{Path: "net/http"})
	v.Runtime = s.Import(gomodel.Import{Path: gomodel.RuntimePath})
	v.Slices = s.Import(gomodel.Import{Path: "slices"})
	for _, op := range g.ops {
		v.Operations = append(v.Operations, operationView(g, op, s, v.HTTP))
	}
	return v
}

func operationView(g *Generator, op *gomodel.Operation, s *gocode.Scope, httpPkg string) OperationView {
	v := OperationView{
		SignatureView:  signatureView(g, op, s),
		Method:         methodExpr(op.Spec.Method, httpPkg),
		Path:           gocode.Quote(op.Spec.Path),
		IsBodyRequired: op.Spec.Body != nil && op.Spec.Body.Required,
	}
	for _, p := range op.Params {
		v.Groups = append(v.Groups, groupView(g, p))
	}
	fields := operation.BodyFields(op.Bodies, g.opts.Namer)
	for i, c := range op.Bodies {
		v.Bodies = append(v.Bodies, bodyView(c, fields[i], s))
	}
	v.IsSendable = isSendable(v.Bodies, v.IsBodyRequired)

	if r, c, ok := SuccessBody(op); ok {
		v.Zero = gocode.Zero(operation.BodyType(c))
		v.Targets = append(v.Targets, TargetView{Status: gocode.Quote(r.Status), MediaType: gocode.Quote(c.MediaType), Dst: gocode.AddressOf("out")})
		v.Targets = append(v.Targets, otherSuccesses(op, r.Status)...)
	}
	v.Targets = append(v.Targets, errorTargets(op, s)...)

	if g.opts.HasStreams {
		v.Stream = streamView(op, s)
	}
	if g.opts.HasEnvelopes {
		for _, f := range envelopeFields(g, op) {
			if f.isStream {
				v.Stream.Field = f.name
				continue
			}
			v.EnvelopeTargets = append(v.EnvelopeTargets, TargetView{
				Status:    gocode.Quote(f.status),
				MediaType: gocode.Quote(f.mediaType),
				Dst:       gocode.AddressOf(gocode.Selector("out", f.name)),
				IsHeaders: f.isHeaders,
			})
		}
	}
	return v
}

// streamView is the Stream method of an operation, nil for one without a sequential response.
func streamView(op *gomodel.Operation, s *gocode.Scope) *StreamView {
	_, c, ok := streamBody(op)
	if !ok {
		return nil
	}
	return &StreamView{
		MediaType: c.MediaType,
		Frame:     s.Expr(frameType(c)),
		Targets:   errorTargets(op, s),
	}
}

// streamType writes the pointer to a runtime.Stream of frame.
func streamType(frame string, s *gocode.Scope) string {
	return gocode.Deref(gocode.Index(gocode.Selector(s.Import(gomodel.Import{Path: gomodel.RuntimePath}), "Stream"), frame))
}

// frameType is the type of one frame of a sequential content: its item type, or bytes without one
// or when its JSON is a string, such as a date-time or a string enum, which a frame carries as text.
func frameType(c gomodel.Content) gomodel.Type {
	if c.Item == nil || gomodel.JSONKinds(c.Item) == gomodel.JSONString {
		return bytesType
	}
	return c.Item
}

// groupView adds each parameter of a location from its field of the group's struct.
func groupView(g *Generator, p gomodel.ParamGroup) GroupView {
	v := GroupView{Field: operation.GroupField(p.In, g.opts.Namer)}
	for i, f := range p.Decl.Struct.Fields {
		param := p.Params[i]
		v.Params = append(v.Params, ParamView{
			Encoder:    encoders[p.In],
			Value:      gocode.Selector(gocode.Selector("opts", v.Field), f.Name),
			Name:       gocode.Quote(param.Name),
			Style:      operation.Style(param),
			IsExplode:  param.Explode,
			IsRequired: param.Required || p.In == spec.InPath,
			IsJSON:     operation.IsJSONParam(param),
		})
	}
	return v
}

// bodyView picks the encoder of a media type by the type of its field: JSON and forms take
// anything, multipart a struct, a File streams, any other media type is sent as a string or as
// bytes, whichever its field is. A wildcard media type sends anything else as JSON. Other pairs,
// such as XML into a struct, cannot be sent.
func bodyView(c gomodel.Content, field string, s *gocode.Scope) BodyView {
	t := operation.BodyType(c)
	value := gocode.Selector("opts", field)
	v := BodyView{IsSet: gocode.NotNil(value), Value: value, MediaType: gocode.Quote(c.MediaType)}
	base := gomodel.Elem(t)
	under := gomodel.Underlying(base)
	mediaType := strings.ToLower(c.MediaType)
	isWildcard := strings.Contains(mediaType, "*")

	switch {
	case runtime.IsJSON(mediaType), isWildcard && under != stringType && under != bytesType && under != fileType:
		v.Encoder = encodeJSON
	case mediaType == "application/x-www-form-urlencoded":
		v.Encoder, v.MediaType = encodeForm, ""
	case mediaType == "multipart/form-data" && gomodel.StructDecl(t) != nil:
		v.Encoder, v.MediaType = encodeMultipart, ""
	case under == fileType:
		v.Encoder, v.Value = encodeFile, gocode.Deref(value)
	case under == stringType:
		v.Encoder, v.Value = encodeText, held(value, base, t, s)
		if t == stringType {
			v.IsSet = gocode.NotEmpty(value)
		}
	case under == bytesType:
		v.Encoder, v.Value = encodeBytes, held(value, base, t, s)
	}
	if isWildcard {
		v.MediaType = gocode.Quote(wildcardMediaTypes[v.Encoder])
	}
	return v
}

// isSendable says whether a request with these bodies can be built: not when the body is required
// and the client can send none of them.
func isSendable(bodies []BodyView, isRequired bool) bool {
	return !isRequired || len(bodies) == 0 || slices.ContainsFunc(bodies, func(b BodyView) bool { return b.Encoder != "" })
}

// held writes value, a field of type t, as its underlying string or bytes: dereferenced when the
// field holds a pointer, and converted when base is a defined type.
func held(value string, base, t gomodel.Type, s *gocode.Scope) string {
	out := value
	if t != base {
		out = gocode.Deref(value)
	}
	if under := gomodel.Underlying(base); under != base {
		out = gocode.Call(s.Expr(under), out)
	}
	return out
}

// SuccessBody is the lowest documented 2xx response with a body the client decodes, with the
// body the plain method returns: its JSON one, else its first the client decodes.
func SuccessBody(op *gomodel.Operation) (gomodel.Response, gomodel.Content, bool) {
	r, ok := lowestSuccess(op, isDecodable)
	if !ok {
		return gomodel.Response{}, gomodel.Content{}, false
	}
	c, _ := operation.FirstBody(slices.DeleteFunc(slices.Clone(r.Contents), func(c gomodel.Content) bool { return !isDecodable(c) }))
	return r, c, true
}

// streamBody is the lowest documented 2xx response with a body in a sequential media type, with
// the first such body: what the Stream method reads.
func streamBody(op *gomodel.Operation) (gomodel.Response, gomodel.Content, bool) {
	r, ok := lowestSuccess(op, isSequential)
	if !ok {
		return gomodel.Response{}, gomodel.Content{}, false
	}
	return r, r.Contents[slices.IndexFunc(r.Contents, isSequential)], true
}

// lowestSuccess is the lowest documented 2xx response with a body that fits accepts.
func lowestSuccess(op *gomodel.Operation, fits func(gomodel.Content) bool) (gomodel.Response, bool) {
	var best *gomodel.Response
	for i := range op.Responses {
		r := &op.Responses[i]
		status := operation.StatusOf(r.Status)
		if status < 200 || status > 299 || !slices.ContainsFunc(r.Contents, fits) {
			continue
		}
		if best == nil || status < operation.StatusOf(best.Status) {
			best = r
		}
	}
	if best == nil {
		return gomodel.Response{}, false
	}
	return *best, true
}

// IsStreamOnly reports an operation whose 2xx responses have bodies in sequential media types
// only, so that only its Stream method can read them: the plain method blocks until the server
// hangs up.
func IsStreamOnly(op *gomodel.Operation) bool {
	if _, _, ok := streamBody(op); !ok {
		return false
	}
	for _, r := range op.Responses {
		if status := operation.StatusOf(r.Status); status < 200 || status > 299 {
			continue
		}
		if slices.ContainsFunc(r.Contents, func(c gomodel.Content) bool { return !isSequential(c) }) {
			return false
		}
	}
	return true
}

// IsOtherSuccess reports a documented 2xx response besides the status success. The plain method
// does not read its body, so it returns none.
func IsOtherSuccess(r gomodel.Response, success string) bool {
	status := operation.StatusOf(r.Status)
	return r.Status != success && status >= 200 && status <= 299
}

func isSequential(c gomodel.Content) bool {
	return runtime.IsSequential(c.MediaType)
}

// otherSuccesses are the documented 2xx statuses besides success, which the plain method takes
// without reading their body.
func otherSuccesses(op *gomodel.Operation, success string) []TargetView {
	var out []TargetView
	for _, r := range op.Responses {
		if IsOtherSuccess(r, success) {
			out = append(out, TargetView{Status: gocode.Quote(r.Status)})
		}
	}
	return out
}

// errorTargets are the bodies of the responses outside 2xx that the client decodes and whose
// type is an error type, which the plain method decodes into the error it returns.
func errorTargets(op *gomodel.Operation, s *gocode.Scope) []TargetView {
	var out []TargetView
	for _, r := range op.Responses {
		if status := operation.StatusOf(r.Status); status >= 200 && status <= 299 {
			continue
		}
		for _, c := range r.Contents {
			d := errorDecl(c.Type)
			if d == nil || !isDecodable(c) {
				continue
			}
			out = append(out, TargetView{Status: gocode.Quote(r.Status), MediaType: gocode.Quote(c.MediaType), Dst: gocode.Call("new", s.Expr(gomodel.DeclRef{Decl: d}))})
		}
	}
	return out
}

// errorDecl is the error type a value of type t is, through pointers and aliases, or nil.
func errorDecl(t gomodel.Type) *gomodel.Decl {
	for {
		switch x := t.(type) {
		case gomodel.Pointer:
			t = x.Elem
		case gomodel.DeclRef:
			if x.Decl.Error != nil {
				return x.Decl
			}
			if x.Decl.Kind != gomodel.KindAlias && x.Decl.Kind != gomodel.KindDefined {
				return nil
			}
			t = x.Decl.Target
		default:
			return nil
		}
	}
}

// methodExpr is the net/http constant of an HTTP method, or the method quoted when it has none.
func methodExpr(method, httpPkg string) string {
	if name, ok := methodConsts[method]; ok {
		return gocode.Selector(httpPkg, name)
	}
	return gocode.Quote(method)
}
