// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

// The encoding object of a form request body: the content type or the style of each property.

package gomodel

import (
	"cmp"
	"strings"

	"github.com/mockzilla/mockzilla-codegen/internal/diag"
	"github.com/mockzilla/mockzilla-codegen/internal/spec"
	"github.com/mockzilla/mockzilla-codegen/pkg/runtime"
)

// planEncodings sets Content.Encoding on each form request body whose media type has an encoding.
func (c *collector) planEncodings(ops []*Operation) {
	for _, op := range ops {
		if op.Spec.Body == nil {
			continue
		}
		for i, mt := range op.Spec.Body.Contents {
			if len(mt.Encodings) > 0 {
				op.Bodies[i].Encoding = c.encodingOf(mt, op.Bodies[i].Type)
			}
		}
	}
}

// encodingOf reads the encoding of mt, as the spec does, and warns about the rest.
func (c *collector) encodingOf(mt *spec.MediaType, t Type) map[string]Encoding {
	base := baseMediaType(mt.Name)
	isMultipart := base == "multipart/form-data"
	if !isMultipart && base != "application/x-www-form-urlencoded" {
		c.diags.Append(encodingWarning(mt, diag.CodeEncodingIgnored, "the encoding of "+mt.Name+" is ignored, as it applies to forms only"))
		return nil
	}
	d := FormDecl(t)
	switch {
	case d != nil && d.Kind == KindUnion:
		c.diags.Append(encodingWarning(mt, diag.CodeEncodingIgnored, "the encoding of "+mt.Name+" is not supported for a union body yet; its parts are written and read by their schema types"))
		return nil
	case d == nil || mt.Schema == nil:
		c.diags.Append(encodingWarning(mt, diag.CodeEncodingIgnored, "the encoding of "+mt.Name+" is ignored, as the body has no properties"))
		return nil
	}

	schemas := c.propertySchemas(mt.Schema)
	out := map[string]Encoding{}
	var unknown, headed []string
	for _, e := range mt.Encodings {
		f := fieldOf(d, e.Name)
		if f == nil {
			unknown = append(unknown, e.Name)
			continue
		}
		if isMultipart && len(e.Headers) > 0 {
			headed = append(headed, e.Name)
		}
		if e.Style != "" || e.Explode != nil || e.AllowReserved {
			if styled, ok := c.styled(mt, e, f.Type, schemas[e.Name]); ok {
				out[e.Name] = styled
				continue
			}
		}
		if e.ContentType != "" {
			out[e.Name] = Encoding{ContentType: e.ContentType}
			if !isJSONListed(e.ContentType) && holds(f.Type, isObject) {
				c.diags.Append(encodingWarning(mt, diag.CodeEncodingUnsupported, "the encoding of "+mt.Name+" declares "+quoted([]string{e.Name})+
					" as "+e.ContentType+", but it holds an object, which is written and read as JSON only, so sending or reading it fails"))
			}
		}
	}

	if len(unknown) > 0 {
		c.diags.Append(encodingWarning(mt, diag.CodeEncodingIgnored, "the encoding of "+mt.Name+" names "+quoted(unknown)+", which the body has no property for"))
	}
	if len(headed) > 0 {
		c.diags.Append(encodingWarning(mt, diag.CodeEncodingIgnored, "the encoding of "+mt.Name+" sets headers for "+quoted(headed)+", which is not supported yet; each part goes without them"))
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// styled is e as the style of a query parameter, and false with a warning when it cannot write the property.
func (c *collector) styled(mt *spec.MediaType, e *spec.Encoding, t Type, s *spec.Schema) (Encoding, bool) {
	style := cmp.Or(e.Style, styleForm)
	isExplode := style == styleForm
	if e.Explode != nil {
		isExplode = *e.Explode
	}

	p := &spec.Parameter{Name: e.Name, In: spec.InQuery, Style: style, Explode: isExplode, Schema: s}
	var why string
	switch {
	case holds(t, isFile):
		why = "holds a file, which no style writes"
	case style == styleDeep:
		// deepObject writes a union through its JSON, so only the other styles need one of scalars.
		why = c.shapeIssue(p)
	default:
		why = c.paramIssue(p)
	}
	what := "the encoding of " + mt.Name + ": property " + quoted([]string{e.Name})
	if why != "" {
		c.diags.Append(encodingWarning(mt, diag.CodeEncodingIgnored, what+" "+why+"; it is written and read by its schema type"))
		return Encoding{}, false
	}
	if style == styleDeep && c.deepUndefined(s, map[*spec.Schema]bool{}) {
		c.diags.Append(deepConvention(mt.Origin.Pointer+"/encoding", mt.Origin, what))
	}
	return Encoding{Style: style, IsExplode: isExplode, IsReserved: e.AllowReserved}, true
}

// propertySchemas are the schemas of the properties of the body schema s, by name.
func (c *collector) propertySchemas(s *spec.Schema) map[string]*spec.Schema {
	out := map[string]*spec.Schema{}
	for _, p := range c.flat.flatten(target(s)).Properties {
		out[p.Name] = p.Schema
	}
	return out
}

func encodingWarning(mt *spec.MediaType, code, message string) diag.Diagnostic {
	return diag.Diagnostic{
		Severity: diag.Warning,
		Code:     code,
		Pointer:  mt.Origin.Pointer + "/encoding",
		Origin:   origin(mt.Origin),
		Message:  message,
	}
}

// fieldOf is the field of d whose JSON name is name, or nil.
func fieldOf(d *Decl, name string) *Field {
	for _, f := range d.Struct.Fields {
		if f.JSONName == name {
			return f
		}
	}
	return nil
}

// isJSONListed reports a content type that lists a JSON media type, which any value is written in.
func isJSONListed(contentType string) bool {
	for t := range strings.SplitSeq(contentType, ",") {
		if t = strings.TrimSpace(t); !strings.Contains(t, "*") && runtime.IsJSON(t) {
			return true
		}
	}
	return false
}

// holds reports a type whose values, or the items of its lists, are what is reports.
func holds(t Type, is func(Type) bool) bool {
	switch x := unalias(t).(type) {
	case Pointer:
		return holds(x.Elem, is)
	case Nullable:
		return holds(x.Elem, is)
	case Slice:
		return holds(x.Elem, is)
	}
	return is(unalias(t))
}

// isObject reports a struct or a map.
func isObject(t Type) bool {
	switch x := t.(type) {
	case Map:
		return true
	case DeclRef:
		return x.Decl.Kind == KindStruct
	}
	return false
}

// isFile reports runtime.File.
func isFile(t Type) bool {
	q, ok := t.(Qualified)
	return ok && q.Import == importRuntime && q.Name == "File"
}

// quoted writes names as "a", "b".
func quoted(names []string) string {
	return `"` + strings.Join(names, `", "`) + `"`
}

// baseMediaType is a media type in lower case, without its parameters.
func baseMediaType(mediaType string) string {
	base, _, _ := strings.Cut(strings.ToLower(mediaType), ";")
	return strings.TrimSpace(base)
}
