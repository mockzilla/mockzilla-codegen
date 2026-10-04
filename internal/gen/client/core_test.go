// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package client

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/mockzilla/mockzilla-codegen/internal/gomodel"
)

func TestCoreView(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		m              *gomodel.Model
		timeout        time.Duration
		hasStreams     bool
		wantTimeout    string
		wantHasStreams bool
	}{
		{name: "A timeout and a Stream method", m: petModel(), timeout: 5 * time.Second, hasStreams: true, wantTimeout: "5 * time.Second", wantHasStreams: true},
		{name: "No timeout is no expression", m: petModel(), hasStreams: true, wantHasStreams: true},
		{name: "Streams turned off", m: petModel(), timeout: 5 * time.Second, wantTimeout: "5 * time.Second"},
		{name: "No operation streams", m: &gomodel.Model{Operations: petModel().Operations[:1]}, timeout: 5 * time.Second, hasStreams: true, wantTimeout: "5 * time.Second"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			opts := allOptions()
			opts.Timeout, opts.HasStreams = tc.timeout, tc.hasStreams
			g, _ := New(tc.m, opts)
			f := fixture{m: tc.m, g: g, cfg: "output: {file: ./gen.go}\n"}
			s := f.scope(t, PartCore)

			got := coreView(f.g, s)

			assert.Len(t, got.Signatures, len(tc.m.Operations))
			got.Signatures = nil
			assert.Equal(t, &CoreView{
				Name:         "PetClient",
				Option:       "PetClientOption",
				Interface:    "PetClientInterface",
				Header:       HeaderView{Name: "PetClientInterface", User: map[string]any{"owner": "platform"}},
				Context:      "context",
				HTTP:         "http",
				Time:         "time",
				URL:          "url",
				Runtime:      "runtime",
				Timeout:      tc.wantTimeout,
				HasStreams:   tc.wantHasStreams,
				HasEnvelopes: true,
				User:         map[string]any{"owner": "platform"},
			}, got)
			assert.True(t, s.Imports.Has("time"))
		})
	}
}

func TestSignatureView(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		op           int
		hasEnvelopes bool
		hasStreams   bool
		want         SignatureView
	}{
		{
			name:         "A body, the spec's text and an envelope",
			op:           0,
			hasEnvelopes: true,
			want:         SignatureView{Name: "ListPets", Route: "GET /pets", Doc: "List pets\n\nReturns pets.", Options: "types.ListPetsRequestOptions", Result: "types.Pets", Response: "types.ListPetsResponse"},
		},
		{
			name: "No body and no envelope",
			op:   2,
			want: SignatureView{Name: "DeletePet", Route: "DELETE /pets/{id}", Doc: "Deprecated: the spec marks it deprecated.", Options: "types.DeletePetRequestOptions"},
		},
		{
			name:       "A stream",
			op:         5,
			hasStreams: true,
			want:       SignatureView{Name: "Chat", Route: "POST /chat", Options: "types.ChatRequestOptions", Result: "*types.Pet", StreamType: "*runtime.Stream[types.ChatResponseItem]"},
		},
		{
			name: "A stream without Stream methods",
			op:   5,
			want: SignatureView{Name: "Chat", Route: "POST /chat", Options: "types.ChatRequestOptions", Result: "*types.Pet"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			m := petModel()
			opts := allOptions()
			opts.HasEnvelopes, opts.HasStreams = tc.hasEnvelopes, tc.hasStreams
			cfg := plainConfig
			if tc.hasEnvelopes {
				cfg = splitConfig
			}
			g, _ := New(m, opts)
			s := fixture{m: m, g: g, cfg: cfg}.scope(t, PartCore)

			got := signatureView(g, m.Operations[tc.op], s)

			assert.Equal(t, tc.want, got)
		})
	}
}
