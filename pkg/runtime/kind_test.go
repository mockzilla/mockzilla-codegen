// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package runtime

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestJSONKind(t *testing.T) {
	t.Parallel()

	tests := []struct {
		data string
		want Kind
	}{
		{data: ` {"a":1}`, want: KindObject},
		{data: `[1]`, want: KindArray},
		{data: `"a"`, want: KindString},
		{data: `true`, want: KindBool},
		{data: `false`, want: KindBool},
		{data: `null`, want: KindNull},
		{data: `-12`, want: KindInteger},
		{data: `1e3`, want: KindNumber},
		{data: `0.5`, want: KindNumber},
		{data: ``},
		{data: `x`},
	}

	for _, tc := range tests {
		t.Run(tc.data, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.want, JSONKind([]byte(tc.data)))
		})
	}
}

func TestKindString(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "integer or number", (KindInteger | KindNumber).String())
	assert.Equal(t, "null or boolean or integer or number or string or array or object", KindAny.String())
	assert.Empty(t, Kind(0).String())
}
