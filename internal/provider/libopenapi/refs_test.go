// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package libopenapi

import (
	"errors"
	"testing"

	"github.com/pb33f/libopenapi/index"
	"github.com/stretchr/testify/assert"

	"github.com/mockzilla/codegen/internal/diag"
	"github.com/mockzilla/codegen/internal/provider"
	"github.com/mockzilla/codegen/internal/spec"
)

func TestBuildIssues(t *testing.T) {
	t.Parallel()

	circular := &index.ResolvingError{ErrorRef: errors.New("loop"), CircularReference: &index.CircularReferenceResult{}}
	err := errors.Join(errors.New("odd thing"), errors.Join(circular, errors.New("other thing")))
	cycles := []*index.CircularReferenceResult{
		{Journey: []*index.Reference{{Name: "A"}, {Name: "B"}}},
	}

	c := newConverter(spec.V31, provider.ParseOptions{File: "spec.yaml"})
	c.buildIssues(err, cycles)
	file := diag.Origin{File: "spec.yaml"}
	assert.Equal(t, []diag.Diagnostic{
		{Severity: diag.Warning, Code: diag.CodeBuildIssue, Origin: file, Message: "odd thing"},
		{Severity: diag.Warning, Code: diag.CodeBuildIssue, Origin: file, Message: "other thing"},
		{Severity: diag.Info, Code: diag.CodeCircularRef, Origin: file, Message: "circular reference: A -> B"},
	}, c.diags.List())
}

func TestRefPointer(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		ref  string
		want string
	}{
		{name: "Local component", ref: "#/components/schemas/Pet", want: "/components/schemas/Pet"},
		{name: "Percent-encoded path matches the escaped form", ref: "#/paths/~1pets~1%7Bid%7D/get", want: "/paths/~1pets~1{id}/get"},
		{name: "Whole document", ref: "#", want: ""},
		{name: "Bad percent escape keeps the text", ref: "#/a%zz", want: "/a%zz"},
		{name: "Other file comes back as written", ref: "models.yaml#/Pet", want: "models.yaml#/Pet"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, refPointer(tt.ref))
		})
	}
}

func TestComponentName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		ptr  string
		want string
	}{
		{name: "Component", ptr: "/components/schemas/Pet", want: "Pet"},
		{name: "Escaped component name", ptr: "/components/schemas/a~1b", want: "a/b"},
		{name: "Inside a component", ptr: "/components/schemas/Pet/properties/name"},
		{name: "Not under components", ptr: "/paths/~1a/get/responses/200"},
		{name: "Other file", ptr: "models.yaml#/Pet"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, componentName(tt.ptr))
		})
	}
}
