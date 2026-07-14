// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package spec

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestVersionString(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		v    Version
		want string
	}{
		{name: "3.0", v: V30, want: "3.0"},
		{name: "3.1", v: V31, want: "3.1"},
		{name: "3.2", v: V32, want: "3.2"},
		{name: "Zero value is unknown", want: "unknown"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, tt.v.String())
		})
	}
}
