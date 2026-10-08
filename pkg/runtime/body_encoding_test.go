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

func TestEncodingPartType(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		declared string
		isText   bool
		want     string
		wantErr  string
	}{
		{name: "Nothing declared", isText: true},
		{name: "JSON for any value", declared: "application/json", want: "application/json"},
		{name: "Text for text", declared: "text/plain; charset=utf-8", isText: true, want: "text/plain; charset=utf-8"},
		{name: "The first that fits", declared: "application/xml, application/vnd.a+json", want: "application/vnd.a+json"},
		{name: "No wildcard", declared: "text/*, text/csv", isText: true, want: "text/csv"},
		{name: "Nothing fits", declared: "application/xml, image/*", wantErr: "unsupported content type: p in application/xml, image/*"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := Encoding{"p": {ContentType: tc.declared}}.partType("p", tc.isText)

			if tc.wantErr != "" {
				require.EqualError(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestEncodingFileType(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		declared string
		file     File
		want     string
		wantErr  string
	}{
		{name: "Its own type wins", declared: "application/pdf", file: NewFile(nil, "a", "text/plain"), want: "text/plain"},
		{name: "The one declared", declared: "application/pdf", file: NewFile(nil, "a", ""), want: "application/pdf"},
		{name: "Bytes when nothing is declared", file: NewFile(nil, "a", ""), want: "application/octet-stream"},
		{name: "A list needs its own", declared: "image/png, image/jpeg", file: NewFile(nil, "a", ""), wantErr: "invalid body value: p: the file has no content type; the spec takes image/png, image/jpeg"},
		{name: "A wildcard needs its own", declared: "image/*", file: NewFile(nil, "a", ""), wantErr: "invalid body value: p: the file has no content type; the spec takes image/*"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := Encoding{"p": {ContentType: tc.declared}}.fileType("p", tc.file)

			if tc.wantErr != "" {
				require.EqualError(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestEncodingIsJSON(t *testing.T) {
	t.Parallel()

	enc := Encoding{"a": {ContentType: "application/json"}, "b": {ContentType: "application/json, application/x+json"}, "c": {ContentType: "application/json, text/plain"}, "d": {ContentType: ""}}
	tests := []struct {
		name     string
		property string
		want     bool
	}{
		{name: "One JSON type", property: "a", want: true},
		{name: "Only JSON types", property: "b", want: true},
		{name: "A type that is not JSON", property: "c"},
		{name: "An empty type", property: "d"},
		{name: "Nothing declared", property: "e"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.want, enc.isJSON(tc.property))
		})
	}
}
