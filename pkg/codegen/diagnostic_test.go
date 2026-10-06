// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package codegen

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDiagnosticString(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		d    Diagnostic
		want string
	}{
		{
			name: "Place and pointer",
			d: Diagnostic{
				Severity: SeverityWarning, Code: "invalid-status", Pointer: "/paths/~1pets/get/responses/ok",
				File: "api.yaml", Line: 9, Col: 9, Message: "left out",
			},
			want: "api.yaml:9:9: warning invalid-status: left out [/paths/~1pets/get/responses/ok]",
		},
		{
			name: "File without a line",
			d:    Diagnostic{Severity: SeverityInfo, Code: "prune-skipped", File: "api.yaml", Message: "nothing pruned"},
			want: "api.yaml: info prune-skipped: nothing pruned",
		},
		{
			name: "No place",
			d:    Diagnostic{Severity: SeverityError, Code: "import-unused", Message: "unused"},
			want: "error import-unused: unused",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.want, tc.d.String())
		})
	}
}
