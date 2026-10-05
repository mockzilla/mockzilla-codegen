// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package server

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/mockzilla/mockzilla-codegen/internal/gomodel"
)

func TestBodyTable(t *testing.T) {
	t.Parallel()

	owner := &gomodel.Decl{Name: "Owner", Kind: gomodel.KindStruct, Struct: &gomodel.Struct{
		Fields: []*gomodel.Field{{JSONName: "city", Default: `"Berlin"`, Value: &gomodel.BodyValue{}}},
	}}
	owner.Struct.AdditionalProperties = &gomodel.Field{JSONName: "-", Value: &gomodel.BodyValue{Values: &gomodel.BodyValue{Object: owner}}}
	pet := &gomodel.Decl{Name: "Pet", Kind: gomodel.KindStruct, Struct: &gomodel.Struct{IsClosed: true, Fields: []*gomodel.Field{
		{JSONName: "name", Required: true, Value: &gomodel.BodyValue{}},
		{JSONName: "id", Required: true, ReadOnly: true, Value: &gomodel.BodyValue{}},
		{JSONName: "tag", Value: &gomodel.BodyValue{IsNullable: true}},
		{JSONName: "owner", Value: &gomodel.BodyValue{Object: owner}},
		{JSONName: "tags", Value: &gomodel.BodyValue{Items: &gomodel.BodyValue{}}},
		{JSONName: "notes", Value: &gomodel.BodyValue{Items: &gomodel.BodyValue{IsNullable: true}}},
		{JSONName: "friends", Value: &gomodel.BodyValue{Values: &gomodel.BodyValue{Object: owner}}},
		{JSONName: "a#", Value: &gomodel.BodyValue{}},
		{JSONName: `a"`, Value: &gomodel.BodyValue{}},
		{JSONName: "unplanned"},
	}}}

	order := &gomodel.Decl{Name: "Order", Kind: gomodel.KindStruct, Struct: &gomodel.Struct{}}
	gift := &gomodel.Decl{Name: "Gift", Kind: gomodel.KindStruct, Struct: &gomodel.Struct{Fields: []*gomodel.Field{
		{JSONName: "order", Value: &gomodel.BodyValue{Object: order}},
		{JSONName: "wrap", Default: "true", Value: &gomodel.BodyValue{}},
	}}}
	gift.Struct.AdditionalProperties = &gomodel.Field{JSONName: "-", Value: &gomodel.BodyValue{Values: &gomodel.BodyValue{Object: gift}}}
	plain := &gomodel.Decl{Name: "Plain", Kind: gomodel.KindStruct, Struct: &gomodel.Struct{Fields: []*gomodel.Field{
		{JSONName: "note", Value: &gomodel.BodyValue{}},
	}}}
	order.Struct.Fields = []*gomodel.Field{
		{JSONName: "gift", Value: &gomodel.BodyValue{Object: gift}},
		{JSONName: "gifts", Value: &gomodel.BodyValue{Items: &gomodel.BodyValue{Object: gift}}},
		{JSONName: "plain", Value: &gomodel.BodyValue{Object: plain}},
		{JSONName: "item", Required: true, Value: &gomodel.BodyValue{}},
	}

	prop := func(p PropView) *PropView {
		p.Runtime = "runtime"
		return &p
	}
	type body struct {
		kind    string
		content gomodel.Content
	}
	tests := []struct {
		name      string
		isChecked bool
		bodies    []body
		wantRoots []*PropView
		want      *BodiesView
	}{
		{
			name:      "Checks list every object a body holds",
			isChecked: true,
			bodies: []body{
				{kind: bodyJSON, content: gomodel.Content{Body: &gomodel.BodyValue{Object: pet}}},
				{kind: bodyForm, content: gomodel.Content{Body: &gomodel.BodyValue{Object: pet}}},
				{kind: bodyJSON, content: gomodel.Content{Body: &gomodel.BodyValue{}}},
				{kind: bodyJSON, content: gomodel.Content{Body: &gomodel.BodyValue{IsNullable: true}}},
				{kind: bodyMultipart, content: gomodel.Content{Body: &gomodel.BodyValue{Values: &gomodel.BodyValue{}}}},
				{kind: bodyText, content: gomodel.Content{Body: &gomodel.BodyValue{Object: pet}}},
				{kind: bodyJSON},
			},
			wantRoots: []*PropView{prop(PropView{Object: `"Pet"`}), prop(PropView{Object: `"Pet"`}), prop(PropView{}), nil, nil, nil, nil},
			want: &BodiesView{Runtime: "runtime", IsChecked: true, Objects: []ObjectView{
				{
					Name:  `"Owner"`,
					Props: []*PropView{prop(PropView{Key: `"city"`, Default: "`\"Berlin\"`"})},
					Extra: prop(PropView{Object: `"Owner"`}),
				},
				{
					Name: `"Pet"`,
					Props: []*PropView{
						prop(PropView{Key: `"a\""`}),
						prop(PropView{Key: `"a#"`}),
						prop(PropView{Key: `"friends"`, Values: prop(PropView{Object: `"Owner"`})}),
						prop(PropView{Key: `"id"`}),
						prop(PropView{Key: `"name"`, IsRequired: true}),
						prop(PropView{Key: `"notes"`}),
						prop(PropView{Key: `"owner"`, Object: `"Owner"`}),
						prop(PropView{Key: `"tag"`, IsNullable: true}),
						prop(PropView{Key: `"tags"`, Items: prop(PropView{})}),
					},
					IsClosed: true,
				},
			}},
		},
		{
			name: "Without checks only what leads to a default is listed",
			bodies: []body{
				{kind: bodyJSON, content: gomodel.Content{Body: &gomodel.BodyValue{Object: order}}},
				{kind: bodyJSON, content: gomodel.Content{Body: &gomodel.BodyValue{Object: plain}}},
			},
			wantRoots: []*PropView{prop(PropView{Object: `"Order"`}), nil},
			want: &BodiesView{Runtime: "runtime", Objects: []ObjectView{
				{
					Name: `"Gift"`,
					Props: []*PropView{
						prop(PropView{Key: `"order"`, Object: `"Order"`}),
						prop(PropView{Key: `"wrap"`, Default: `"true"`}),
					},
					Extra: prop(PropView{Object: `"Gift"`}),
				},
				{
					Name: `"Order"`,
					Props: []*PropView{
						prop(PropView{Key: `"gift"`, Object: `"Gift"`}),
						prop(PropView{Key: `"gifts"`, Items: prop(PropView{Object: `"Gift"`})}),
					},
				},
			}},
		},
		{
			name:      "No table without a body to check",
			bodies:    []body{{kind: bodyJSON, content: gomodel.Content{Body: &gomodel.BodyValue{Object: plain}}}},
			wantRoots: []*PropView{nil},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			op := &gomodel.Operation{}
			for _, b := range tc.bodies {
				op.Bodies = append(op.Bodies, b.content)
			}
			table := newBodyTable([]*gomodel.Operation{op}, tc.isChecked, "runtime")
			roots := make([]*PropView, len(tc.bodies))
			for i, b := range tc.bodies {
				roots[i] = table.root(b.content, b.kind)
			}

			assert.Equal(t, tc.wantRoots, roots)
			assert.Equal(t, tc.want, table.view())
		})
	}
}
