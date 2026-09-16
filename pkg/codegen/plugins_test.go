// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package codegen

import (
	"cmp"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"text/template"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mockzilla/mockzilla-codegen/internal/gomodel"
	"github.com/mockzilla/mockzilla-codegen/internal/layout"
	"github.com/mockzilla/mockzilla-codegen/pkg/config"
)

// storeSpec has an operation of every shape the API tells apart: routed with a JSON body, with a
// raw body, without a body, without a 2xx response, and a webhook.
const storeSpec = `openapi: 3.1.0
info: {title: store, version: "1"}
paths:
  /pets:
    get:
      operationId: listPets
      summary: List pets
      tags: [pets]
      parameters: [{name: limit, in: query, schema: {type: integer}}]
      responses:
        "200": {description: ok, content: {application/json: {schema: {type: array, items: {$ref: '#/components/schemas/Pet'}}}}}
    post:
      operationId: createPet
      requestBody: {content: {application/json: {schema: {$ref: '#/components/schemas/Pet'}}}}
      responses:
        "201": {description: created, content: {application/octet-stream: {}}}
  /pets/{id}:
    delete:
      operationId: deletePet
      parameters: [{name: id, in: path, required: true, schema: {type: integer}}]
      responses:
        "204": {description: gone}
  /health:
    get:
      operationId: health
      responses:
        default: {description: unknown}
webhooks:
  newPet:
    post:
      operationId: newPet
      responses:
        "200": {description: ok}
components:
  schemas:
    Pet: {type: object, properties: {name: {type: string}}}
`

// storeConfig spreads the output over two packages and writes the service scaffold.
const storeConfig = `package: api
output:
  file: ./api/gen.go
  files: {./api/register.go: [plugin.sample.register], ./models/models.go: [models.types]}
server: {framework: chi, name: Pets, scaffold: {service: ./api/service.go}}
user-context: {owner: platform}
`

const registerTemplate = `// Routes lists the routed operations, {{shout "loudly"}}.
var Routes = []string{
{{- range .Routes}}
	{{quote .}},
{{- end}}
}

// Register mounts the routes on r.
func Register(r {{import "github.com/go-chi/chi/v5"}}.Router, list {{expr .List}}, opts {{expr .Options}}, span {{expr .Span}}) {}
`

const serviceTemplate = `// {{.Name}} is what the sample plugin makes of the service.
type {{.Name}} struct{}
{{range .Operations}}
func (s *{{$.Name}}) {{.Name}}(ctx {{$.Context}}.Context, opts *{{.Options}}) (*{{.Data}}, error) {
	return nil, {{import "errors"}}.ErrUnsupported
}
{{end}}`

var errContribute = errors.New("nothing to add")

// registerData is what the register template runs on.
type registerData struct {
	Routes  []string
	List    TypeRef
	Options TypeRef
	Span    TypeRef
}

// fakePlugin answers with what the test sets and keeps the API it saw.
type fakePlugin struct {
	name       string
	res        Reservations
	contribute func(api *API) (*Contribution, error)

	api *API
}

func (p *fakePlugin) Name() string {
	return p.name
}

func (p *fakePlugin) Reserve() Reservations {
	return p.res
}

func (p *fakePlugin) Contribute(api *API) (*Contribution, error) {
	p.api = api
	if p.contribute == nil {
		return nil, nil
	}
	return p.contribute(api)
}

// samplePlugin adds a field of a type from another package, a register part and a service
// scaffold of its own.
func samplePlugin() *fakePlugin {
	return &fakePlugin{
		name: "sample",
		res: Reservations{
			Idents: []string{"Routes", "Register"},
			RequestOptionFields: []FieldSpec{
				{Name: "GenerateResponse", Type: TypeRef{Name: "func() any"}, Doc: "GenerateResponse makes the body."},
				{Name: "Span", Type: TypeRef{Name: "*Span", Package: "trace", ImportPath: "example.com/trace"}},
			},
		},
		contribute: func(api *API) (*Contribution, error) {
			data := registerData{Options: api.Operations[0].RequestOptions, Span: TypeRef{Name: "*Span", Package: "trace", ImportPath: "example.com/trace"}}
			for _, op := range api.Operations {
				if op.IsRouted {
					data.Routes = append(data.Routes, op.Method+" "+op.Path)
				}
				if op.ID == "ListPets" {
					data.List = op.Success.Body
				}
			}
			return &Contribution{
				Parts:     []PartSource{{Name: "register", Template: registerTemplate, Data: data, Imports: []Import{{Path: "example.com/trace"}}}},
				Scaffolds: map[ScaffoldKind]string{ScaffoldService: serviceTemplate},
				Funcs:     template.FuncMap{"shout": strings.ToUpper},
			}, nil
		},
	}
}

