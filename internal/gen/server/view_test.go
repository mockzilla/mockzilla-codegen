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

func TestBodyFields(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		mediaTypes []string
		want       []string
	}{
		{name: "No body"},
		{name: "One body", mediaTypes: []string{"application/xml"}, want: []string{"Body"}},
		{name: "Several bodies take their tags", mediaTypes: []string{"application/json", "text/plain"}, want: []string{"BodyJSON", "BodyText"}},
		{
			name:       "A tag used twice takes the type, then a number",
			mediaTypes: []string{"application/xml", "text/xml", "application/xml; charset=utf-8", "text/xml; q=1"},
			want:       []string{"BodyXML", "BodyTextXML", "BodyApplicationXML", "BodyTextXML2"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var bodies []gomodel.Content
			for _, mt := range tc.mediaTypes {
				bodies = append(bodies, gomodel.Content{MediaType: mt})
			}

			got := bodyFields(bodies, naming.New(nil))

			if tc.want == nil {
				assert.Empty(t, got)
				return
			}
			assert.Equal(t, tc.want, got)
		})
	}
}
