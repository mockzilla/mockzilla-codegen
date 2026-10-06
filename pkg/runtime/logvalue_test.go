// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package runtime

import (
	"bytes"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLogValue(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		value any
		want  string
	}{
		{
			name: "Object keeps its key order",
			value: struct {
				B string         `json:"b"`
				A int            `json:"a"`
				F float64        `json:"f"`
				T bool           `json:"t"`
				N *string        `json:"n"`
				L []int          `json:"l"`
				O map[string]int `json:"o"`
			}{B: "x", A: 1, F: 1.5, T: true, L: []int{1, 2}, O: map[string]int{"k": 3}},
			want: `level=INFO msg=m v.b=x v.a=1 v.f=1.5 v.t=true v.n=<nil> v.l="[1 2]" v.o.k=3` + "\n",
		},
		{name: "Plain value", value: "x", want: "level=INFO msg=m v=x\n"},
		{name: "Value without JSON", value: func() {}, want: `level=INFO msg=m v="json: unsupported type: func()"` + "\n"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var b bytes.Buffer
			log := slog.New(slog.NewTextHandler(&b, &slog.HandlerOptions{ReplaceAttr: func(_ []string, a slog.Attr) slog.Attr {
				if a.Key == slog.TimeKey {
					return slog.Attr{}
				}
				return a
			}}))
			log.Info("m", "v", LogValue(tc.value))

			assert.Equal(t, tc.want, b.String())
		})
	}
}
