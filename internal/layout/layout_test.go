// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package layout

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mockzilla/mockzilla-codegen/pkg/config"
)

var modelParts = []Part{
	{ID: "models.types", Uses: []PartID{"models.enums"}},
	{ID: "models.enums"},
	{ID: "models.unions"},
	{ID: "models.params", Uses: []PartID{"models.types"}},
}

var workModule = Module{Path: "example.com/work", Dir: "/work"}

func parseConfig(t *testing.T, src, dir string) *config.Config {
	t.Helper()

	cfg, err := config.Parse([]byte(src), dir)
	require.NoError(t, err)
	return cfg
}

func TestPlan(t *testing.T) {
	t.Parallel()

	allParts := []PartID{"models.types", "models.enums", "models.unions", "models.params"}
	tests := []struct {
		name  string
		cfg   string
		mod   Module
		parts []Part
		want  []*File
	}{
		{
			name: "Every part goes to output.file by default",
			cfg:  "output: {file: ./api/gen.go}\n",
			mod:  workModule,
			want: []*File{
				{Path: "/work/api/gen.go", Rel: "./api/gen.go", Package: "api", ImportPath: "example.com/work/api", Parts: allParts},
			},
		},
		{
			name: "Exact part beats its group and output.file is dropped when empty",
			cfg:  "output:\n  file: ./gen.go\n  files:\n    ./models.go: [models]\n    ./enums.go: [models.enums]\n",
			mod:  workModule,
			want: []*File{
				{Path: "/work/enums.go", Rel: "./enums.go", Package: "work", ImportPath: "example.com/work", Parts: []PartID{"models.enums"}},
				{Path: "/work/models.go", Rel: "./models.go", Package: "work", ImportPath: "example.com/work", Parts: []PartID{"models.types", "models.unions", "models.params"}},
			},
		},
		{
			name: "Listing output.file in output.files keeps one file",
			cfg:  "output:\n  file: ./gen.go\n  files:\n    gen.go: [models.enums]\n    ./types.go: [models.types]\n",
			mod:  workModule,
			want: []*File{
				{Path: "/work/gen.go", Rel: "./gen.go", Package: "work", ImportPath: "example.com/work", Parts: []PartID{"models.enums", "models.unions", "models.params"}},
				{Path: "/work/types.go", Rel: "./types.go", Package: "work", ImportPath: "example.com/work", Parts: []PartID{"models.types"}},
			},
		},
		{
			name: "Package names come from package, output.packages, then the folder",
			cfg: "package: pets\noutput:\n  file: ./api/gen.go\n" +
				"  files:\n    ./models/types.go: [models.types]\n    ./my-enums/enums.go: [models.enums]\n" +
				"  packages: {./models: model}\n",
			mod: workModule,
			want: []*File{
				{Path: "/work/api/gen.go", Rel: "./api/gen.go", Package: "pets", ImportPath: "example.com/work/api", Parts: []PartID{"models.unions", "models.params"}},
				{Path: "/work/models/types.go", Rel: "./models/types.go", Package: "model", ImportPath: "example.com/work/models", Parts: []PartID{"models.types"}},
				{Path: "/work/my-enums/enums.go", Rel: "./my-enums/enums.go", Package: "myenums", ImportPath: "example.com/work/my-enums", Parts: []PartID{"models.enums"}},
			},
		},
		{
			name: "output.packages wins over package for the folder of output.file",
			cfg:  "package: pets\noutput:\n  file: ./api/gen.go\n  packages: {./api/: petapi}\n",
			mod:  workModule,
			want: []*File{
				{Path: "/work/api/gen.go", Rel: "./api/gen.go", Package: "petapi", ImportPath: "example.com/work/api", Parts: allParts},
			},
		},
		{
			name: "Scaffold files take their part and main its package",
			cfg: "output: {file: ./api/gen.go}\nserver:\n  framework: chi\n" +
				"  scaffold: {service: ./api/service.go, middleware: ./api/middleware.go, main: ./cmd/server/main.go}\n",
			mod:   workModule,
			parts: []Part{{ID: "models.types"}, {ID: PartScaffoldService}, {ID: PartScaffoldMain, Package: "main"}, {ID: PartScaffoldMiddleware}},
			want: []*File{
				{Path: "/work/api/gen.go", Rel: "./api/gen.go", Package: "api", ImportPath: "example.com/work/api", Parts: []PartID{"models.types"}},
				{Path: "/work/api/middleware.go", Rel: "./api/middleware.go", Package: "api", ImportPath: "example.com/work/api", Kind: Scaffold, Parts: []PartID{PartScaffoldMiddleware}},
				{Path: "/work/api/service.go", Rel: "./api/service.go", Package: "api", ImportPath: "example.com/work/api", Kind: Scaffold, Parts: []PartID{PartScaffoldService}},
				{Path: "/work/cmd/server/main.go", Rel: "./cmd/server/main.go", Package: "main", ImportPath: "example.com/work/cmd/server", Kind: Scaffold, Parts: []PartID{PartScaffoldMain}},
			},
		},
		{
			name: "Extra files take the part named by their path, whatever a selector says",
			cfg: "output:\n  file: ./api/gen.go\n  files: {./api/models.go: [models]}\n" +
				"extra-files: {./wrap/wrap.go: 'var A = 1', models.go: 'var B = 1'}\n",
			mod:   workModule,
			parts: []Part{{ID: "models.types"}, {ID: "./wrap/wrap.go"}, {ID: "models.go"}},
			want: []*File{
				{Path: "/work/api/models.go", Rel: "./api/models.go", Package: "api", ImportPath: "example.com/work/api", Parts: []PartID{"models.types"}},
				{Path: "/work/models.go", Rel: "models.go", Package: "work", ImportPath: "example.com/work", Parts: []PartID{"models.go"}},
				{Path: "/work/wrap/wrap.go", Rel: "./wrap/wrap.go", Package: "wrap", ImportPath: "example.com/work/wrap", Parts: []PartID{"./wrap/wrap.go"}},
			},
		},
		{
			name: "One folder needs no module",
			cfg:  "output: {file: ./api/gen.go}\n",
			want: []*File{
				{Path: "/work/api/gen.go", Rel: "./api/gen.go", Package: "api", Parts: allParts},
			},
		},
		{
			name: "One folder outside the module gets no import path",
			cfg:  "output: {file: ../elsewhere/gen.go}\n",
			mod:  workModule,
			want: []*File{
				{Path: "/elsewhere/gen.go", Rel: "../elsewhere/gen.go", Package: "elsewhere", Parts: allParts},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			parts := modelParts
			if tc.parts != nil {
				parts = tc.parts
			}
			l, err := Plan(parseConfig(t, tc.cfg, "/work"), parts, tc.mod)

			require.NoError(t, err)
			assert.Equal(t, tc.want, l.Files)
			for _, f := range l.Files {
				for _, p := range f.Parts {
					assert.Same(t, f, l.FileOf(p))
				}
			}
			assert.Nil(t, l.FileOf("server.router"))
		})
	}
}

