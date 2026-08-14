// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package runtime

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMarshalUnion(t *testing.T) {
	t.Parallel()

	shared := struct {
		ID int `json:"id"`
	}{ID: 1}

	tests := []struct {
		name    string
		shared  any
		set     []any
		want    string
		wantErr bool
	}{
		{name: "Nothing set is null", want: `null`},
		{name: "Shared alone", shared: shared, want: `{"id":1}`},
		{name: "One variant", set: []any{&cat{Name: "a"}}, want: `{"name":"a","meow":false}`},
		{name: "Shared and a variant merge", shared: shared, set: []any{&dog{Name: "a"}}, want: `{"id":1,"name":"a","bark":false}`},
		{name: "Several objects merge", set: []any{&cat{Name: "a"}, &dog{Name: "b"}}, want: `{"name":"b","meow":false,"bark":false}`},
		{name: "A part that is no object gives the first variant", shared: shared, set: []any{new("x"), &cat{}}, want: `"x"`},
		{name: "Variant that fails to marshal", set: []any{func() {}}, wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := MarshalUnion(tc.shared, tc.set...)

			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.JSONEq(t, tc.want, string(got))
			assert.Equal(t, tc.want, string(got))
		})
	}
}

func TestMergeObjects(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		parts   []string
		want    string
		wantErr error
	}{
		{name: "Keys keep their first place and last value", parts: []string{`{"a":1,"b":2}`, `{"c":3,"a":4}`}, want: `{"a":4,"b":2,"c":3}`},
		{name: "No parts", want: `{}`},
		{name: "Values are kept as written", parts: []string{`{"a": [1, 2]}`}, want: `{"a":[1, 2]}`},
		{name: "Part that is no object", parts: []string{`{}`, `[1]`}, wantErr: ErrNotObject},
		{name: "Empty part", parts: []string{``}, wantErr: ErrNotObject},
		{name: "Broken key", parts: []string{`{1:2}`}},
		{name: "Broken value", parts: []string{`{"a":}`}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			parts := make([][]byte, len(tc.parts))
			for i, p := range tc.parts {
				parts[i] = []byte(p)
			}

			got, err := MergeObjects(parts...)

			if tc.want == "" {
				require.Error(t, err)
				if tc.wantErr != nil {
					require.ErrorIs(t, err, tc.wantErr)
				}
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, string(got))
		})
	}
}
