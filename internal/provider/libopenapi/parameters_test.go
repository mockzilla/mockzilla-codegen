// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package libopenapi

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/mockzilla/mockzilla-codegen/internal/spec"
)

func TestMergeParameters(t *testing.T) {
	t.Parallel()

	pathID := &spec.Parameter{Name: "id", In: spec.InPath}
	pathTrace := &spec.Parameter{Name: "trace", In: spec.InHeader}
	opTrace := &spec.Parameter{Name: "trace", In: spec.InHeader, Required: true}
	opTraceQuery := &spec.Parameter{Name: "trace", In: spec.InQuery}
	opDry := &spec.Parameter{Name: "dry", In: spec.InQuery}
	opDryAgain := &spec.Parameter{Name: "dry", In: spec.InQuery, Required: true}

	tests := []struct {
		name   string
		shared []*spec.Parameter
		own    []*spec.Parameter
		want   []*spec.Parameter
	}{
		{name: "No path-level parameters", own: []*spec.Parameter{opDry}, want: []*spec.Parameter{opDry}},
		{name: "No operation parameters", shared: []*spec.Parameter{pathID}, want: []*spec.Parameter{pathID}},
		{
			name:   "Same in and name replaces in place",
			shared: []*spec.Parameter{pathID, pathTrace},
			own:    []*spec.Parameter{opDry, opTrace},
			want:   []*spec.Parameter{pathID, opTrace, opDry},
		},
		{
			name:   "Same name in another location is a new parameter",
			shared: []*spec.Parameter{pathTrace},
			own:    []*spec.Parameter{opTraceQuery},
			want:   []*spec.Parameter{pathTrace, opTraceQuery},
		},
		{
			name:   "Operation duplicates are kept, only path-level ones are replaced",
			shared: []*spec.Parameter{pathID},
			own:    []*spec.Parameter{opDry, opDryAgain},
			want:   []*spec.Parameter{pathID, opDry, opDryAgain},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, mergeParameters(tt.shared, tt.own))
		})
	}
}
