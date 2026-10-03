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

			assert.Equal(t, &CoreView{
				Name:       "PetClient",
				Option:     "PetClientOption",
				Context:    "context",
				HTTP:       "http",
				Time:       "time",
				URL:        "url",
				Runtime:    "runtime",
				Timeout:    tc.wantTimeout,
				HasStreams: tc.wantHasStreams,
				User:       map[string]any{"owner": "platform"},
			}, got)
			assert.True(t, s.Imports.Has("time"))
		})
	}
}
