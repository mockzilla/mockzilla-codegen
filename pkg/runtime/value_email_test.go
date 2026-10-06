// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package runtime

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/mockzilla/mockzilla-codegen/pkg/runtime/validation"
)

func TestEmailValidate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		email Email
		want  error
	}{
		{name: "Bare address", email: "a@example.com"},
		{name: "Display name", email: "A <a@example.com>", want: validation.Error{Message: "must be a valid email", Rule: validation.RuleFormat, Limit: "email"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, tt.email.Validate())
		})
	}
}
