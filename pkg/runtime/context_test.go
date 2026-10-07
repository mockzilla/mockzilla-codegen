// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package runtime

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestOperationID(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		ctx  context.Context
		want string
	}{
		{name: "A context without an operation holds none", ctx: t.Context(), want: ""},
		{name: "The ID set is the ID read", ctx: WithOperationID(t.Context(), "GetPet"), want: "GetPet"},
		{
			name: "The innermost ID wins",
			ctx:  WithOperationID(WithOperationID(t.Context(), "ListPets"), "GetPet"),
			want: "GetPet",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.want, OperationID(tc.ctx))
		})
	}
}
