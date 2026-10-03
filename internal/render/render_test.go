// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package render

import (
	"io/fs"
	"slices"
	"strings"
	"testing"
	"testing/fstest"
	"text/template"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mockzilla/mockzilla-codegen/internal/gocode"
	"github.com/mockzilla/mockzilla-codegen/internal/layout"
	"github.com/mockzilla/mockzilla-codegen/pkg/config"
)

type failFS struct{}

func (failFS) Open(string) (fs.File, error) {
	return nil, fs.ErrPermission
}

// fakeSet has a part with a block before its types and one after each type, and a part that
// reads a key of its data.
func fakeSet() Set {
	return Set{
		Name: "fake",
		FS: fstest.MapFS{
			"part.tmpl": {Data: []byte(`{{with override "fake.header" .}}{{.}}` + "\n\n" + `{{end}}` +
				`{{range .}}type {{.}} struct{}{{override "fake.extra" .}}` + "\n" + `{{end}}`)},
			"owner.tmpl": {Data: []byte(`// Owned by {{.owner}}.{{override "fake.extra" .}}`)},
			"other.tmpl": {Data: []byte(`{{.Missing}}`)},
			"README.md":  {Data: []byte("not a template {{")},
		},
		Parts:  map[layout.PartID]string{"fake.types": "part.tmpl", "fake.owner": "owner.tmpl", "fake.broken": "other.tmpl"},
		Blocks: []string{"fake.extra", "fake.header"},
	}
}

func TestRenderPart(t *testing.T) {
	t.Parallel()

	const kinds = "type Pet struct{}\nfunc (pet) Kind() string { return \"Pet\" }\ntype Tag struct{}\nfunc (tag) Kind() string { return \"Tag\" }\n"
	tests := []struct {
		name      string
		templates map[string]string
		want      string
	}{
		{
			name: "Default block",
			want: "type Pet struct{}\ntype Tag struct{}\n",
		},
		{
			name:      "Overridden block",
			templates: map[string]string{"fake.extra": "func ({{lower .}}) Kind() string { return {{quote .}} }"},
			want:      kinds,
		},
		{
			name:      "Blank lines around the text are left out",
			templates: map[string]string{"fake.extra": " \n\nfunc ({{lower .}}) Kind() string { return {{quote .}} }\t\n \n"},
			want:      kinds,
		},
		{
			name:      "First line of the text keeps its indent",
			templates: map[string]string{"fake.extra": "\n\t// A type.\n\t// Named {{.}}.\n"},
			want:      "type Pet struct{}\n\t// A type.\n\t// Named Pet.\ntype Tag struct{}\n\t// A type.\n\t// Named Tag.\n",
		},
		{
			name:      "Blank lines an action writes are left out",
			templates: map[string]string{"fake.extra": `{{printf "\n\n// A %s.\n\n" (lower .)}}`},
			want:      "type Pet struct{}\n// A pet.\ntype Tag struct{}\n// A tag.\n",
		},
		{
			name:      "Override that writes nothing for one type",
			templates: map[string]string{"fake.extra": "{{if eq . \"Tag\"}}// The last one.{{end}}"},
			want:      "type Pet struct{}\ntype Tag struct{}\n// The last one.\n",
		},
		{
			name:      "Override of blank space alone",
			templates: map[string]string{"fake.extra": " \n\t", "fake.header": "\n"},
			want:      "type Pet struct{}\ntype Tag struct{}\n",
		},
		{
			name:      "Block that is followed by a blank line",
			templates: map[string]string{"fake.header": "// {{len .}} types.\n"},
			want:      "\n// 2 types.\n\ntype Pet struct{}\ntype Tag struct{}\n",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			e, err := New([]Set{fakeSet()}, Options{Templates: tc.templates})
			require.NoError(t, err)

			got, err := e.RenderPart("fake.types", []string{"Pet", "Tag"})
			require.NoError(t, err)
			assert.Equal(t, tc.want, string(got))
		})
	}
}

