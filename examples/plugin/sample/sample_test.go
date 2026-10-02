// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package sample_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mockzilla/mockzilla-codegen/examples/plugin/sample"
	"github.com/mockzilla/mockzilla-codegen/pkg/codegen"
	"github.com/mockzilla/mockzilla-codegen/pkg/config"
)

// TestExamples compares the basic example with a fresh run with the plugin. UPDATE=1 writes the
// run instead. The generator's own example test leaves the plugin examples out, since the plugin
// lives in this module.
func TestExamples(t *testing.T) {
	t.Parallel()

	cfg, err := config.Load(filepath.Join("..", "basic", "codegen.yaml"))
	require.NoError(t, err)
	res, err := codegen.Generate(context.Background(), cfg, codegen.WithPlugins(sample.Plugin{}))
	require.NoError(t, err)

	if os.Getenv("UPDATE") != "" {
		_, err = codegen.Write(res, codegen.WriteOptions{OverwriteScaffolds: true})
		require.NoError(t, err)
		return
	}
	for _, f := range res.Files {
		want, readErr := os.ReadFile(f.Path)
		require.NoError(t, readErr, "run make examples")
		assert.Equal(t, string(want), string(f.Content), f.Path)
	}
}

func TestExamplesAreDeterministic(t *testing.T) {
	t.Parallel()

	cfg, err := config.Load(filepath.Join("..", "basic", "codegen.yaml"))
	require.NoError(t, err)

	var first *codegen.Result
	for range 3 {
		res, genErr := codegen.Generate(context.Background(), cfg, codegen.WithPlugins(sample.Plugin{}))
		require.NoError(t, genErr)
		if first == nil {
			first = res
			continue
		}
		assert.Equal(t, first, res)
	}
}

func TestGenerateWithoutServer(t *testing.T) {
	t.Parallel()

	spec, err := filepath.Abs(filepath.Join("..", "basic", "api.yaml"))
	require.NoError(t, err)
	cfg, err := config.Parse([]byte("spec: {path: "+spec+"}\npackage: basic\noutput: {file: ./gen.go}\nclient:\n"), t.TempDir())
	require.NoError(t, err)

	res, err := codegen.Generate(context.Background(), cfg, codegen.WithPlugins(sample.Plugin{}))

	require.NoError(t, err)
	require.Len(t, res.Files, 1)
	assert.Equal(t, []string{
		"models.types", "models.enums", "models.unions", "models.params", "models.bodies", "models.responses",
		"client.core", "client.options", "client.operations", "plugin.sample.register",
	}, res.Files[0].Parts, "no service to wrap")
}