// workDir is a module folder, so output in several folders can import across them.
func workDir(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/work\n"), 0o600))
	return dir
}

// generate runs the store spec with cfg and the plugins.
func generate(t *testing.T, cfg string, plugins ...Plugin) (*Result, error) {
	t.Helper()

	c, err := config.Parse([]byte(cfg), workDir(t))
	require.NoError(t, err)
	return Generate(context.Background(), c, WithSpec([]byte(storeSpec)), WithPlugins(plugins...))
}

func TestGenerateWithPlugin(t *testing.T) {
	t.Parallel()

	p := samplePlugin()
	res, err := generate(t, storeConfig, p)
	require.NoError(t, err)

	inAPI, inModels := TypeRef{Package: "api", ImportPath: "example.com/work/api"}, TypeRef{Package: "models", ImportPath: "example.com/work/models"}
	named := func(base TypeRef, name string) TypeRef {
		base.Name = name
		return base
	}
	assert.Equal(t, &API{
		Package: "api",
		Operations: []Operation{
			{
				ID: "ListPets", Method: "GET", Path: "/pets", Summary: "List pets", Tags: []string{"pets"}, HasOptions: true, IsRouted: true,
				RequestOptions: named(inAPI, "ListPetsServiceRequestOptions"), ResponseData: named(inAPI, "ListPetsResponseData"),
				Success: &Success{Status: 200, ContentType: "application/json", Body: named(inAPI, "ListPetsResponse200")},
			},
			{
				ID: "CreatePet", Method: "POST", Path: "/pets", HasOptions: true, IsRouted: true,
				RequestOptions: named(inAPI, "CreatePetServiceRequestOptions"), ResponseData: named(inAPI, "CreatePetResponseData"),
				Success: &Success{Status: 201, ContentType: "application/octet-stream", Body: TypeRef{Name: "[]byte"}, IsRaw: true},
			},
			{
				ID: "DeletePet", Method: "DELETE", Path: "/pets/{id}", HasOptions: true, IsRouted: true,
				RequestOptions: named(inAPI, "DeletePetServiceRequestOptions"), ResponseData: named(inAPI, "DeletePetResponseData"),
				Success: &Success{Status: 204},
			},
			{
				ID: "Health", Method: "GET", Path: "/health", IsRouted: true,
				RequestOptions: named(inAPI, "HealthServiceRequestOptions"), ResponseData: named(inAPI, "HealthResponseData"),
			},
			{
				ID: "NewPet", Method: "POST", Path: "newPet",
				RequestOptions: named(inAPI, "NewPetServiceRequestOptions"), ResponseData: named(inAPI, "NewPetResponseData"),
				Success: &Success{Status: 200},
			},
		},
		Types:       []TypeRef{named(inModels, "Pet"), named(inAPI, "ListPetsQuery"), named(inAPI, "ListPetsResponse200"), named(inAPI, "DeletePetPathParams")},
		UserContext: map[string]any{"owner": "platform"},
	}, p.api)

	byName := make(map[string]File, len(res.Files))
	for _, f := range res.Files {
		byName[filepath.Base(f.Path)] = f
	}
	assert.Equal(t, "// Code generated by mockzilla-codegen. DO NOT EDIT.\n\npackage api\n\nimport (\n\t\"example.com/trace\"\n\tchi \"github.com/go-chi/chi/v5\"\n)\n\n"+
		"// Routes lists the routed operations, LOUDLY.\nvar Routes = []string{\n\t\"GET /pets\",\n\t\"POST /pets\",\n\t\"DELETE /pets/{id}\",\n\t\"GET /health\",\n}\n\n"+
		"// Register mounts the routes on r.\nfunc Register(r chi.Router, list ListPetsResponse200, opts ListPetsServiceRequestOptions, span *trace.Span) {\n}\n",
		string(byName["register.go"].Content))
	assert.Equal(t, []string{"plugin.sample.register"}, byName["register.go"].Parts)
	assert.Equal(t, FileScaffold, byName["service.go"].Kind)
	assert.Contains(t, string(byName["service.go"].Content), "import (\n\t\"context\"\n\t\"errors\"\n)\n\n// Pets is what the sample plugin makes of the service.\ntype Pets struct{}\n\n"+
		"func (s *Pets) ListPets(ctx context.Context, opts *ListPetsServiceRequestOptions) (*ListPetsResponseData, error) {\n\treturn nil, errors.ErrUnsupported\n}\n")
	assert.Contains(t, string(byName["gen.go"].Content), "\tQuery *ListPetsQuery\n\t// GenerateResponse makes the body.\n\tGenerateResponse func() any\n\tSpan             *trace.Span\n\tRawRequest       *http.Request\n")
}

