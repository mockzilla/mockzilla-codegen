// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package extension

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/mockzilla/mockzilla-codegen/internal/spec"
)

func TestMCPIsSkipped(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		mcp         *MCP
		defaultSkip bool
		want        bool
	}{
		{name: "No x-mcp keeps the operation"},
		{name: "No x-mcp follows the default", defaultSkip: true, want: true},
		{name: "An x-mcp without skip follows the default", mcp: &MCP{Name: "pets"}, defaultSkip: true, want: true},
		{name: "Skip wins over the default", mcp: &MCP{Skip: new(true)}, want: true},
		{name: "Skip false wins over the default", mcp: &MCP{Skip: new(false)}, defaultSkip: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, tt.mcp.IsSkipped(tt.defaultSkip))
		})
	}
}

func TestParse(t *testing.T) {
	t.Parallel()

	on, off := true, false
	tests := []struct {
		name      string
		exts      []spec.Extension
		want      Set
		wantDiags []string
	}{
		{name: "Nothing"},
		{
			name: "Names and flags",
			exts: []spec.Extension{
				ext(GoName, str("accountID")),
				ext(GoTypeName, str("Account")),
				ext(GoNameExact, boolean(true)),
				ext(SkipPointer, str("true")),
				ext(Nullable, boolean(false)),
				ext(JSONIgnore, boolean(true)),
				ext(OmitEmpty, boolean(false)),
				ext(DeprecatedReason, str("use v2")),
			},
			want: Set{Name: "accountID", TypeName: "Account", IsExactName: true, IsPointerSkipped: true, Nullable: &off, IsJSONIgnored: true, OmitEmpty: &off, DeprecatedReason: "use v2"},
		},
		{
			name: "Extra tags sorted by key",
			exts: []spec.Extension{ext(ExtraTags, obj(spec.Field{Name: "yaml", Value: str("a")}, spec.Field{Name: "db", Value: str("b")}))},
			want: Set{Tags: []Tag{{Key: "db", Value: "b"}, {Key: "yaml", Value: "a"}}},
		},
		{name: "Enum names", exts: []spec.Extension{ext(EnumNames, list(str("Low"), str("High")))}, want: Set{EnumNames: []string{"Low", "High"}}},
		{name: "Go type alone", exts: []spec.Extension{ext(GoType, str("int64"))}, want: Set{GoType: &Type{Name: "int64"}}},
		{
			name: "Go type with an import object",
			exts: []spec.Extension{ext(GoTypeImport, obj(spec.Field{Name: "Path", Value: str("github.com/google/uuid")}, spec.Field{Name: "name", Value: str("gu")})), ext(GoType, str("gu.UUID"))},
			want: Set{GoType: &Type{Name: "gu.UUID", Path: "github.com/google/uuid", Alias: "gu"}},
		},
		{
			name: "Go type with an import path",
			exts: []spec.Extension{ext(GoType, str("decimal.Decimal")), ext(GoTypeImport, str("github.com/shopspring/decimal"))},
			want: Set{GoType: &Type{Name: "decimal.Decimal", Path: "github.com/shopspring/decimal"}},
		},
		{name: "Sensitive true is full", exts: []spec.Extension{ext(SensitiveData, boolean(true))}, want: Set{Sensitive: &Mask{}}},
		{name: "Sensitive false", exts: []spec.Extension{ext(SensitiveData, boolean(false))}},
		{name: "Sensitive by name", exts: []spec.Extension{ext(SensitiveData, str("hash"))}, want: Set{Sensitive: &Mask{Kind: MaskHash}}},
		{
			name: "Sensitive object",
			exts: []spec.Extension{ext(SensitiveData, obj(spec.Field{Name: "mask", Value: str("partial")}, spec.Field{Name: "keepPrefix", Value: num("2")}, spec.Field{Name: "keepSuffix", Value: num("4")}))},
			want: Set{Sensitive: &Mask{Kind: MaskPartial, KeepPrefix: 2, KeepSuffix: 4}},
		},
		{
			name: "Sensitive regex",
			exts: []spec.Extension{ext(SensitiveData, obj(spec.Field{Name: "mask", Value: str("regex")}, spec.Field{Name: "pattern", Value: str(`\d`)}))},
			want: Set{Sensitive: &Mask{Kind: MaskRegex, Pattern: `\d`}},
		},
		{
			name: "MCP",
			exts: []spec.Extension{ext(MCPName, obj(spec.Field{Name: "skip", Value: boolean(true)}, spec.Field{Name: "name", Value: str("list_users")}, spec.Field{Name: "description", Value: str("Lists")}, spec.Field{Name: "other", Value: str("x")}))},
			want: Set{MCP: &MCP{Skip: &on, Name: "list_users", Description: "Lists"}},
		},
		{name: "Other extensions are left alone", exts: []spec.Extension{ext("x-internal", boolean(true)), ext("x-logo", obj())}},
		{
			name:      "Unknown name of ours",
			exts:      []spec.Extension{ext("x-go-nam", str("A")), ext("x-go-extra-tag", obj())},
			wantDiags: []string{"unknown extension x-go-nam; it is left out", "unknown extension x-go-extra-tag; it is left out"},
		},
		{
			name: "Wrong values",
			exts: []spec.Extension{
				ext(GoName, str("not an identifier")),
				ext(SkipPointer, str("yes")),
				ext(OmitEmpty, num("1")),
				ext(DeprecatedReason, boolean(true)),
				ext(ExtraTags, list()),
				ext(EnumNames, str("A")),
				ext(MCPName, str("skip")),
			},
			wantDiags: []string{
				"x-go-name must be a Go identifier; it is left out",
				"x-go-type-skip-optional-pointer must be a boolean; it is left out",
				"x-omitempty must be a boolean; it is left out",
				"x-deprecated-reason must be a string; it is left out",
				"x-go-extra-tags must be an object of strings; it is left out",
				"x-enum-names must be a list of Go identifiers; it is left out",
				"x-mcp must be an object; it is left out",
			},
		},
		{
			name: "Wrong values inside",
			exts: []spec.Extension{
				ext(ExtraTags, obj(spec.Field{Name: "db", Value: num("1")})),
				ext(EnumNames, list(str("A"), num("1"))),
				ext(MCPName, obj(spec.Field{Name: "skip", Value: str("maybe")}, spec.Field{Name: "name", Value: num("1")})),
			},
			want: Set{MCP: &MCP{}},
			wantDiags: []string{
				"x-go-extra-tags must be an object of strings; it is left out",
				"x-enum-names must be a list of Go identifiers; it is left out",
				"x-mcp.skip must be a boolean; it is left out",
				"x-mcp.name must be a string; it is left out",
			},
		},
		{
			name: "Wrong sensitive values",
			exts: []spec.Extension{
				ext(SensitiveData, num("1")),
				ext(SensitiveData, str("regex")),
				ext(SensitiveData, obj(spec.Field{Name: "mask", Value: str("blur")})),
				ext(SensitiveData, obj(spec.Field{Name: "keepPrefix", Value: num("-1")})),
				ext(SensitiveData, obj(spec.Field{Name: "mask", Value: str("regex")})),
			},
			wantDiags: []string{
				"x-sensitive-data must be true, a mask name (full, regex, hash, partial) or an object; it is left out",
				"x-sensitive-data must be true, a mask name (full, regex, hash, partial) or an object; it is left out",
				"x-sensitive-data must be true, a mask name (full, regex, hash, partial) or an object; it is left out",
				"x-sensitive-data must be true, a mask name (full, regex, hash, partial) or an object; it is left out",
				"x-sensitive-data must be an object with a pattern for the regex mask; it is left out",
			},
		},
		{
			name:      "Import without a type",
			exts:      []spec.Extension{ext(GoTypeImport, str("github.com/google/uuid"))},
			wantDiags: []string{"x-go-type-import needs x-go-type; it is left out"},
		},
		{
			name:      "Go type that is no string",
			exts:      []spec.Extension{ext(GoType, num("1"))},
			wantDiags: []string{"x-go-type must be a Go type; it is left out"},
		},
		{
			name:      "Import of the wrong kind",
			exts:      []spec.Extension{ext(GoType, str("uuid.UUID")), ext(GoTypeImport, list())},
			want:      Set{GoType: &Type{Name: "uuid.UUID"}},
			wantDiags: []string{"x-go-type-import must be a path or an object with path and name; it is left out"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, diags := Parse(tc.exts, spec.Origin{Pointer: "/components/schemas/A", File: "api.yaml", Line: 3, Col: 5})

			assert.Equal(t, tc.want, got)
			var messages []string
			for _, d := range diags {
				messages = append(messages, d.Message)
				assert.Equal(t, 3, d.Origin.Line)
			}
			assert.Equal(t, tc.wantDiags, messages)
		})
	}
}

func str(s string) spec.Value {
	return spec.Value{Kind: spec.KindString, Str: s}
}

func boolean(b bool) spec.Value {
	return spec.Value{Kind: spec.KindBool, Bool: b}
}

func num(n string) spec.Value {
	return spec.Value{Kind: spec.KindNumber, Num: json.Number(n)}
}

func obj(fields ...spec.Field) spec.Value {
	return spec.Value{Kind: spec.KindObject, Fields: fields}
}

func list(items ...spec.Value) spec.Value {
	return spec.Value{Kind: spec.KindArray, Items: items}
}

func ext(name string, v spec.Value) spec.Extension {
	return spec.Extension{Name: name, Value: v}
}
