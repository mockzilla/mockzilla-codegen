// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package server

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/mockzilla/mockzilla-codegen/internal/gomodel"
	"github.com/mockzilla/mockzilla-codegen/internal/naming"
)

func TestHeaderSuffix(t *testing.T) {
	t.Parallel()

	headers := &gomodel.Decl{Name: "Headers"}
	tests := []struct {
		name      string
		responses []gomodel.Response
		want      string
	}{
		{name: "One response with headers takes no suffix", responses: []gomodel.Response{{Status: "200", Headers: headers}, {Status: "404"}}},
		{name: "Several take their status", responses: []gomodel.Response{{Status: "200", Headers: headers}, {Status: "4XX", Headers: headers}}, want: "4XX"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.want, headerSuffix(naming.New(nil), "4XX", &gomodel.Operation{Responses: tc.responses}))
		})
	}
}