func TestRenderPartErrors(t *testing.T) {
	t.Parallel()

	e, err := New([]Set{fakeSet()}, Options{Templates: map[string]string{
		"fake.extra":  "\n// Run by {{.team}}.",
		"fake.header": `{{template "fake/part.tmpl" .}}`,
	}})
	require.NoError(t, err)

	tests := []struct {
		name    string
		part    layout.PartID
		data    any
		wantErr error
		wantMsg string
	}{
		{
			name:    "Part without a template",
			part:    "fake.enums",
			wantErr: ErrUnknownPart,
			wantMsg: "no template for part: fake.enums",
		},
		{
			name:    "Field the data does not have",
			part:    "fake.broken",
			data:    []string{},
			wantErr: ErrExecute,
			wantMsg: `render: template: fake/other.tmpl:1:2: executing "fake/other.tmpl" at <.Missing>: can't evaluate field Missing in type []string`,
		},
		{
			name:    "Key the data does not have",
			part:    "fake.owner",
			data:    map[string]any{"team": "core"},
			wantErr: ErrExecute,
			wantMsg: `render: template: fake/owner.tmpl:1:14: executing "fake/owner.tmpl" at <.owner>: map has no entry for key "owner"`,
		},
		{
			name:    "Key the data does not have, in an override",
			part:    "fake.owner",
			data:    map[string]any{"owner": "platform"},
			wantErr: ErrExecute,
			wantMsg: `render: template: fake.extra:2:12: executing "fake.extra" at <.team>: map has no entry for key "team"`,
		},
		{
			name:    "Key without a value, in an override",
			part:    "fake.owner",
			data:    map[string]any{"owner": "platform", "team": nil},
			wantErr: ErrNoValue,
			wantMsg: "render: fake.extra: a value that is not set was written as <no value>",
		},
		{
			name:    "Override that runs the template that asks for it",
			part:    "fake.types",
			data:    []string{"Pet"},
			wantErr: ErrExecute,
			wantMsg: `render: template: fake.header:1:11: executing "fake.header" at <{{template "fake/part.tmpl" .}}>: template "fake/part.tmpl" not defined`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, renderErr := e.RenderPart(tc.part, tc.data)

			require.ErrorIs(t, renderErr, tc.wantErr)
			require.EqualError(t, renderErr, tc.wantMsg)
			assert.Nil(t, got)
		})
	}
}

func TestRenderPartWithKeys(t *testing.T) {
	t.Parallel()

	e, err := New([]Set{fakeSet()}, Options{Templates: map[string]string{"fake.extra": `// Run by {{.team}}{{if index . "since"}} since {{.since}}{{end}}.`}})
	require.NoError(t, err)

	tests := []struct {
		name string
		data map[string]any
		want string
	}{
		{
			name: "Every key is set",
			data: map[string]any{"owner": "platform", "team": "core", "since": 2024},
			want: "// Owned by platform.\n// Run by core since 2024.",
		},
		{
			name: "Key that is not set is asked for with index",
			data: map[string]any{"owner": "platform", "team": "core"},
			want: "// Owned by platform.\n// Run by core.",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, renderErr := e.RenderPart("fake.owner", tc.data)

			require.NoError(t, renderErr)
			assert.Equal(t, tc.want, string(got))
		})
	}
}

func TestRenderSource(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		src     Source
		data    any
		want    string
		wantErr error
		wantMsg string
	}{
		{
			name: "Own funcs next to the engine's",
			src:  Source{Name: "plugin.sample.register", Text: `{{shout .}} {{quote .}}`, Funcs: template.FuncMap{"shout": strings.ToUpper}},
			data: "Pet",
			want: `PET "Pet"`,
		},
		{
			name: "Own func in place of the engine's",
			src:  Source{Name: "plugin.sample.register", Text: `{{quote .}} {{lower .}}`, Funcs: template.FuncMap{"quote": strings.ToUpper}},
			data: "Pet",
			want: "PET pet",
		},
		{
			name: "Key of the data",
			src:  Source{Name: "plugin.sample.register", Text: `{{.owner}}{{if index . "team"}} and {{.team}}{{end}}`},
			data: map[string]any{"owner": "platform"},
			want: "platform",
		},
		{
			name:    "Text that does not parse",
			src:     Source{Name: "plugin.sample.register", Text: `{{if}}`},
			data:    "Pet",
			wantErr: ErrTemplate,
			wantMsg: "load templates: plugin.sample.register: template: plugin.sample.register:1: missing value for if",
		},
		{
			name:    "Text that fails to run",
			src:     Source{Name: "plugin.sample.register", Text: `{{.Missing}}`},
			data:    "Pet",
			wantErr: ErrExecute,
			wantMsg: `render: template: plugin.sample.register:1:2: executing "plugin.sample.register" at <.Missing>: can't evaluate field Missing in type string`,
		},
		{
			name:    "Key the data does not have",
			src:     Source{Name: "plugin.sample.register", Text: `// Owned by {{.owner}}.`},
			data:    map[string]any{"team": "core"},
			wantErr: ErrExecute,
			wantMsg: `render: template: plugin.sample.register:1:14: executing "plugin.sample.register" at <.owner>: map has no entry for key "owner"`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := RenderSource(tc.src, tc.data)

			if tc.wantErr != nil {
				require.ErrorIs(t, err, tc.wantErr)
				assert.EqualError(t, err, tc.wantMsg)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, string(got))
		})
	}
}

func TestNewOverrideErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		sets      []Set
		templates map[string]string
		needs     map[string]string
		want      []config.Issue
	}{
		{
			name:      "Unknown blocks list the known ones",
			sets:      []Set{fakeSet()},
			templates: map[string]string{"models.struct": "x", "fake.extra": "ok"},
			want:      []config.Issue{{Key: "templates.models.struct", Message: "unknown block; the blocks are fake.extra, fake.header"}},
		},
		{
			name:      "Unknown block when there is none",
			templates: map[string]string{"fake.extra": "x"},
			want:      []config.Issue{{Key: "templates.fake.extra", Message: "unknown block; there is no block to override"}},
		},
		{
			name:      "Block of a generator the config leaves out",
			sets:      []Set{fakeSet()},
			templates: map[string]string{"server.router-extra": "x", "fake.extra": "ok"},
			needs:     map[string]string{"server.router-extra": "a server block", "server.service-header": "a server block"},
			want:      []config.Issue{{Key: "templates.server.router-extra", Message: "needs a server block"}},
		},
		{
			name:      "Unknown blocks list those of a generator the config leaves out",
			sets:      []Set{fakeSet()},
			templates: map[string]string{"models.struct": "x"},
			needs:     map[string]string{"server.router-extra": "a server block", "server.service-header": "a server block"},
			want: []config.Issue{{
				Key:     "templates.models.struct",
				Message: "unknown block; the blocks are fake.extra, fake.header, server.router-extra, server.service-header",
			}},
		},
		{
			name:      "Override that does not parse",
			sets:      []Set{fakeSet()},
			templates: map[string]string{"fake.extra": "{{.Name"},
			want:      []config.Issue{{Key: "templates.fake.extra", Message: "template: fake.extra:1: unclosed action"}},
		},
		{
			name:      "Override that defines another template",
			sets:      []Set{fakeSet()},
			templates: map[string]string{"fake.extra": `{{define "fake/part.tmpl"}}{{end}}`},
			want:      []config.Issue{{Key: "templates.fake.extra", Message: `defines "fake/part.tmpl"; an override replaces its own block only`}},
		},
		{
			name:      "Override that asks for another block",
			sets:      []Set{fakeSet()},
			templates: map[string]string{"fake.extra": `{{override "fake.header" .}}`},
			want:      []config.Issue{{Key: "templates.fake.extra", Message: `template: fake.extra:1: function "override" not defined`}},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			_, err := New(tc.sets, Options{Templates: tc.templates, Needs: tc.needs})

			require.ErrorIs(t, err, config.ErrInvalid)
			assert.Equal(t, &config.ValidationError{Issues: tc.want}, err)
		})
	}
}

func TestNewLoadErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		set     Set
		wantMsg string
	}{
		{
			name:    "Unreadable folder",
			set:     Set{Name: "fake", FS: failFS{}},
			wantMsg: "load templates: fake: permission denied",
		},
		{
			name:    "Unreadable template",
			set:     Set{Name: "fake", FS: fstest.MapFS{"dir.tmpl/x": {}}},
			wantMsg: "load templates: fake/dir.tmpl: read dir.tmpl: invalid argument",
		},
		{
			name:    "Template that does not parse",
			set:     Set{Name: "fake", FS: fstest.MapFS{"bad.tmpl": {Data: []byte("{{if}}")}}},
			wantMsg: "load templates: fake/bad.tmpl: template: fake/bad.tmpl:1: missing value for if",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			_, err := New([]Set{tc.set}, Options{})

			require.ErrorIs(t, err, ErrTemplate)
			assert.EqualError(t, err, tc.wantMsg)
		})
	}
}

