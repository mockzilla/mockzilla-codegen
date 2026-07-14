// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package libopenapi

import (
	"encoding/json"
	"testing"

	"github.com/pb33f/libopenapi/datamodel/high/base"
	"github.com/stretchr/testify/assert"
	"go.yaml.in/yaml/v4"

	"github.com/mockzilla/codegen/internal/spec"
)

func TestBoundSourceResolve(t *testing.T) {
	t.Parallel()

	flag := func(b bool) *base.DynamicValue[bool, float64] { return &base.DynamicValue[bool, float64]{N: 0, A: b} }
	num := func(f float64) *base.DynamicValue[bool, float64] {
		return &base.DynamicValue[bool, float64]{N: 1, B: f}
	}
	tests := []struct {
		name    string
		src     boundSource
		isLower bool
		want    *spec.Bound
	}{
		{name: "Nothing set", isLower: true},
		{name: "Inclusive only", src: boundSource{value: new(3.0)}, isLower: true, want: &spec.Bound{Value: "3"}},
		{name: "3.0 flag makes the value exclusive", src: boundSource{value: new(3.0), exclusive: flag(true)}, isLower: true, want: &spec.Bound{Value: "3", Exclusive: true}},
		{name: "3.0 false flag keeps it inclusive", src: boundSource{value: new(3.0), exclusive: flag(false)}, isLower: true, want: &spec.Bound{Value: "3"}},
		{name: "3.0 flag without a value means nothing", src: boundSource{exclusive: flag(true)}, isLower: true},
		{name: "3.1 exclusive only", src: boundSource{exclusive: num(5)}, isLower: true, want: &spec.Bound{Value: "5", Exclusive: true}},
		{name: "Lower: larger exclusive wins", src: boundSource{value: new(3.0), exclusive: num(5)}, isLower: true, want: &spec.Bound{Value: "5", Exclusive: true}},
		{name: "Lower: larger inclusive wins", src: boundSource{value: new(7.0), exclusive: num(5)}, isLower: true, want: &spec.Bound{Value: "7"}},
		{name: "Lower: equal values are exclusive", src: boundSource{value: new(5.0), exclusive: num(5)}, isLower: true, want: &spec.Bound{Value: "5", Exclusive: true}},
		{name: "Upper: smaller exclusive wins", src: boundSource{value: new(10.0), exclusive: num(5)}, want: &spec.Bound{Value: "5", Exclusive: true}},
		{name: "Upper: smaller inclusive wins", src: boundSource{value: new(3.0), exclusive: num(5)}, want: &spec.Bound{Value: "3"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, tt.src.resolve(tt.isLower))
		})
	}
}

func TestNumber(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		node *yaml.Node
		f    float64
		want json.Number
	}{
		{name: "Written form is kept", node: &yaml.Node{Value: "1.50"}, f: 1.5, want: "1.50"},
		{name: "No node formats the float", f: 0.25, want: "0.25"},
		{name: "YAML-only form is reformatted", node: &yaml.Node{Value: "+2"}, f: 2, want: "2"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, number(tt.node, tt.f))
		})
	}
}

func TestRefsShareTargets(t *testing.T) {
	t.Parallel()

	doc, _ := parseFixture(t, "schemas.yaml")
	schemas := map[string]*spec.Schema{}
	for _, s := range doc.Components.Schemas {
		schemas[s.Name] = s.Value
	}
	pathSchema := doc.Operations[0].Responses[0].Contents[0].Schema

	tests := []struct {
		name   string
		target *spec.Schema
		want   *spec.Schema
	}{
		{name: "Ref to a component", target: pathSchema.Items.Ref.Target, want: schemas["Pet"]},
		{name: "Ref next to sibling keywords", target: schemas["Described"].Ref.Target, want: schemas["Pet"]},
		{name: "Ref into a converted component", target: schemas["PetName"].Ref.Target, want: schemas["Pet"].Properties[1].Schema},
		{name: "Ref into a component converted later", target: schemas["Early"].Ref.Target, want: schemas["Late"].Properties[0].Schema},
		{name: "Ref into an operation", target: schemas["FromPath"].Ref.Target, want: pathSchema},
		{name: "Discriminator mapping", target: schemas["Shape"].Discriminator.Mapping[1].Ref.Target, want: schemas["Square"]},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Same(t, tt.want, tt.target)
		})
	}
}

func TestComponentUsagesShareSchemas(t *testing.T) {
	t.Parallel()

	params, _ := parseFixture(t, "parameters.yaml")
	responses, _ := parseFixture(t, "responses.yaml")
	bodies, _ := parseFixture(t, "bodies.yaml")

	tests := []struct {
		name  string
		usage *spec.Schema
		want  *spec.Schema
	}{
		{name: "Parameter", usage: params.Operations[0].Params[7].Schema, want: params.Components.Parameters[0].Value.Schema},
		{name: "Parameter through an alias component", usage: params.Operations[0].Params[8].Schema, want: params.Components.Parameters[0].Value.Schema},
		{name: "Response", usage: responses.Operations[0].Responses[2].Contents[0].Schema, want: responses.Components.Responses[0].Value.Contents[0].Schema},
		{name: "Header", usage: responses.Operations[0].Responses[0].Headers[1].Schema, want: responses.Components.Headers[0].Value.Schema},
		{name: "Request body", usage: bodies.Operations[1].Body.Contents[0].Schema, want: bodies.Components.RequestBodies[0].Value.Contents[0].Schema},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Same(t, tt.want, tt.usage)
		})
	}
}
