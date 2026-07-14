// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package spec

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTypeSetHas(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		set   TypeSet
		other TypeSet
		want  bool
	}{
		{name: "Single type is found", set: TypeString | TypeNull, other: TypeNull, want: true},
		{name: "Every type of the argument must be present", set: TypeString, other: TypeString | TypeNull},
		{name: "Missing type", set: TypeInteger, other: TypeNumber},
		{name: "Empty argument is never contained", set: TypeString},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, tt.set.Has(tt.other))
		})
	}
}
