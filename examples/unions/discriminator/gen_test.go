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

func TestPaymentMarshalDiscriminator(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		value   Payment
		want    string
		wantErr string
	}{
		{name: "An empty value gets the only one", value: Payment{Bank: &Bank{Iban: "DE00"}}, want: `{"type":"Bank","iban":"DE00"}`},
		{name: "A value that picks the variant stays", value: Payment{Card: &Card{Type: "credit_card", Number: "1"}}, want: `{"type":"credit_card","number":"1"}`},
		{name: "Two values are not chosen from", value: Payment{Card: &Card{Number: "1"}}, wantErr: "type: must be set, Card takes card or credit_card"},
		{name: "A value of another variant", value: Payment{Card: &Card{Type: "Bank", Number: "1"}}, wantErr: `type: "Bank" picks Bank, not Card`},
		{name: "An unknown value", value: Payment{Bank: &Bank{Type: "cash"}}, wantErr: `type: "cash" picks no variant`},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			out, err := json.Marshal(tc.value)

			if tc.wantErr != "" {
				require.ErrorContains(t, err, tc.wantErr)
				require.EqualError(t, tc.value.Validate(), tc.wantErr)
				return
			}
			require.NoError(t, err)
			assert.JSONEq(t, tc.want, string(out))
			assert.NoError(t, tc.value.Validate())
		})
	}
}

func TestTolerantMarshalDefault(t *testing.T) {
	t.Parallel()

	out, err := json.Marshal(Tolerant{Unknown: &Unknown{Type: new("cash")}})
	require.NoError(t, err)
	assert.JSONEq(t, `{"type":"cash"}`, string(out))

	_, err = json.Marshal(Tolerant{Card: &Card{Type: "cash"}})
	require.ErrorContains(t, err, `type: "cash" picks Unknown, not Card`)
}

func TestServerJSON(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		data string
		want Server
	}{
		{name: "The second value of Web", data: `{"server_type":"apache","host":"h"}`, want: Server{ServerType: new("apache"), Web: &Web{Host: new("h")}}},
		{name: "The one value of Shell", data: `{"server_type":"shell","key":"k"}`, want: Server{ServerType: new("shell"), Shell: &Shell{Key: new("k")}}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var got Server
			require.NoError(t, json.Unmarshal([]byte(tc.data), &got))
			assert.Equal(t, tc.want, got)

			out, err := json.Marshal(got)
			require.NoError(t, err)
			assert.JSONEq(t, tc.data, string(out))
		})
	}
}

func TestServerMarshal(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		value   Server
		want    string
		wantErr string
	}{
		{name: "The value set on the union", value: Server{ServerType: new("nginx"), Web: &Web{}}, want: `{"server_type":"nginx"}`},
		{name: "An empty value gets the only one", value: Server{Shell: &Shell{}}, want: `{"server_type":"shell"}`},
		{name: "Two values are not chosen from", value: Server{Web: &Web{}}, wantErr: "server_type: must be set, Web takes nginx or apache"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			out, err := json.Marshal(tc.value)

			if tc.wantErr != "" {
				require.ErrorContains(t, err, tc.wantErr)
				require.EqualError(t, tc.value.Validate(), tc.wantErr)
				return
			}
			require.NoError(t, err)
			assert.JSONEq(t, tc.want, string(out))
		})
	}
}

func TestConnectionJSON(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		data string
		want Connection
	}{
		{name: "A true flag picks Secure", data: `{"is_secure":true,"cert":"c"}`, want: Connection{Secure: &Secure{IsSecure: new(SecureIsSecureTrue), Cert: new("c")}}},
		{name: "A false flag picks Plain", data: `{"is_secure":false,"port":1}`, want: Connection{Plain: &Plain{IsSecure: new(PlainIsSecureFalse), Port: new(1)}}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var got Connection
			require.NoError(t, json.Unmarshal([]byte(tc.data), &got))
			assert.Equal(t, tc.want, got)
			require.NoError(t, got.Validate())

			out, err := json.Marshal(got)
			require.NoError(t, err)
			assert.JSONEq(t, tc.data, string(out))
		})
	}

	out, err := json.Marshal(Connection{Plain: &Plain{Port: new(1)}})
	require.NoError(t, err)
	assert.JSONEq(t, `{"port":1}`, string(out))
}
