// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package bench

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/mockzilla/mockzilla-codegen/internal/prepare"
	"github.com/mockzilla/mockzilla-codegen/internal/provider"
	"github.com/mockzilla/mockzilla-codegen/internal/provider/libopenapi"
	"github.com/mockzilla/mockzilla-codegen/pkg/config"
)

const specsDir = "../../testdata/specs"

var largeSpecs = []string{
	"3.0/microsoft.com/graph.1.0.1.yml",
	"3.0/misc/slope.yml",
	"3.0/github.com/api.github.com.1.1.4.yml",
}

// BenchmarkPrepare runs Prepare with pruning on, as generation does.
func BenchmarkPrepare(b *testing.B) {
	p := libopenapi.New()
	cfg, parseErr := config.Parse([]byte("{}"), "")
	if parseErr != nil {
		b.Fatal(parseErr)
	}
	for _, name := range largeSpecs {
		b.Run(name, func(b *testing.B) {
			data := read(b, name)
			for b.Loop() {
				if _, err := prepare.Run(context.Background(), p, prepare.Input{Spec: data, Path: name, Config: cfg}); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkParse(b *testing.B) {
	p := libopenapi.New()
	for _, name := range largeSpecs {
		b.Run(name, func(b *testing.B) {
			data := read(b, name)
			for b.Loop() {
				if _, _, err := p.Parse(context.Background(), data, provider.ParseOptions{File: name}); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func read(b *testing.B, name string) []byte {
	b.Helper()

	data, err := os.ReadFile(filepath.Join(specsDir, name))
	if err != nil {
		b.Skip("spec not in testdata/specs: " + name)
	}
	return data
}
