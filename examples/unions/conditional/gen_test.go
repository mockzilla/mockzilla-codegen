// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package conditional

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestShippingJSON(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		data string
		want Shipping
	}{
		{name: "Then for the tested value", data: `{"kind":"post","address":"Main St"}`, want: Shipping{Kind: "post", Then: &ShippingThen{Address: new("Main St")}}},
		{name: "Else for any other value", data: `{"kind":"pickup","store":"North"}`, want: Shipping{Kind: "pickup", Else: &ShippingElse{Store: new("North")}}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var got Shipping
			require.NoError(t, json.Unmarshal([]byte(tc.data), &got))
			assert.Equal(t, tc.want, got)

			out, err := json.Marshal(got)
			require.NoError(t, err)
			assert.JSONEq(t, tc.data, string(out))
		})
	}
}

func TestDiscountSingleBranch(t *testing.T) {
	t.Parallel()

	var got Discount
	require.NoError(t, json.Unmarshal([]byte(`{"percent":100,"reason":"staff"}`), &got))

	assert.Equal(t, Discount{Percent: new(100), Reason: new("staff")}, got)
}
