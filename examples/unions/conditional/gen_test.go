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

func TestEndpointJSON(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		data    string
		want    Endpoint
		wantOut string
	}{
		{name: "Then for true", data: `{"secure":true,"cert":"c"}`, want: Endpoint{Secure: new(true), Then: &EndpointThen{Cert: new("c")}}, wantOut: `{"secure":true,"cert":"c"}`},
		{name: "Else for false", data: `{"secure":false,"port":8080}`, want: Endpoint{Secure: new(false), Else: &EndpointElse{Port: new(8080)}}, wantOut: `{"secure":false,"port":8080}`},
		{name: "Then without the property", data: `{"cert":"c"}`, want: Endpoint{Then: &EndpointThen{Cert: new("c")}}, wantOut: `{"secure":true,"cert":"c"}`},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var got Endpoint
			require.NoError(t, json.Unmarshal([]byte(tc.data), &got))
			assert.Equal(t, tc.want, got)

			out, err := json.Marshal(got)
			require.NoError(t, err)
			assert.JSONEq(t, tc.wantOut, string(out))
		})
	}
}

func TestEndpointElseNeedsFalse(t *testing.T) {
	t.Parallel()

	built := Endpoint{Else: &EndpointElse{Port: new(8080)}}
	require.EqualError(t, built.Validate(), "secure: an empty value picks Then, not Else")

	built.Secure = new(false)
	require.NoError(t, built.Validate())
}

func TestRetryJSON(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		data string
		want Retry
	}{
		{name: "Then for the tested number", data: `{"attempts":3,"backoff":"slow"}`, want: Retry{Attempts: new(3), Then: &RetryThen{Backoff: new("slow")}}},
		{name: "Else for another number", data: `{"attempts":2,"delay":5}`, want: Retry{Attempts: new(2), Else: &RetryElse{Delay: new(5)}}},
		{name: "Else without the property the if requires", data: `{"delay":5}`, want: Retry{Else: &RetryElse{Delay: new(5)}}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var got Retry
			require.NoError(t, json.Unmarshal([]byte(tc.data), &got))
			assert.Equal(t, tc.want, got)

			out, err := json.Marshal(got)
			require.NoError(t, err)
			assert.JSONEq(t, tc.data, string(out))
		})
	}
}

func TestRetryFillsTheNumber(t *testing.T) {
	t.Parallel()

	out, err := json.Marshal(Retry{Then: &RetryThen{Backoff: new("slow")}})

	require.NoError(t, err)
	assert.JSONEq(t, `{"attempts":3,"backoff":"slow"}`, string(out))
}
