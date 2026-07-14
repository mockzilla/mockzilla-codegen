// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package diag

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSeverityString(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		sev  Severity
		want string
	}{
		{name: "Info", sev: Info, want: "info"},
		{name: "Warning", sev: Warning, want: "warning"},
		{name: "Error", sev: Error, want: "error"},
		{name: "Unknown value reads as error", sev: Severity(9), want: "error"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, tt.sev.String())
		})
	}
}

func TestCollectorList(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   []Diagnostic
		want []Diagnostic
	}{
		{name: "Empty collector lists nothing"},
		{
			name: "Sorted by file, line, column, code, pointer and message",
			in: []Diagnostic{
				{Code: "b", Origin: Origin{File: "b.yaml", Line: 1}},
				{Code: "z", Origin: Origin{File: "a.yaml", Line: 2, Col: 1}},
				{Code: "b", Pointer: "/b", Origin: Origin{File: "a.yaml", Line: 1, Col: 3}},
				{Code: "b", Pointer: "/a", Message: "y", Origin: Origin{File: "a.yaml", Line: 1, Col: 3}},
				{Code: "b", Pointer: "/a", Message: "x", Origin: Origin{File: "a.yaml", Line: 1, Col: 3}},
				{Code: "a", Origin: Origin{File: "a.yaml", Line: 1, Col: 3}},
				{Code: "c", Origin: Origin{File: "a.yaml", Line: 1, Col: 2}},
			},
			want: []Diagnostic{
				{Code: "c", Origin: Origin{File: "a.yaml", Line: 1, Col: 2}},
				{Code: "a", Origin: Origin{File: "a.yaml", Line: 1, Col: 3}},
				{Code: "b", Pointer: "/a", Message: "x", Origin: Origin{File: "a.yaml", Line: 1, Col: 3}},
				{Code: "b", Pointer: "/a", Message: "y", Origin: Origin{File: "a.yaml", Line: 1, Col: 3}},
				{Code: "b", Pointer: "/b", Origin: Origin{File: "a.yaml", Line: 1, Col: 3}},
				{Code: "z", Origin: Origin{File: "a.yaml", Line: 2, Col: 1}},
				{Code: "b", Origin: Origin{File: "b.yaml", Line: 1}},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var c Collector
			c.Append(tt.in...)
			assert.Equal(t, tt.want, c.List())
		})
	}
}

func TestCollectorListIsACopy(t *testing.T) {
	t.Parallel()

	var c Collector
	c.Append(Diagnostic{Code: "b"}, Diagnostic{Code: "a"})
	first := c.List()
	c.Append(Diagnostic{Code: "0"})

	assert.Equal(t, []Diagnostic{{Code: "a"}, {Code: "b"}}, first)
	assert.Equal(t, []Diagnostic{{Code: "0"}, {Code: "a"}, {Code: "b"}}, c.List())
}
