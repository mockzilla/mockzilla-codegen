// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package transform

import (
	"testing"

	"github.com/mockzilla/mockzilla-codegen/internal/diag"
	"github.com/mockzilla/mockzilla-codegen/internal/oasdoc"
	"github.com/mockzilla/mockzilla-codegen/pkg/config"
)

func TestFilter(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		dir  string
		f    config.Filter
	}{
		{
			name: "Exclude beats include on paths",
			dir:  "filter-paths",
			f:    config.Filter{Include: config.FilterSet{Paths: []string{"/a", "/b"}}, Exclude: config.FilterSet{Paths: []string{"/b"}}},
		},
		{
			name: "Tags filter operations and webhooks",
			dir:  "filter-tags",
			f:    config.Filter{Include: config.FilterSet{Tags: []string{"pets"}}, Exclude: config.FilterSet{Tags: []string{"internal"}}},
		},
		{
			name: "Operation ids match explicit and derived ids",
			dir:  "filter-operation-ids",
			f: config.Filter{
				Include: config.FilterSet{OperationIDs: []string{"getUsersId", "replaceUser", "getShared", "getExtra", "copyOnly", "postPing"}},
				Exclude: config.FilterSet{OperationIDs: []string{"replaceUser"}},
			},
		},
		{
			name: "Webhooks by name",
			dir:  "filter-webhooks",
			f:    config.Filter{Include: config.FilterSet{Webhooks: []string{"a", "b"}}, Exclude: config.FilterSet{Webhooks: []string{"b"}}},
		},
		{
			name: "Schema properties never drop required ones",
			dir:  "filter-schema-properties",
			f: config.Filter{
				Include: config.FilterSet{SchemaProperties: map[string][]string{"Pet": {"name", "tag", "missing"}, "Nope": {"a"}, "NoProps": {"a"}}},
				Exclude: config.FilterSet{SchemaProperties: map[string][]string{"User": {"email", "id"}, "Pet": {"tag"}}},
			},
		},
		{
			name: "Extensions on component schemas",
			dir:  "filter-extensions",
			f:    config.Filter{Include: config.FilterSet{Extensions: []string{"x-keep", "x-drop"}}, Exclude: config.FilterSet{Extensions: []string{"x-drop"}}},
		},
		{
			name: "Removing every operation warns",
			dir:  "filter-everything",
			f:    config.Filter{Include: config.FilterSet{Tags: []string{"b"}}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			runGolden(t, tt.dir, func(doc *oasdoc.Doc) (bool, []diag.Diagnostic) { return Filter(doc, tt.f) })
		})
	}
}