func TestPlanPackage(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		cfg  string
		want string
	}{
		{name: "Package of the config", cfg: "package: pets\noutput: {file: ./api/gen.go}\n", want: "pets"},
		{name: "output.packages wins over package", cfg: "package: pets\noutput: {file: ./api/gen.go, packages: {./api: petapi}}\n", want: "petapi"},
		{name: "output.file that holds no part", cfg: "package: pets\noutput: {file: ./api/gen.go, files: {./models/models.go: [models]}}\n", want: "pets"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			l, err := Plan(parseConfig(t, tc.cfg, "/work"), modelParts, workModule)

			require.NoError(t, err)
			assert.Equal(t, tc.want, l.Package)
		})
	}
}

func TestPlanErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		cfg     *config.Config
		parts   []Part
		dir     string
		mod     Module
		wantErr error
		wantMsg string
	}{
		{
			name: "Equal selectors in two files conflict",
			cfg: &config.Config{Output: config.Output{File: "./gen.go", Files: map[string][]string{
				"./a.go": {"models.types"},
				"./b.go": {"models.types"},
			}}},
			wantErr: ErrSelectorConflict,
			wantMsg: "selector conflict: models.types is selected by both ./a.go and ./b.go",
		},
		{
			name: "Selector that matches no part lists the parts",
			cfg: &config.Config{Output: config.Output{File: "./gen.go", Files: map[string][]string{
				"./server.go": {"server", "models.types"},
				"./client.go": {"client.http"},
			}}},
			wantErr: ErrUnknownSelector,
			wantMsg: `unknown selector "client.http" in ./client.go, "server" in ./server.go; ` +
				"the parts are models.types, models.enums, models.unions, models.params",
		},
		{
			name: "Several folders without a module",
			cfg: &config.Config{Output: config.Output{File: "./gen.go", Files: map[string][]string{
				"./models/types.go": {"models.types"},
			}}},
			wantErr: ErrNoModule,
			wantMsg: "no module path: the output spans 2 folders, which import each other by module path; add a go.mod or set output.module",
		},
		{
			name: "Several folders with one outside the module",
			cfg: &config.Config{Output: config.Output{File: "./gen.go", Files: map[string][]string{
				"../other/types.go": {"models.types"},
			}}},
			mod:     workModule,
			wantErr: ErrOutsideModule,
			wantMsg: "outside the module: /other is not inside /work",
		},
		{
			name: "A main file next to generated code",
			cfg: &config.Config{
				Output: config.Output{File: "./gen.go"},
				Server: &config.Server{Framework: "chi", Scaffold: config.Scaffold{Service: "./service.go", Main: "./main.go"}},
			},
			parts:   []Part{{ID: "models.types"}, {ID: PartScaffoldService}, {ID: PartScaffoldMain, Package: "main"}},
			mod:     workModule,
			wantErr: ErrPackageConflict,
			wantMsg: "two packages in one folder: ./main.go is package main next to ./gen.go, package work",
		},
		{
			name: "A part away from the folder of its owner",
			cfg: &config.Config{Output: config.Output{File: "./gen.go", Files: map[string][]string{
				"./client/ops.go": {"client.operations"},
			}}},
			parts:   []Part{{ID: "client.core"}, {ID: "client.operations", Owner: "client.core"}},
			mod:     workModule,
			wantErr: ErrSplitParts,
			wantMsg: "parts that belong together are in different folders: client.operations adds methods to the types of client.core, so ./client/ops.go must be in the folder of ./gen.go",
		},
		{
			name: "Relative config folder cannot be placed in the module",
			cfg: &config.Config{Output: config.Output{File: "./gen.go", Files: map[string][]string{
				"./models/types.go": {"models.types"},
			}}},
			dir:     "work",
			mod:     workModule,
			wantErr: ErrOutsideModule,
			wantMsg: "outside the module: work is not inside /work",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			dir := tc.dir
			if dir == "" {
				dir = "/work"
			}
			cfg := parseConfig(t, "", dir)
			cfg.Output, cfg.Server = tc.cfg.Output, tc.cfg.Server

			parts := modelParts
			if tc.parts != nil {
				parts = tc.parts
			}
			l, err := Plan(cfg, parts, tc.mod)

			require.ErrorIs(t, err, tc.wantErr)
			require.EqualError(t, err, tc.wantMsg)
			assert.Nil(t, l)
		})
	}
}
