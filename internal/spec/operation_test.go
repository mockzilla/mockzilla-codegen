// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package spec

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDeriveOperationID(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		method string
		path   string
		want   string
	}{
		{name: "Path words are capitalized after the method", method: "GET", path: "/users/{userId}/orders", want: "getUsersUserIdOrders"},
		{name: "Root path is the method alone", method: "POST", path: "/", want: "post"},
		{name: "Separators and digits split words", method: "put", path: "/v1/pets.json/2fa-codes", want: "putV1PetsJson2faCodes"},
		{name: "Webhook name works as a path", method: "POST", path: "newPet", want: "postNewPet"},
		{name: "Non-ASCII letters are kept", method: "GET", path: "/über", want: "getÜber"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, DeriveOperationID(tt.method, tt.path))
		})
	}
}

func TestStatusCode(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		status string
		want   int
		wantOK bool
	}{
		{name: "Code", status: "404", want: 404, wantOK: true},
		{name: "Zero is a code", status: "0", wantOK: true},
		{name: "Range takes its start", status: "4XX", want: 400, wantOK: true},
		{name: "Lower case range", status: "2xx", want: 200, wantOK: true},
		{name: "Any three characters after a digit read like a range", status: "20X", want: 200, wantOK: true},
		{name: "Number outside the codes", status: "600", want: 600, wantOK: true},
		{name: "Default", status: "default"},
		{name: "Key that is no status", status: "ok"},
		{name: "Digit and one more character are no range", status: "4X"},
		{name: "Empty key", status: ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, ok := StatusCode(tc.status)

			assert.Equal(t, tc.want, got)
			assert.Equal(t, tc.wantOK, ok)
		})
	}
}