func TestRenderFile(t *testing.T) {
	t.Parallel()

	full := FileData{
		Header:  "Code generated by test. DO NOT EDIT.",
		Package: "api",
		Imports: "import \"github.com/mockzilla/mockzilla-codegen/pkg/runtime\"",
		Guard:   "runtime.SupportsGeneratorV2",
		Parts:   []string{"\ntype Day = runtime.Date\n", "", "\ntype Pet struct {\nName string\n}\n"},
	}
	tests := []struct {
		name     string
		data     FileData
		isFormat bool
		want     string
	}{
		{
			name:     "Everything, formatted",
			data:     full,
			isFormat: true,
			want: "// Code generated by test. DO NOT EDIT.\n\npackage api\n\n" +
				"import \"github.com/mockzilla/mockzilla-codegen/pkg/runtime\"\n\n" +
				"// Fails to compile when the runtime package does not match the mockzilla-codegen version that\n" +
				"// wrote this file.\n" +
				"const _ = runtime.SupportsGeneratorV2\n\n" +
				"type Day = runtime.Date\n\ntype Pet struct {\n\tName string\n}\n",
		},
		{
			name: "Everything, not formatted",
			data: full,
			want: "// Code generated by test. DO NOT EDIT.\n\npackage api\n\n" +
				"import \"github.com/mockzilla/mockzilla-codegen/pkg/runtime\"\n\n" +
				"// Fails to compile when the runtime package does not match the mockzilla-codegen version that\n" +
				"// wrote this file.\n" +
				"const _ = runtime.SupportsGeneratorV2\n\n" +
				"type Day = runtime.Date\n\ntype Pet struct {\nName string\n}\n",
		},
		{
			name:     "Package clause only",
			data:     FileData{Package: "api"},
			isFormat: true,
			want:     "package api\n",
		},
		{
			name:     "Comment of a part stays off the last line of the part before it",
			data:     FileData{Package: "api", Parts: []string{"const (\n\tOne = 1\n)", "// Pet is a pet.\ntype Pet struct{}"}},
			isFormat: true,
			want:     "package api\n\nconst (\n\tOne = 1\n)\n\n// Pet is a pet.\ntype Pet struct{}\n",
		},
		{
			name:     "Part stays out of the comment that ends the part before it",
			data:     FileData{Package: "api", Parts: []string{"var One = 1 // The first.", "var Two = 2"}},
			isFormat: true,
			want:     "package api\n\nvar One = 1 // The first.\n\nvar Two = 2\n",
		},
		{
			name: "Any line breaks around a part make one blank line, not formatted",
			data: FileData{Package: "api", Parts: []string{"type A int", "\n\n\n// B follows A.\ntype B int\n\n\n", " \n\t\n", "\t// C keeps its indent.\n\ttype C int\t\n"}},
			want: "package api\n\ntype A int\n\n// B follows A.\ntype B int\n\n\t// C keeps its indent.\n\ttype C int\n",
		},
		{
			name: "Parts without text, not formatted",
			data: FileData{Package: "api", Imports: "import \"embed\"", Parts: []string{"", "\n"}},
			want: "package api\n\nimport \"embed\"\n",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			e, err := New(nil, Options{Format: tc.isFormat})
			require.NoError(t, err)
			parts := slices.Clone(tc.data.Parts)

			got, err := e.RenderFile(tc.data)
			require.NoError(t, err)
			assert.Equal(t, tc.want, string(got))
			assert.Equal(t, parts, tc.data.Parts, "the parts of the caller stay as they are")
		})
	}
}

func TestRenderFileFormatError(t *testing.T) {
	t.Parallel()

	e, err := New(nil, Options{Format: true})
	require.NoError(t, err)

	_, err = e.RenderFile(FileData{Package: "api", Parts: []string{"type Pet struct {"}})

	require.ErrorIs(t, err, gocode.ErrFormat)
}
