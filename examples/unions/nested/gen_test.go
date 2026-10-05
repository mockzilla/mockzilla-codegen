// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package nested

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPaymentJSON(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		data string
		want Payment
	}{
		{name: "A card two levels down", data: `{"number":"4111","cvc":"123"}`, want: Payment{Card: &Card{Visa: &Visa{Number: "4111", Cvc: "123"}}}},
		{name: "The other card", data: `{"number":"3782","cid":"1234"}`, want: Payment{Card: &Card{Amex: &Amex{Number: "3782", Cid: "1234"}}}},
		{name: "A bank account", data: `{"iban":"DE89"}`, want: Payment{Bank: &Bank{Sepa: &Sepa{Iban: "DE89"}}}},
		{name: "Three levels down", data: `{"routing":"011","account":"42"}`, want: Payment{Bank: &Bank{Domestic: &Domestic{Ach: &Ach{Routing: "011", Account: "42"}}}}},
		{name: "Its other member", data: `{"swift":"DEUT","account":"42"}`, want: Payment{Bank: &Bank{Domestic: &Domestic{Wire: &Wire{Swift: "DEUT", Account: "42"}}}}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var got Payment
			require.NoError(t, json.Unmarshal([]byte(tc.data), &got))
			assert.Equal(t, tc.want, got)
			require.NoError(t, got.Validate())

			out, err := json.Marshal(got)
			require.NoError(t, err)
			assert.JSONEq(t, tc.data, string(out))
		})
	}
}
