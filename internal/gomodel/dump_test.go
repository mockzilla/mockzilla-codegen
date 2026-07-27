// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package gomodel

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTypeText(t *testing.T) {
	t.Parallel()

	pet := &Decl{Name: "Pet"}
	tests := []struct {
		name string
		t    Type
		want string
	}{
		{name: "Builtin", t: stringType, want: "string"},
		{name: "Declaration", t: DeclRef{Decl: pet}, want: "Pet"},
		{name: "Qualified", t: Qualified{Import: importRuntime, Name: "Date"}, want: "runtime.Date"},
		{name: "Qualified with an alias", t: Qualified{Import: Import{Path: "github.com/google/uuid", Alias: "gid"}, Name: "UUID"}, want: "gid.UUID"},
		{name: "Composite", t: Pointer{Elem: Map{Key: stringType, Elem: Slice{Elem: DeclRef{Decl: pet}}}}, want: "*map[string][]Pet"},
		{name: "No type", want: "-"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, typeText(tt.t))
		})
	}
}
