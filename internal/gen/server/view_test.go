// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package server

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/mockzilla/mockzilla-codegen/internal/gomodel"
	"github.com/mockzilla/mockzilla-codegen/internal/naming"
)

func TestHeaderSuffix(t *testing.T) {
	t.Parallel()

	headers := &gomodel.Decl{Name: "Headers"}
	tests := []struct {
		name      string
		responses []gomodel.Response
		want      string
	}{
		{name: "One response with headers takes no suffix", responses: []gomodel.Response{{Status: "200", Headers: headers}, {Status: "404"}}},
		{name: "Several take their status", responses: []gomodel.Response{{Status: "200", Headers: headers}, {Status: "4XX", Headers: headers}}, want: "4XX"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.want, headerSuffix(naming.New(nil), "4XX", &gomodel.Operation{Responses: tc.responses}))
		})
	}
}

func TestConstructors(t *testing.T) {
	t.Parallel()

	whole := gomodel.Content{MediaType: "application/json", Type: gomodel.Builtin{Name: "string"}}
	events := gomodel.Content{MediaType: "text/event-stream", Item: gomodel.Builtin{Name: "int"}}
	tests := []struct {
		name     string
		response gomodel.Response
		want     []Constructor
	}{
		{name: "No body", response: gomodel.Response{Status: "204"}, want: []Constructor{{Name: "NewOpResponseData"}}},
		{name: "A body read whole", response: gomodel.Response{Status: "200", Contents: []gomodel.Content{whole}}, want: []Constructor{{Name: "NewOpResponseData", Body: whole, HasBody: true}}},
		{name: "Frames alone", response: gomodel.Response{Status: "200", Contents: []gomodel.Content{events}}, want: []Constructor{{Name: "NewOpResponseData", Body: events, HasBody: true, IsStream: true}}},
		{
			name:     "Both, the frames with a suffix",
			response: gomodel.Response{Status: "default", Contents: []gomodel.Content{events, whole}},
			want: []Constructor{
				{Name: "NewOpResponseData", HasStatusArg: true, Body: whole, HasBody: true},
				{Name: "NewOpResponseDataStream", HasStatusArg: true, Body: events, HasBody: true, IsStream: true},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			op := &gomodel.Operation{Name: "Op", Responses: []gomodel.Response{tc.response}}
			assert.Equal(t, tc.want, Constructors(naming.New(nil), op, tc.response))
		})
	}
}