func TestGenerateWithPluginWithoutServer(t *testing.T) {
	t.Parallel()

	p := &fakePlugin{name: "sample", res: Reservations{Idents: []string{"Pet"}}}
	_, err := generate(t, "output: {file: ./api/gen.go}\n", p)

	require.NoError(t, err)
	assert.Equal(t, Operation{
		ID: "ListPets", Method: "GET", Path: "/pets", Summary: "List pets", Tags: []string{"pets"}, HasOptions: true,
		Success: &Success{Status: 200, ContentType: "application/json", Body: TypeRef{Name: "ListPetsResponse200", Package: "api", ImportPath: "example.com/work/api"}},
	}, p.api.Operations[0])
	assert.Equal(t, TypeRef{Name: "PetSchema", Package: "api", ImportPath: "example.com/work/api"}, p.api.Types[0], "the reserved name is left to the plugin")
}

func TestGenerateWithPluginAndClient(t *testing.T) {
	t.Parallel()

	p := &fakePlugin{name: "sample"}
	_, err := generate(t, "output: {file: ./api/gen.go, files: {./types/types.go: [client.options, client.responses, models]}}\nclient: {with-response: true}\n", p)

	require.NoError(t, err)
	inTypes := TypeRef{Package: "types", ImportPath: "example.com/work/types"}
	assert.Equal(t, TypeRef{Name: "ListPetsRequestOptions", Package: inTypes.Package, ImportPath: inTypes.ImportPath}, p.api.Operations[0].ClientRequestOptions)
	assert.Equal(t, TypeRef{Name: "ListPetsResponse", Package: inTypes.Package, ImportPath: inTypes.ImportPath}, p.api.Operations[0].ClientResponse)
	assert.Equal(t, TypeRef{}, p.api.Operations[4].ClientRequestOptions, "a webhook has no client method")
	assert.Equal(t, TypeRef{}, p.api.Operations[4].ClientResponse)
}

func TestGenerateWithPluginAndPlainClient(t *testing.T) {
	t.Parallel()

	p := &fakePlugin{name: "sample"}
	_, err := generate(t, "output: {file: ./api/gen.go}\nclient:\n", p)

	require.NoError(t, err)
	assert.Equal(t, TypeRef{Name: "ListPetsRequestOptions", Package: "api", ImportPath: "example.com/work/api"}, p.api.Operations[0].ClientRequestOptions)
	assert.Equal(t, TypeRef{}, p.api.Operations[0].ClientResponse, "no envelopes without with-response")
}

