// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package libopenapi

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestStatusRank(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		status string
		want   int
	}{
		{name: "Code", status: "204", want: rankCode},
		{name: "Range", status: "4XX", want: rankRange},
		{name: "Lower-case range", status: "4xx", want: rankRange},
		{name: "Default", status: "default", want: rankDefault},
		{name: "Out of range code", status: "600", want: rankOther},
		{name: "Anything else", status: "ok", want: rankOther},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, statusRank(tt.status))
		})
	}
}
