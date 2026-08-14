// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package discriminator

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mockzilla/mockzilla-codegen/pkg/runtime"
)

func TestPaymentJSON(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		data string
		want Payment
	}{
		{name: "Explicit mapping", data: `{"type":"card","number":"4242"}`, want: Payment{Card: &Card{Type: "card", Number: "4242"}}},
		{name: "Second mapping value", data: `{"type":"credit_card","number":"4242"}`, want: Payment{Card: &Card{Type: "credit_card", Number: "4242"}}},
		{name: "Implicit component name", data: `{"type":"Bank","iban":"DE00"}`, want: Payment{Bank: &Bank{Type: "Bank", Iban: "DE00"}}},
		{name: "Const on the property", data: `{"type":"wallet","provider":"x"}`, want: Payment{Wallet: &Wallet{Type: "wallet", Provider: new("x")}}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var got Payment
			require.NoError(t, json.Unmarshal([]byte(tc.data), &got))
			assert.Equal(t, tc.want, got)

			out, err := json.Marshal(got)
			require.NoError(t, err)
			assert.JSONEq(t, tc.data, string(out))
		})
	}
}

func TestPaymentUnknownValue(t *testing.T) {
	t.Parallel()

	var got Payment
	err := json.Unmarshal([]byte(`{"type":"cash"}`), &got)

	require.ErrorIs(t, err, runtime.ErrUnknownDiscriminator)
	assert.EqualError(t, err, `unknown discriminator value "cash", want one of card, credit_card, Bank, wallet`)
}

func TestTolerantDefault(t *testing.T) {
	t.Parallel()

	var got Tolerant
	require.NoError(t, json.Unmarshal([]byte(`{"type":"cash"}`), &got))
	assert.Equal(t, Tolerant{Unknown: &Unknown{Type: new("cash")}}, got)

	require.NoError(t, json.Unmarshal([]byte(`{"type":"card","number":"1"}`), &got))
	assert.Equal(t, Tolerant{Card: &Card{Type: "card", Number: "1"}}, got)
}

func TestInlineJSON(t *testing.T) {
	t.Parallel()

	var got Inline
	require.NoError(t, json.Unmarshal([]byte(`{"kind":"square","side":2}`), &got))
	assert.Equal(t, Inline{Square: &InlineSquare{Kind: new("square"), Side: new(2.0)}}, got)
}