func TestGenerateWithPluginErrors(t *testing.T) {
	t.Parallel()

	giving := func(c *Contribution) func(*API) (*Contribution, error) {
		return func(*API) (*Contribution, error) { return c, nil }
	}
	tests := []struct {
		name    string
		cfg     string
		plugins []Plugin
		wantMsg string
	}{
		{
			name:    "Draft layout that fails",
			cfg:     strings.Replace(storeConfig, "scaffold: {service: ./api/service.go}", "scaffold: {service: ./api/service.go, main: ./api/main.go}", 1),
			plugins: []Plugin{&fakePlugin{name: "sample"}},
			wantMsg: "two packages in one folder: ./api/main.go is package main next to ./api/gen.go, package api",
		},
		{
			name:    "Name that is no segment",
			plugins: []Plugin{&fakePlugin{name: "Sample"}},
			wantMsg: `plugin "Sample": the name must match [a-z][a-z0-9]*`,
		},
		{
			name:    "Name used twice",
			plugins: []Plugin{&fakePlugin{name: "sample"}, &fakePlugin{name: "sample"}},
			wantMsg: "plugin sample: the name is used twice",
		},
		{
			name:    "Field that is no exported identifier",
			plugins: []Plugin{&fakePlugin{name: "sample", res: Reservations{RequestOptionFields: []FieldSpec{{Name: "generate", Type: TypeRef{Name: "int"}}}}}},
			wantMsg: `plugin sample: request option field "generate" is no exported identifier`,
		},
		{
			name:    "Field without a type",
			plugins: []Plugin{&fakePlugin{name: "sample", res: Reservations{RequestOptionFields: []FieldSpec{{Name: "Generate"}}}}},
			wantMsg: "plugin sample: request option field Generate has no type",
		},
		{
			name:    "Field the request options declare",
			plugins: []Plugin{&fakePlugin{name: "sample", res: Reservations{RequestOptionFields: []FieldSpec{{Name: "RawRequest", Type: TypeRef{Name: "int"}}}}}},
			wantMsg: "plugin sample: request option field RawRequest is one the request options declare themselves",
		},
		{
			name: "Field added by two plugins",
			plugins: []Plugin{
				&fakePlugin{name: "a", res: Reservations{RequestOptionFields: []FieldSpec{{Name: "Generate", Type: TypeRef{Name: "int"}}}}},
				&fakePlugin{name: "b", res: Reservations{RequestOptionFields: []FieldSpec{{Name: "Generate", Type: TypeRef{Name: "int"}}}}},
			},
			wantMsg: "plugin b: request option field Generate is added twice",
		},
		{
			name:    "Contribute fails",
			plugins: []Plugin{&fakePlugin{name: "sample", contribute: func(*API) (*Contribution, error) { return nil, errContribute }}},
			wantMsg: "plugin sample: nothing to add",
		},
		{
			name:    "Part name that is no segment",
			plugins: []Plugin{&fakePlugin{name: "sample", contribute: giving(&Contribution{Parts: []PartSource{{Name: "register.go"}}})}},
			wantMsg: `plugin sample: the part name "register.go" must match [a-z][a-z0-9]*`,
		},
		{
			name:    "Part contributed twice",
			plugins: []Plugin{&fakePlugin{name: "sample", contribute: giving(&Contribution{Parts: []PartSource{{Name: "register"}, {Name: "register"}}})}},
			wantMsg: "plugin sample: the part register is contributed twice",
		},
		{
			name:    "Scaffold kind that does not exist",
			plugins: []Plugin{&fakePlugin{name: "sample", contribute: giving(&Contribution{Scaffolds: map[ScaffoldKind]string{ScaffoldKind(9): "x"}})}},
			wantMsg: "plugin sample: 9 is no scaffold kind",
		},
		{
			name: "Scaffold replaced by two plugins",
			plugins: []Plugin{
				&fakePlugin{name: "a", contribute: giving(&Contribution{Scaffolds: map[ScaffoldKind]string{ScaffoldService: "x"}})},
				&fakePlugin{name: "b", contribute: giving(&Contribution{Scaffolds: map[ScaffoldKind]string{ScaffoldService: "y"}})},
			},
			wantMsg: "plugin b: the service scaffold is already replaced by a",
		},
		{
			name:    "Part template that does not parse",
			plugins: []Plugin{&fakePlugin{name: "sample", contribute: giving(&Contribution{Parts: []PartSource{{Name: "register", Template: "{{if}}"}}})}},
			wantMsg: "./api/register.go: plugin sample: load templates: plugin.sample.register: template: plugin.sample.register:1: missing value for if",
		},
		{
			name:    "Part that is not placed",
			plugins: []Plugin{&fakePlugin{name: "sample", contribute: giving(&Contribution{Parts: []PartSource{{Name: "other", Template: ""}}})}},
			wantMsg: `unknown selector "plugin.sample.register" in ./api/register.go; the parts are models.types, models.enums, models.unions, models.params, models.bodies, models.responses, ` +
				"server.service, server.errors, server.adapter, server.router, server.scaffold.service, plugin.sample.other",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			res, err := generate(t, cmp.Or(tc.cfg, storeConfig), tc.plugins...)

			require.EqualError(t, err, tc.wantMsg)
			assert.Nil(t, res)
			assert.Equal(t, strings.Contains(tc.wantMsg, "plugin "), errors.Is(err, ErrPlugin))
		})
	}
}

func TestTypeRef(t *testing.T) {
	t.Parallel()

	cfg, err := config.Parse([]byte("output:\n  file: ./api/gen.go\n  files: {./models/types.go: [models.types]}\n"), "/work")
	require.NoError(t, err)
	lay, err := layout.Plan(cfg, []layout.Part{{ID: gomodel.PartTypes}, {ID: gomodel.PartParams}}, layout.Module{Path: "example.com/work", Dir: "/work"})
	require.NoError(t, err)

	pet := &gomodel.Decl{Name: "Pet", Part: gomodel.PartTypes}
	tests := []struct {
		name string
		typ  gomodel.Type
		want TypeRef
	}{
		{name: "Builtin", typ: gomodel.Builtin{Name: "string"}, want: TypeRef{Name: "string"}},
		{name: "Nil type is any", want: TypeRef{Name: "any"}},
		{name: "Declaration takes its file's package", typ: gomodel.Slice{Elem: gomodel.DeclRef{Decl: pet}}, want: TypeRef{Name: "[]Pet", Package: "models", ImportPath: "example.com/work/models"}},
		{name: "Declaration of a part the layout does not know", typ: gomodel.DeclRef{Decl: &gomodel.Decl{Name: "Orphan", Part: "client.types"}}, want: TypeRef{Name: "Orphan"}},
		{
			name: "Qualified type takes its import",
			typ:  gomodel.Pointer{Elem: gomodel.Qualified{Import: gomodel.Import{Path: "github.com/google/uuid", Alias: "guid"}, Name: "UUID"}},
			want: TypeRef{Name: "*UUID", Package: "guid", ImportPath: "github.com/google/uuid"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.want, typeRef(tc.typ, lay))
		})
	}
}
