// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package runtime

import (
	"mime/multipart"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type withExtra struct {
	Name  string         `json:"name,omitempty"`
	Extra map[string]int `json:"-"`
}

// tagged stands in for a generated struct with additional properties that reads forms.
type tagged struct {
	Name  string         `json:"name,omitempty"`
	Extra map[string]any `json:"-"`
}

func (g tagged) MarshalJSON() ([]byte, error) {
	type plain tagged
	return MarshalAdditional(plain(g), g.Extra, "name")
}

func (g *tagged) UnmarshalForm(form *multipart.Form) error {
	type plain tagged
	return UnmarshalAdditionalForm(form, (*plain)(g), &g.Extra, "name")
}

func TestMarshalAdditional(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		fields  any
		extra   map[string]int
		want    string
		wantErr bool
	}{
		{
			name:   "Extra keys follow the fields in key order",
			fields: withExtra{Name: "a"},
			extra:  map[string]int{"z": 1, "b": 2},
			want:   `{"name":"a","b":2,"z":1}`,
		},
		{
			name:   "No fields set gives only the extra keys",
			fields: withExtra{},
			extra:  map[string]int{"b": 2},
			want:   `{"b":2}`,
		},
		{
			name:   "No extra keys gives the fields as they are",
			fields: withExtra{Name: "a"},
			want:   `{"name":"a"}`,
		},
		{
			name:   "Extra keys named like a property are left out",
			fields: withExtra{Name: "a"},
			extra:  map[string]int{"name": 1, "b": 2},
			want:   `{"name":"a","b":2}`,
		},
		{
			name:    "Fields that fail to marshal give the error",
			fields:  make(chan int),
			extra:   map[string]int{"b": 2},
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := MarshalAdditional(tc.fields, tc.extra, "name")

			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, string(got))
		})
	}
}

func TestMarshalAdditionalBadValue(t *testing.T) {
	t.Parallel()

	_, err := MarshalAdditional(withExtra{}, map[string]any{"c": make(chan int)})

	require.ErrorIs(t, err, ErrAdditionalProperty)
	assert.ErrorContains(t, err, `invalid additional property "c": json: unsupported type`)
}

func TestUnmarshalAdditional(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		data      string
		extra     map[string]int
		want      withExtra
		wantExtra map[string]int
	}{
		{
			name:      "Unknown keys go to extra",
			data:      `{"name":"a","b":2,"z":1}`,
			want:      withExtra{Name: "a"},
			wantExtra: map[string]int{"b": 2, "z": 1},
		},
		{
			name: "Only known keys leave extra nil",
			data: `{"name":"a"}`,
			want: withExtra{Name: "a"},
		},
		{
			name:      "Entries already in extra are kept",
			data:      `{"b":2}`,
			extra:     map[string]int{"a": 1},
			wantExtra: map[string]int{"a": 1, "b": 2},
		},
		{
			name:      "Null changes nothing",
			data:      `null`,
			extra:     map[string]int{"a": 1},
			wantExtra: map[string]int{"a": 1},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var got withExtra
			extra := tc.extra
			err := UnmarshalAdditional([]byte(tc.data), &got, &extra, "name")

			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
			assert.Equal(t, tc.wantExtra, extra)
		})
	}
}

func TestUnmarshalAdditionalErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		data    string
		fields  any
		wantErr error
		wantMsg string
	}{
		{
			name:    "Fields that fail to decode give the error",
			data:    `{"name":1}`,
			fields:  &withExtra{},
			wantMsg: "json: cannot unmarshal number into Go struct field withExtra.name of type string",
		},
		{
			name:    "Data that is not an object gives the error",
			data:    `[1]`,
			fields:  &[]int{},
			wantMsg: "json: cannot unmarshal array into Go value of type map[string]json.RawMessage",
		},
		{
			name:    "Extra value of the wrong type names its key",
			data:    `{"a":1,"b":"x"}`,
			fields:  &withExtra{},
			wantErr: ErrAdditionalProperty,
			wantMsg: `invalid additional property "b": json: cannot unmarshal string into Go value of type int`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var extra map[string]int
			err := UnmarshalAdditional([]byte(tc.data), tc.fields, &extra)

			if tc.wantErr != nil {
				require.ErrorIs(t, err, tc.wantErr)
			}
			assert.EqualError(t, err, tc.wantMsg)
		})
	}
}

func TestUnmarshalAdditionalForm(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		values    url.Values
		extra     map[string]int
		want      withExtra
		wantExtra map[string]int
	}{
		{
			name:      "Other names go to extra",
			values:    url.Values{"name": {"a"}, "b": {"2"}, "z": {"1"}},
			want:      withExtra{Name: "a"},
			wantExtra: map[string]int{"b": 2, "z": 1},
		},
		{
			name:   "Only known names leave extra nil",
			values: url.Values{"name": {"a"}, "": {"x"}},
			want:   withExtra{Name: "a"},
		},
		{
			name:      "Entries already in extra are kept",
			values:    url.Values{"b": {"2"}},
			extra:     map[string]int{"a": 1},
			wantExtra: map[string]int{"a": 1, "b": 2},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var got withExtra
			extra := tc.extra
			err := UnmarshalAdditionalForm(&multipart.Form{Value: tc.values}, &got, &extra, "name")

			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
			assert.Equal(t, tc.wantExtra, extra)
		})
	}
}

func TestUnmarshalAdditionalFormErrors(t *testing.T) {
	t.Parallel()

	var extra map[string]int
	require.ErrorIs(t, UnmarshalAdditionalForm(&multipart.Form{}, withExtra{}, &extra), ErrParamValue)
	require.ErrorIs(t, UnmarshalAdditionalForm(&multipart.Form{Value: url.Values{"b": {"x"}}}, &withExtra{}, &extra, "name"), ErrAdditionalProperty)
}

func TestUnmarshalAdditionalFormNested(t *testing.T) {
	t.Parallel()

	var got struct {
		Tags *tagged `json:"tags"`
	}
	values := url.Values{"tags[name]": {"a"}, "tags[n]": {"1"}, "tags[list][0][k]": {"v"}, "tags[list][1][k]": {"w"}, "tags[obj][x]": {"y"}, "tags[many]": {"1", "2"}}
	require.NoError(t, fillPointer(&multipart.Form{Value: values}, &got))

	assert.Equal(t, &tagged{Name: "a", Extra: map[string]any{
		"n":    int64(1),
		"list": []any{map[string]any{"k": "v"}, map[string]any{"k": "w"}},
		"obj":  map[string]any{"x": "y"},
		"many": []any{int64(1), int64(2)},
	}}, got.Tags)
}
