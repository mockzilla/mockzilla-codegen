// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

// A oneOf or anyOf whose members only list required properties, checked on the fields of a struct.

package gomodel

import (
	"fmt"
	"slices"
	"strings"

	"github.com/mockzilla/mockzilla-codegen/internal/diag"
	"github.com/mockzilla/mockzilla-codegen/internal/spec"
)

// requiredCounts gives each struct and union the counts of its required-only oneOfs and anyOfs.
func (b *builder) requiredCounts(decls []*Decl) {
	for _, d := range decls {
		if d.schema == nil || d.Struct == nil || d.isParams {
			continue
		}
		for _, r := range b.flat.requirements(b.flat.flatten(d.schema)) {
			kind := "oneOf"
			if r.AnyOf != nil {
				kind = "anyOf"
			}
			c, why := requiredCount(d, r)
			if why != "" {
				b.diags.Append(diag.Diagnostic{
					Severity: diag.Warning,
					Code:     diag.CodeKeywordUnsupported,
					Pointer:  r.Origin.Pointer + "/" + kind,
					Origin:   origin(r.Origin),
					Message:  fmt.Sprintf("the %s of %s is not checked: %s", kind, d.Name, why),
				})
				continue
			}
			d.requires = append(d.requires, c)
		}
	}
}

// requiredCount counts the members of r whose properties d has all set, or says why it cannot.
func requiredCount(d *Decl, r *spec.Schema) (Count, string) {
	c, list := Count{Func: "ExactlyOneOf"}, r.OneOf
	if r.AnyOf != nil {
		c.Func, list = "AtLeastOneOf", r.AnyOf
	}

	names := make([]string, len(list))
	for i, m := range list {
		var fields []*Field
		for _, name := range m.Required {
			j := slices.IndexFunc(d.Struct.Fields, func(f *Field) bool { return f.JSONName == name })
			if j < 0 {
				return Count{}, fmt.Sprintf("it has no property %q", name)
			}

			// A required field that cannot be nil is always there.
			switch f := d.Struct.Fields[j]; {
			case nilable(f.Type), isWrapped(f.Type):
				fields = append(fields, f)
			case !f.Required:
				return Count{}, fmt.Sprintf("property %q has no pointer, so whether it is set is unknown", name)
			}
		}
		c.Fields = append(c.Fields, fields)
		names[i] = strings.Join(m.Required, " and ")
		if len(m.Required) > 1 {
			names[i] = "(" + names[i] + ")"
		}
	}
	c.Names = strings.Join(names, " or ")
	return c, ""
}
