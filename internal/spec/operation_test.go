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
