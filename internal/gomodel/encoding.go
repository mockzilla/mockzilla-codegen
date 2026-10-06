// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

// The content types the encoding of a form request body declares for its properties.

package gomodel

import (
	"strings"

	"github.com/mockzilla/mockzilla-codegen/internal/diag"
	"github.com/mockzilla/mockzilla-codegen/internal/spec"
	"github.com/mockzilla/mockzilla-codegen/pkg/runtime"
)

// planEncodings sets Content.Encoding on each form request body whose media type has an encoding.
func planEncodings(ops []*Operation, diags *diag.Collector) {
	for _, op := range ops {
		if op.Spec.Body == nil {
			continue
		}
		for i, mt := range op.Spec.Body.Contents {
			if len(mt.Encodings) > 0 {
				op.Bodies[i].Encoding = encodingOf(mt, op.Bodies[i].Type, diags)
			}
		}
	}
}

// encodingOf reads the content types of mt's encoding, as the spec does, and warns about the rest.
func encodingOf(mt *spec.MediaType, t Type, diags *diag.Collector) map[string]string {
	base := baseMediaType(mt.Name)
	isMultipart := base == "multipart/form-data"
	if !isMultipart && base != "application/x-www-form-urlencoded" {
		diags.Append(encodingWarning(mt, diag.CodeEncodingIgnored, "the encoding of "+mt.Name+" is ignored, as it applies to forms only"))
		return nil
	}
	d := FormDecl(t)
	switch {
	case d != nil && d.Kind == KindUnion:
		diags.Append(encodingWarning(mt, diag.CodeEncodingIgnored, "the encoding of "+mt.Name+" is not supported for a union body yet; its parts are written and read by their schema types"))
		return nil
	case d == nil:
		diags.Append(encodingWarning(mt, diag.CodeEncodingIgnored, "the encoding of "+mt.Name+" is ignored, as the body has no properties"))
		return nil
	}

	out := map[string]string{}
	var unknown, styled, headed []string
	for _, e := range mt.Encodings {
		f := fieldOf(d, e.Name)
		switch {
		case f == nil:
			unknown = append(unknown, e.Name)
			continue
		case e.Style != "" || e.Explode != nil || e.AllowReserved:
			styled = append(styled, e.Name)
		case e.ContentType != "":
			out[e.Name] = e.ContentType
			if !isJSONListed(e.ContentType) && holdsObject(f.Type) {
				diags.Append(encodingWarning(mt, diag.CodeEncodingUnsupported, "the encoding of "+mt.Name+" declares "+quoted([]string{e.Name})+
					" as "+e.ContentType+", but it holds an object, which is written and read as JSON only, so sending or reading it fails"))
			}
		}
		if isMultipart && len(e.Headers) > 0 {
			headed = append(headed, e.Name)
		}
	}

	if len(unknown) > 0 {
		diags.Append(encodingWarning(mt, diag.CodeEncodingIgnored, "the encoding of "+mt.Name+" names "+quoted(unknown)+", which the body has no property for"))
	}
	if len(styled) > 0 {
		diags.Append(encodingWarning(mt, diag.CodeEncodingIgnored, "the encoding of "+mt.Name+" sets style, explode or allowReserved for "+quoted(styled)+
			", which is not supported yet; each is written and read by its schema type"))
	}
	if len(headed) > 0 {
		diags.Append(encodingWarning(mt, diag.CodeEncodingIgnored, "the encoding of "+mt.Name+" sets headers for "+quoted(headed)+", which is not supported yet; each part goes without them"))
	}
	if len(out) == 0 {
		return nil
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

// holdsObject reports a type whose values, or the items of its lists, are objects: structs or maps.
func holdsObject(t Type) bool {
	switch x := unalias(t).(type) {
	case Pointer:
		return holdsObject(x.Elem)
	case Slice:
		return holdsObject(x.Elem)
	case Map:
		return true
	case DeclRef:
		return x.Decl.Kind == KindStruct
	}
	return false
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
