// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package codegen

import (
	"cmp"
	"context"
	"errors"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"text/template"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mockzilla/mockzilla-codegen/internal/gen/server"
	"github.com/mockzilla/mockzilla-codegen/internal/gomodel"
	"github.com/mockzilla/mockzilla-codegen/internal/layout"
	"github.com/mockzilla/mockzilla-codegen/pkg/config"
)

// storeSpec has an operation of every shape the API tells apart: routed with a JSON body, with a
// raw body and a second response, without a body, without a 2xx response, one the router drops,
// and a webhook.
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
        default: {description: taken, content: {application/json: {schema: {$ref: '#/components/schemas/Pet'}}}}
        "201": {description: created, content: {application/octet-stream: {}}}
  /pets/{id}:
    delete:
      operationId: deletePet
      parameters: [{name: id, in: path, required: true, schema: {type: integer}}]
      responses:
        "204": {description: gone}
  /pets/{petId}:
    delete:
      operationId: removePet
      parameters: [{name: petId, in: path, required: true, schema: {type: integer}}]
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

// ownedConfig has a user-context value of every shape YAML gives one: a text, a map, a list with
// a map in it and a map with a key that is no text. The header block writes the text.
const ownedConfig = `package: api
output: {file: ./api/gen.go}
server: {framework: chi}
templates: {server.service-header: "// Owned by {{.User.owner}}.\n"}
user-context:
  owner: platform
  team: {name: core}
  tiers: [free, {name: pro}]
  ports: {8080: http}
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

// rawConfig is storeConfig with the formatting of the output turned off.
var rawConfig = strings.Replace(storeConfig, "output:\n", "output:\n  format: false\n", 1)

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

// samplePlugin adds a register part, a service scaffold of its own and fields to three operations:
// one that returns the response data of its operation, to a routed one, a dropped one and a
// webhook, and one of a type from another package, to the first alone.
func samplePlugin() *fakePlugin {
	return &fakePlugin{
		name: "sample",
		res:  Reservations{Idents: []string{"Routes", "Register"}},
		contribute: func(api *API) (*Contribution, error) {
			span := TypeRef{Name: "*Span", Package: "trace", ImportPath: "example.com/trace"}
			data := registerData{Options: api.Operations[0].RequestOptions, Span: span}
			fields := make(map[string][]FieldSpec)
			for _, op := range api.Operations {
				if op.IsRouted {
					data.Routes = append(data.Routes, op.Method+" "+op.Path)
				}
				if op.ID == "ListPets" {
					data.List = op.Success.Body
				}
				if slices.Contains([]string{"ListPets", "RemovePet", "NewPet"}, op.ID) {
					fields[op.ID] = []FieldSpec{{
						Name: "GenerateResponse",
						Type: TypeRef{Name: "func() (*" + op.ResponseData.Name + ", error)"},
						Doc:  "GenerateResponse makes the response.",
					}}
				}
			}
			fields["ListPets"] = append(fields["ListPets"], FieldSpec{Name: "Span", Type: span})
			return &Contribution{
				Parts:               []PartSource{{Name: "register", Template: registerTemplate, Data: data, Imports: []Import{{Path: "example.com/trace"}}}},
				RequestOptionFields: fields,
				Scaffolds:           map[ScaffoldKind]string{ScaffoldService: serviceTemplate},
				Funcs:               template.FuncMap{"shout": strings.ToUpper},
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
	listPets := []Response{{
		Status: "200", Code: 200, ContentType: "application/json", Body: named(inAPI, "ListPetsResponse200"),
		Constructor: named(inAPI, "NewListPetsResponseData"),
	}}
	createPet := []Response{
		{
			Status: "201", Code: 201, ContentType: "application/octet-stream", Body: TypeRef{Name: "[]byte"}, IsRaw: true,
			Constructor: named(inAPI, "NewCreatePetResponseData201"),
		},
		{
			Status: "default", ContentType: "application/json", Body: named(inModels, "*Pet"),
			Constructor: named(inAPI, "NewCreatePetResponseDataDefault"), HasStatusArg: true,
		},
	}
	deletePet := []Response{{Status: "204", Code: 204, Constructor: named(inAPI, "NewDeletePetResponseData")}}
	removePet := []Response{{Status: "204", Code: 204, Constructor: named(inAPI, "NewRemovePetResponseData")}}
	health := []Response{{Status: "default", Constructor: named(inAPI, "NewHealthResponseData"), HasStatusArg: true}}
	newPet := []Response{{Status: "200", Code: 200, Constructor: named(inAPI, "NewNewPetResponseData")}}
	assert.Equal(t, &API{
		Package: "api",
		Service: named(inAPI, "PetsInterface"),
		Operations: []Operation{
			{
				ID: "ListPets", Method: "GET", Path: "/pets", Summary: "List pets", Tags: []string{"pets"}, HasOptions: true, IsRouted: true,
				RequestOptions: named(inAPI, "ListPetsServiceRequestOptions"), ResponseData: named(inAPI, "ListPetsResponseData"),
				Responses: listPets, Success: &listPets[0],
			},
			{
				ID: "CreatePet", Method: "POST", Path: "/pets", HasOptions: true, IsRouted: true,
				RequestOptions: named(inAPI, "CreatePetServiceRequestOptions"), ResponseData: named(inAPI, "CreatePetResponseData"),
				Responses: createPet, Success: &createPet[0],
			},
			{
				ID: "DeletePet", Method: "DELETE", Path: "/pets/{id}", HasOptions: true, IsRouted: true,
				RequestOptions: named(inAPI, "DeletePetServiceRequestOptions"), ResponseData: named(inAPI, "DeletePetResponseData"),
				Responses: deletePet, Success: &deletePet[0],
			},
			{
				ID: "RemovePet", Method: "DELETE", Path: "/pets/{petId}", HasOptions: true,
				RequestOptions: named(inAPI, "RemovePetServiceRequestOptions"), ResponseData: named(inAPI, "RemovePetResponseData"),
				Responses: removePet, Success: &removePet[0],
			},
			{
				ID: "Health", Method: "GET", Path: "/health", IsRouted: true,
				RequestOptions: named(inAPI, "HealthServiceRequestOptions"), ResponseData: named(inAPI, "HealthResponseData"),
				Responses: health,
			},
			{
				ID: "NewPet", Method: "POST", Path: "newPet",
				RequestOptions: named(inAPI, "NewPetServiceRequestOptions"), ResponseData: named(inAPI, "NewPetResponseData"),
				Responses: newPet, Success: &newPet[0],
			},
		},
		Types: []TypeRef{
			named(inModels, "Pet"), named(inAPI, "ListPetsQuery"), named(inAPI, "ListPetsResponse200"),
			named(inAPI, "DeletePetPathParams"), named(inAPI, "RemovePetPathParams"),
		},
		UserContext: map[string]any{"owner": "platform"},
	}, p.api)
	assert.Same(t, &p.api.Operations[1].Responses[0], p.api.Operations[1].Success, "Success points into Responses")

	byName := make(map[string]File, len(res.Files))
	for _, f := range res.Files {
		byName[filepath.Base(f.Path)] = f
	}
	gen := string(byName["gen.go"].Content)
	for _, op := range p.api.Operations {
		for _, r := range op.Responses {
			assert.Contains(t, gen, "\nfunc "+r.Constructor.Name+"(", "the constructor of %s %s", op.ID, r.Status)
		}
	}
	assert.Contains(t, gen, "\nfunc NewCreatePetResponseDataDefault(status int, body *models.Pet) *CreatePetResponseData {\n")
	assert.Contains(t, gen, "\nfunc NewHealthResponseData(status int) *HealthResponseData {\n")

	assert.Equal(t, "// Code generated by mockzilla-codegen. DO NOT EDIT.\n\npackage api\n\nimport (\n\t\"example.com/trace\"\n\tchi \"github.com/go-chi/chi/v5\"\n)\n\n"+
		"// Routes lists the routed operations, LOUDLY.\nvar Routes = []string{\n\t\"GET /pets\",\n\t\"POST /pets\",\n\t\"DELETE /pets/{id}\",\n\t\"GET /health\",\n}\n\n"+
		"// Register mounts the routes on r.\nfunc Register(r chi.Router, list ListPetsResponse200, opts ListPetsServiceRequestOptions, span *trace.Span) {\n}\n",
		string(byName["register.go"].Content))
	assert.Equal(t, []string{"plugin.sample.register"}, byName["register.go"].Parts)
	assert.Equal(t, FileScaffold, byName["service.go"].Kind)
	assert.Contains(t, string(byName["service.go"].Content), "import (\n\t\"context\"\n\t\"errors\"\n)\n\n// Pets is what the sample plugin makes of the service.\ntype Pets struct{}\n\n"+
		"func (s *Pets) ListPets(ctx context.Context, opts *ListPetsServiceRequestOptions) (*ListPetsResponseData, error) {\n\treturn nil, errors.ErrUnsupported\n}\n")
	assert.Contains(t, gen, "\tQuery *ListPetsQuery\n\t// GenerateResponse makes the response.\n\tGenerateResponse func() (*ListPetsResponseData, error)\n"+
		"\tSpan             *trace.Span\n\tRawRequest       *http.Request\n}\n")
	assert.Contains(t, gen, "type DeletePetServiceRequestOptions struct {\n\tPathParams *DeletePetPathParams\n\tRawRequest *http.Request\n}\n", "an operation without a key gets no field")
	assert.Contains(t, gen, "\tPathParams *RemovePetPathParams\n\t// GenerateResponse makes the response.\n\tGenerateResponse func() (*RemovePetResponseData, error)\n\tRawRequest       *http.Request\n}\n")
	assert.Contains(t, gen, "type NewPetServiceRequestOptions struct {\n\t// GenerateResponse makes the response.\n\tGenerateResponse func() (*NewPetResponseData, error)\n\tRawRequest       *http.Request\n}\n")
}

func TestGenerateWithPluginWithoutServer(t *testing.T) {
	t.Parallel()

	p := &fakePlugin{
		name: "sample",
		res:  Reservations{Idents: []string{"Pet"}},
		contribute: func(*API) (*Contribution, error) {
			return &Contribution{
				Parts:               []PartSource{{Name: "limit", Template: "var Limit = 8"}},
				RequestOptionFields: map[string][]FieldSpec{"ListPets": {{Name: "Tenant", Type: TypeRef{Name: "string"}}}},
				Scaffolds:           map[ScaffoldKind]string{ScaffoldService: serviceTemplate},
			}, nil
		},
	}
	res, err := generate(t, "output: {file: ./api/gen.go}\nclient:\n", p)

	require.NoError(t, err)
	listPets := []Response{{Status: "200", Code: 200, ContentType: "application/json", Body: TypeRef{Name: "ListPetsResponse200", Package: "api", ImportPath: "example.com/work/api"}}}
	assert.Equal(t, Operation{
		ID: "ListPets", Method: "GET", Path: "/pets", Summary: "List pets", Tags: []string{"pets"}, HasOptions: true,
		ClientRequestOptions: TypeRef{Name: "ListPetsRequestOptions", Package: "api", ImportPath: "example.com/work/api"},
		Responses:            listPets, Success: &listPets[0],
	}, p.api.Operations[0], "no constructor without a server")
	assert.Equal(t, []Response{{Status: "default"}}, p.api.Operations[4].Responses, "nor a status argument")
	assert.Equal(t, TypeRef{Name: "PetSchema", Package: "api", ImportPath: "example.com/work/api"}, p.api.Types[0], "the reserved name is left to the plugin")
	require.Len(t, res.Files, 1, "no scaffold file to replace")
	assert.Equal(t, []string{
		"models.types", "models.enums", "models.unions", "models.params", "models.bodies", "models.responses",
		"client.core", "client.options", "client.operations", "plugin.sample.limit",
	}, res.Files[0].Parts)
	assert.Contains(t, string(res.Files[0].Content), "type ListPetsRequestOptions struct {\n\tQuery *ListPetsQuery\n}\n", "the field is the server's, the client's options do not get it")
}

func TestGenerateWithPluginAndClient(t *testing.T) {
	t.Parallel()

	p := &fakePlugin{name: "sample"}
	_, err := generate(t, "output: {file: ./api/gen.go, files: {./types/types.go: [client.options, client.responses, models]}}\nclient: {with-response: true}\n", p)

	require.NoError(t, err)
	inTypes := TypeRef{Package: "types", ImportPath: "example.com/work/types"}
	assert.Equal(t, TypeRef{Name: "ListPetsRequestOptions", Package: inTypes.Package, ImportPath: inTypes.ImportPath}, p.api.Operations[0].ClientRequestOptions)
	assert.Equal(t, TypeRef{Name: "ListPetsResponse", Package: inTypes.Package, ImportPath: inTypes.ImportPath}, p.api.Operations[0].ClientResponse)
	assert.Equal(t, TypeRef{}, p.api.Operations[5].ClientRequestOptions, "a webhook has no client method")
	assert.Equal(t, TypeRef{}, p.api.Operations[5].ClientResponse)
}

func TestGenerateWithPluginAndPlainClient(t *testing.T) {
	t.Parallel()

	p := &fakePlugin{name: "sample"}
	_, err := generate(t, "output: {file: ./api/gen.go}\nclient:\n", p)

	require.NoError(t, err)
	assert.Equal(t, TypeRef{Name: "ListPetsRequestOptions", Package: "api", ImportPath: "example.com/work/api"}, p.api.Operations[0].ClientRequestOptions)
	assert.Equal(t, TypeRef{}, p.api.Operations[0].ClientResponse, "no envelopes without with-response")
}

func TestGenerateWithPluginOutsideModule(t *testing.T) {
	t.Parallel()

	p := &fakePlugin{name: "sample", contribute: func(api *API) (*Contribution, error) {
		return &Contribution{
			Parts:               []PartSource{{Name: "register", Template: "\nvar Options {{expr .}}\n", Data: api.Operations[0].RequestOptions}},
			RequestOptionFields: map[string][]FieldSpec{"ListPets": {{Name: "Latest", Type: TypeRef{Name: api.Operations[0].Success.Body.Name}}}},
		}, nil
	}}
	cfg, err := config.Parse([]byte("package: api\noutput: {file: ./gen.go}\nserver: {framework: chi}\n"), t.TempDir())
	require.NoError(t, err)
	res, err := Generate(context.Background(), cfg, WithSpec([]byte(storeSpec)), WithPlugins(p))

	require.NoError(t, err)
	assert.Equal(t, TypeRef{Name: "ListPetsServiceRequestOptions", Package: "api"}, p.api.Operations[0].RequestOptions, "no import path without a module")
	require.Len(t, res.Files, 1)
	assert.Contains(t, string(res.Files[0].Content), "\nvar Options ListPetsServiceRequestOptions\n")
	assert.Contains(t, string(res.Files[0].Content), "\tQuery      *ListPetsQuery\n\tLatest     ListPetsResponse200\n\tRawRequest *http.Request\n", "a field takes the name alone of a type of the API")
}

func TestGenerateWithPluginThatChangesItsAPI(t *testing.T) {
	t.Parallel()

	dir := workDir(t)
	run := func(plugins ...Plugin) (*config.Config, *Result) {
		t.Helper()

		c, err := config.Parse([]byte(ownedConfig), dir)
		require.NoError(t, err)
		made, err := Generate(context.Background(), c, WithSpec([]byte(storeSpec)), WithPlugins(plugins...))
		require.NoError(t, err)
		return c, made
	}
	alone := &fakePlugin{name: "reader"}
	wantCfg, want := run(alone)

	writer := &fakePlugin{name: "writer", contribute: func(api *API) (*Contribution, error) {
		api.Package = "changed"
		api.Service.Name = "Changed"
		api.Operations[0].Tags[0] = "changed"
		api.Operations[0].Success.Status = "500"
		api.Operations[1].Responses[1].Constructor.Name = "Changed"
		api.Operations[1].ID = "Changed"
		api.Operations = api.Operations[:2]
		api.Types[0].Name = "Changed"
		api.UserContext["owner"] = "changed"
		api.UserContext["team"].(map[string]any)["name"] = "changed"
		api.UserContext["tiers"].([]any)[0] = "changed"
		api.UserContext["tiers"].([]any)[1].(map[string]any)["name"] = "changed"
		api.UserContext["ports"].(map[any]any)[8080] = "changed"
		return nil, nil
	}}
	reader := &fakePlugin{name: "reader"}
	cfg, res := run(writer, reader)

	assert.Equal(t, alone.api, reader.api, "the next plugin sees none of it")
	assert.Equal(t, wantCfg.UserContext, cfg.UserContext, "nor does the config")
	assert.Equal(t, want, res, "nor do the templates")
}

func TestGenerateWithPluginPackage(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		cfg  string
		want string
	}{
		{name: "Package of the config", cfg: "package: pets\noutput: {file: ./api/gen.go}\n", want: "pets"},
		{name: "Package that output.packages gives the folder of output.file", cfg: "package: pets\noutput: {file: ./api/gen.go, packages: {./api: petapi}}\n", want: "petapi"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			p := &fakePlugin{name: "sample"}
			_, err := generate(t, tc.cfg, p)

			require.NoError(t, err)
			assert.Equal(t, tc.want, p.api.Package)
		})
	}
}

func TestGenerateWithPluginService(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		cfg             string
		want            TypeRef
		wantConstructor TypeRef
	}{
		{
			name:            "Interface named after the server, in the package of output.file",
			cfg:             "package: pets\noutput: {file: ./api/gen.go}\nserver: {framework: chi, name: Pets}\n",
			want:            TypeRef{Name: "PetsInterface", Package: "pets", ImportPath: "example.com/work/api"},
			wantConstructor: TypeRef{Name: "NewListPetsResponseData", Package: "pets", ImportPath: "example.com/work/api"},
		},
		{
			name:            "Interface of a server without a name",
			cfg:             "package: pets\noutput: {file: ./api/gen.go}\nserver: {framework: chi}\n",
			want:            TypeRef{Name: "ServiceInterface", Package: "pets", ImportPath: "example.com/work/api"},
			wantConstructor: TypeRef{Name: "NewListPetsResponseData", Package: "pets", ImportPath: "example.com/work/api"},
		},
		{
			name:            "Interface in the folder the config moves the service part to",
			cfg:             "package: pets\noutput: {file: ./api/gen.go, files: {./contract/service.go: [server.service, models]}}\nserver: {framework: chi, name: Pets}\n",
			want:            TypeRef{Name: "PetsInterface", Package: "contract", ImportPath: "example.com/work/contract"},
			wantConstructor: TypeRef{Name: "NewListPetsResponseData", Package: "contract", ImportPath: "example.com/work/contract"},
		},
		{
			name: "No interface without a server",
			cfg:  "package: pets\noutput: {file: ./api/gen.go}\nclient:\n",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			p := &fakePlugin{name: "sample"}
			_, err := generate(t, tc.cfg, p)

			require.NoError(t, err)
			assert.Equal(t, tc.want, p.api.Service)
			assert.Equal(t, tc.wantConstructor, p.api.Operations[0].Responses[0].Constructor, "next to the interface")
		})
	}
}

func TestGenerateWithPluginResponses(t *testing.T) {
	t.Parallel()

	constructor := func(name string) TypeRef {
		return TypeRef{Name: name, Package: "api", ImportPath: "example.com/work/api"}
	}
	tests := []struct {
		name        string
		keys        []string
		want        []Response
		wantSuccess int
	}{
		{
			name: "Codes in the order of the generated code",
			keys: []string{"201", "200"},
			want: []Response{
				{Status: "200", Code: 200, Constructor: constructor("NewPingResponseData200")},
				{Status: "201", Code: 201, Constructor: constructor("NewPingResponseData201")},
			},
		},
		{
			name: "Range after the codes takes the status",
			keys: []string{"2XX", "404"},
			want: []Response{
				{Status: "404", Code: 404, Constructor: constructor("NewPingResponseData404")},
				{Status: "2XX", Code: 200, Constructor: constructor("NewPingResponseData2XX"), HasStatusArg: true},
			},
			wantSuccess: 1,
		},
		{
			name: "Range as the spec writes it",
			keys: []string{"2xx"},
			want: []Response{{Status: "2xx", Code: 200, Constructor: constructor("NewPingResponseData"), HasStatusArg: true}},
		},
		{
			name:        "Key that is no status names no code",
			keys:        []string{"ok"},
			want:        []Response{{Status: "ok", Constructor: constructor("NewPingResponseData"), HasStatusArg: true}},
			wantSuccess: -1,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			doc := "openapi: 3.1.0\ninfo: {title: ping, version: \"1\"}\npaths:\n  /ping:\n    get:\n      operationId: ping\n      responses:\n"
			for _, key := range tc.keys {
				doc += "        " + strconv.Quote(key) + ": {description: answer}\n"
			}
			cfg, err := config.Parse([]byte("package: api\noutput: {file: ./api/gen.go}\nserver: {framework: chi}\n"), workDir(t))
			require.NoError(t, err)
			p := &fakePlugin{name: "sample"}

			_, err = Generate(context.Background(), cfg, WithSpec([]byte(doc)), WithPlugins(p))

			require.NoError(t, err)
			op := p.api.Operations[0]
			assert.Equal(t, tc.want, op.Responses)
			if tc.wantSuccess < 0 {
				assert.Nil(t, op.Success)
				return
			}
			assert.Same(t, &op.Responses[tc.wantSuccess], op.Success)
		})
	}
}

func TestGenerateWithPluginFields(t *testing.T) {
	t.Parallel()

	shared := []FieldSpec{{Name: "Tenant", Type: TypeRef{Name: "string"}}}
	adding := func(fields map[string][]FieldSpec) func(*API) (*Contribution, error) {
		return func(*API) (*Contribution, error) { return &Contribution{RequestOptionFields: fields}, nil }
	}
	tests := []struct {
		name    string
		plugins []Plugin
		want    []string
	}{
		{
			name: "Fields of two plugins on one operation, in the order of the plugins",
			plugins: []Plugin{
				&fakePlugin{name: "b", contribute: adding(map[string][]FieldSpec{
					"Health": {{Name: "Tenant", Type: TypeRef{Name: "string"}}, {Name: "Region", Type: TypeRef{Name: "string"}}},
				})},
				&fakePlugin{name: "a", contribute: adding(map[string][]FieldSpec{"Health": {{Name: "Account", Type: TypeRef{Name: "int"}}}})},
			},
			want: []string{"type HealthServiceRequestOptions struct {\n\tTenant     string\n\tRegion     string\n\tAccount    int\n\tRawRequest *http.Request\n}\n"},
		},
		{
			name: "One name on two operations, each with a type of its own",
			plugins: []Plugin{
				&fakePlugin{name: "a", contribute: adding(map[string][]FieldSpec{"Health": {{Name: "Tenant", Type: TypeRef{Name: "string"}}}})},
				&fakePlugin{name: "b", contribute: adding(map[string][]FieldSpec{"NewPet": {{Name: "Tenant", Type: TypeRef{Name: "int"}}}})},
			},
			want: []string{
				"type HealthServiceRequestOptions struct {\n\tTenant     string\n\tRawRequest *http.Request\n}\n",
				"type NewPetServiceRequestOptions struct {\n\tTenant     int\n\tRawRequest *http.Request\n}\n",
			},
		},
		{
			name: "Fields changed after the plugin gave them",
			plugins: []Plugin{
				&fakePlugin{name: "sample", contribute: adding(map[string][]FieldSpec{"Health": shared})},
				&fakePlugin{name: "other", contribute: func(*API) (*Contribution, error) {
					shared[0] = FieldSpec{Name: "no name"}
					return nil, nil
				}},
			},
			want: []string{"type HealthServiceRequestOptions struct {\n\tTenant     string\n\tRawRequest *http.Request\n}\n"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			res, err := generate(t, "output: {file: ./api/gen.go}\nserver: {framework: chi}\n", tc.plugins...)

			require.NoError(t, err)
			require.Len(t, res.Files, 1)
			for _, want := range tc.want {
				assert.Contains(t, string(res.Files[0].Content), want)
			}
		})
	}
}

func TestGenerateWithPluginFuncs(t *testing.T) {
	t.Parallel()

	own := func(s string) string { return strconv.Quote("own " + s) }
	shared := template.FuncMap{"shout": strings.ToUpper}
	withPart := func(text string, funcs template.FuncMap) func(*API) (*Contribution, error) {
		return func(*API) (*Contribution, error) {
			return &Contribution{Parts: []PartSource{{Name: "register", Template: text}}, Funcs: funcs}, nil
		}
	}
	tests := []struct {
		name    string
		plugins []Plugin
		want    string
	}{
		{
			name: "Func of the plugin replaces the generator's of that name",
			plugins: []Plugin{&fakePlugin{
				name: "sample",
				contribute: withPart(
					"var Names = []string{ {{quote `a`}}, {{expr `b`}}, {{import `c`}}, {{symbol `d`}} }\n",
					template.FuncMap{"quote": own, "expr": own, "import": own, "symbol": own},
				),
			}},
			want: "var Names = []string{\"own a\", \"own b\", \"own c\", \"own d\"}\n",
		},
		{
			name: "Func map changed after the plugin gave it",
			plugins: []Plugin{
				&fakePlugin{name: "sample", contribute: withPart("var Name = {{quote (shout `a`)}}\n", shared)},
				&fakePlugin{name: "other", contribute: func(*API) (*Contribution, error) {
					shared["shout"] = "no func"
					return nil, nil
				}},
			},
			want: "var Name = \"A\"\n",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			res, err := generate(t, storeConfig, tc.plugins...)

			require.NoError(t, err)
			i := slices.IndexFunc(res.Files, func(f File) bool { return filepath.Base(f.Path) == "register.go" })
			require.GreaterOrEqual(t, i, 0)
			assert.Equal(t, "// Code generated by mockzilla-codegen. DO NOT EDIT.\n\npackage api\n\n"+tc.want, string(res.Files[i].Content))
		})
	}
}

func TestGenerateWithPluginParts(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		cfg   string
		parts []string
		want  string
	}{
		{
			name:  "Comment of a part stays off the last line of the part before it",
			parts: []string{"var Limit = 8", "// Names lists the pets.\nvar Names []string"},
			want:  "var Limit = 8\n\n// Names lists the pets.\nvar Names []string\n",
		},
		{
			name:  "Part stays out of the comment that ends the part before it",
			parts: []string{"var Limit = 8 // The most pets.", "var Names []string"},
			want:  "var Limit = 8 // The most pets.\n\nvar Names []string\n",
		},
		{
			name:  "Part without text between two others",
			parts: []string{"var Limit = 8\n", "{{if .}}var Skipped bool{{end}}\n", "var Names []string\n"},
			want:  "var Limit = 8\n\nvar Names []string\n",
		},
		{
			name:  "Any line breaks around a part make one blank line, not formatted",
			cfg:   rawConfig,
			parts: []string{"var Limit = 8", "\n\n\nvar Names []string\n\n\n", "\n", "func Count() int { return len(Names) }"},
			want:  "var Limit = 8\n\nvar Names []string\n\nfunc Count() int { return len(Names) }\n",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			p := &fakePlugin{name: "sample", contribute: func(*API) (*Contribution, error) {
				c := &Contribution{}
				for i, text := range tc.parts {
					c.Parts = append(c.Parts, PartSource{Name: "part" + strconv.Itoa(i), Template: text})
				}
				return c, nil
			}}
			res, err := generate(t, strings.Replace(cmp.Or(tc.cfg, storeConfig), "[plugin.sample.register]", "[plugin.sample]", 1), p)

			require.NoError(t, err)
			i := slices.IndexFunc(res.Files, func(f File) bool { return filepath.Base(f.Path) == "register.go" })
			require.GreaterOrEqual(t, i, 0)
			assert.Equal(t, "// Code generated by mockzilla-codegen. DO NOT EDIT.\n\npackage api\n\n"+tc.want, string(res.Files[i].Content))
		})
	}
}

func TestGenerateWithPluginPlacement(t *testing.T) {
	t.Parallel()

	builtIn := []string{
		"models.types", "models.enums", "models.unions", "models.params", "models.bodies", "models.responses",
		"server.service", "server.errors", "server.adapter", "server.router",
	}
	adding := func(parts ...PartSource) func(*API) (*Contribution, error) {
		return func(*API) (*Contribution, error) { return &Contribution{Parts: parts}, nil }
	}
	tests := []struct {
		name  string
		files string
		want  map[string][]string
	}{
		{
			name:  "Part that no selector names goes to output.file, below the built-in parts",
			files: "{}",
			want:  map[string][]string{"gen.go": slices.Concat(builtIn, []string{"plugin.sample.register", "plugin.sample.names", "plugin.other.names"})},
		},
		{
			name:  "Selector plugin takes the parts of every plugin",
			files: "{./api/plugins.go: [plugin]}",
			want:  map[string][]string{"gen.go": builtIn, "plugins.go": {"plugin.sample.register", "plugin.sample.names", "plugin.other.names"}},
		},
		{
			name:  "Selector of a plugin takes its parts alone",
			files: "{./api/sample.go: [plugin.sample]}",
			want: map[string][]string{
				"gen.go":    slices.Concat(builtIn, []string{"plugin.other.names"}),
				"sample.go": {"plugin.sample.register", "plugin.sample.names"},
			},
		},
		{
			name:  "Selector of a part wins over the one of every plugin",
			files: "{./api/plugins.go: [plugin], ./api/register.go: [plugin.sample.register]}",
			want: map[string][]string{
				"gen.go":      builtIn,
				"plugins.go":  {"plugin.sample.names", "plugin.other.names"},
				"register.go": {"plugin.sample.register"},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			sample := &fakePlugin{name: "sample", contribute: adding(
				PartSource{Name: "register", Template: "func Register() {}"},
				PartSource{Name: "names", Template: "var Names []string"},
			)}
			other := &fakePlugin{name: "other", contribute: adding(PartSource{Name: "names", Template: "var Others []string"})}
			res, err := generate(t, "package: api\noutput: {file: ./api/gen.go, files: "+tc.files+"}\nserver: {framework: chi}\n", sample, other)

			require.NoError(t, err)
			got := make(map[string][]string, len(res.Files))
			for _, f := range res.Files {
				got[filepath.Base(f.Path)] = f.Parts
			}
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestGenerateWithPluginImports(t *testing.T) {
	t.Parallel()

	shared := []Import{{Path: "embed", Alias: "_"}}
	tests := []struct {
		name   string
		parts  []PartSource
		others []Plugin
		want   string
	}{
		{
			name: "Imports under _ and . next to named ones",
			parts: []PartSource{{
				Name:     "register",
				Template: "\nvar Mux = {{import `net/http`}}.NewServeMux()\n\nvar Profiles = {{import `net/http/pprof`}}.Handler\n",
				Imports:  []Import{{Path: "net/http/pprof", Alias: "_"}, {Path: "embed", Alias: "_"}, {Path: "example.com/b", Alias: "."}, {Path: "example.com/a", Alias: "."}},
			}},
			want: "import (\n\t_ \"embed\"\n\t\"net/http\"\n\t\"net/http/pprof\"\n\n\t. \"example.com/a\"\n\t. \"example.com/b\"\n)\n\n" +
				"var Mux = http.NewServeMux()\n\nvar Profiles = pprof.Handler\n",
		},
		{
			name: "Import a part lists stays when only another part names it",
			parts: []PartSource{
				{Name: "register", Template: "\nvar Name = \"pets\"\n", Imports: []Import{{Path: "strings"}}},
				{Name: "loud", Template: "\nvar Loud = strings.ToUpper(Name)\n"},
			},
			want: "import \"strings\"\n\nvar Name = \"pets\"\n\nvar Loud = strings.ToUpper(Name)\n",
		},
		{
			name:  "Imports changed after the plugin gave them",
			parts: []PartSource{{Name: "register", Template: "\nvar Name = \"pets\"\n", Imports: shared}},
			others: []Plugin{&fakePlugin{name: "other", contribute: func(*API) (*Contribution, error) {
				shared[0] = Import{Path: "net/http/pprof", Alias: "no name"}
				return nil, nil
			}}},
			want: "import _ \"embed\"\n\nvar Name = \"pets\"\n",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			p := &fakePlugin{name: "sample", contribute: func(*API) (*Contribution, error) { return &Contribution{Parts: tc.parts}, nil }}
			res, err := generate(t, strings.Replace(storeConfig, "[plugin.sample.register]", "[plugin.sample]", 1), slices.Concat([]Plugin{p}, tc.others)...)

			require.NoError(t, err)
			i := slices.IndexFunc(res.Files, func(f File) bool { return filepath.Base(f.Path) == "register.go" })
			require.GreaterOrEqual(t, i, 0)
			assert.Equal(t, "// Code generated by mockzilla-codegen. DO NOT EDIT.\n\npackage api\n\n"+tc.want, string(res.Files[i].Content))
		})
	}
}

func TestGenerateWithPluginSymbol(t *testing.T) {
	t.Parallel()

	const apart = "package: api\noutput: {file: ./api/gen.go, files: {./app/register.go: [plugin.sample.register], ./models/models.go: [models.types]}}\n" +
		"server: {framework: chi}\n"
	tests := []struct {
		name string
		cfg  string
		text string
		want string
	}{
		{
			name: "Name of a part in the folder of the file, bare",
			cfg:  storeConfig,
			text: "\nvar New = {{symbol `server.router` `NewRouter`}}\n",
			want: "package api\n\nvar New = NewRouter\n",
		},
		{
			name: "Name of a part in another folder, with its import",
			text: "\nvar New = {{symbol `server.router` `NewRouter`}}\n",
			want: "package app\n\nimport \"example.com/work/api\"\n\nvar New = api.NewRouter\n",
		},
		{
			name: "Name of a part in a folder with a package name of its own",
			cfg:  strings.Replace(apart, "files:", "packages: {./api: petapi}, files:", 1),
			text: "\nvar New = {{symbol `server.router` `NewRouter`}}\n",
			want: "package app\n\nimport petapi \"example.com/work/api\"\n\nvar New = petapi.NewRouter\n",
		},
		{
			name: "Name of a part whose package name another import of the file holds",
			text: "\nvar Get = {{import `example.com/other/api`}}.Get\n\nvar New = {{symbol `server.router` `NewRouter`}}\n",
			want: "package app\n\nimport (\n\t\"example.com/other/api\"\n\tapi2 \"example.com/work/api\"\n)\n\nvar Get = api.Get\n\nvar New = api2.NewRouter\n",
		},
		{
			name: "Type of a model part",
			text: "\nvar Pets []{{symbol `models.types` `Pet`}}\n",
			want: "package app\n\nimport \"example.com/work/models\"\n\nvar Pets []models.Pet\n",
		},
		{
			name: "Name of another part of the plugin, in another folder",
			text: "\nvar All = {{symbol `plugin.sample.names` `Names`}}\n",
			want: "package app\n\nimport \"example.com/work/api\"\n\nvar All = api.Names\n",
		},
		{
			name: "Unexported name of another part of the plugin, in the folder of the file",
			cfg:  storeConfig,
			text: "\nvar All = {{symbol `plugin.sample.names` `names`}}\n",
			want: "package api\n\nvar All = names\n",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			p := &fakePlugin{name: "sample", contribute: func(*API) (*Contribution, error) {
				return &Contribution{Parts: []PartSource{
					{Name: "register", Template: tc.text},
					{Name: "names", Template: "\nvar Names, names []string\n"},
				}}, nil
			}}
			res, err := generate(t, cmp.Or(tc.cfg, apart), p)

			require.NoError(t, err)
			i := slices.IndexFunc(res.Files, func(f File) bool { return filepath.Base(f.Path) == "register.go" })
			require.GreaterOrEqual(t, i, 0)
			assert.Equal(t, "// Code generated by mockzilla-codegen. DO NOT EDIT.\n\n"+tc.want, string(res.Files[i].Content))
		})
	}
}

func TestGenerateWithReplacedScaffold(t *testing.T) {
	t.Parallel()

	const apart = "package: api\noutput: {file: ./api/gen.go}\n" +
		"server: {framework: chi, name: Pets, scaffold: {service: ./app/service.go, middleware: ./app/middleware.go, main: ./cmd/main.go}}\n"
	tests := []struct {
		name string
		cfg  string
		kind ScaffoldKind
		text string
		want string
	}{
		{
			name: "Service that names no package of the view",
			kind: ScaffoldService,
			text: "\n// {{.Name}} serves {{len .Operations}} operations.\ntype {{.Name}} struct{}\n",
			want: "package app\n\n// Pets serves 6 operations.\ntype Pets struct{}\n",
		},
		{
			name: "Service that names one package of the view and one of its own",
			kind: ScaffoldService,
			text: "\nvar _ {{.Interface}} = (*{{.Name}})(nil)\n\nvar ErrNone = {{import `fmt`}}.Errorf(\"none\")\n\ntype {{.Name}} struct{ {{.Interface}} }\n",
			want: "package app\n\nimport (\n\t\"fmt\"\n\n\t\"example.com/work/api\"\n)\n\n" +
				"var _ api.PetsInterface = (*Pets)(nil)\n\nvar ErrNone = fmt.Errorf(\"none\")\n\ntype Pets struct{ api.PetsInterface }\n",
		},
		{
			name: "Service with a local that has the name of a package of the view",
			kind: ScaffoldService,
			text: "\ntype {{.Name}} struct{ errors []string }\n\nfunc (s {{.Name}}) Count() int {\n\tcontext := s\n\treturn len(context.errors)\n}\n",
			want: "package app\n\ntype Pets struct{ errors []string }\n\nfunc (s Pets) Count() int {\n\tcontext := s\n\treturn len(context.errors)\n}\n",
		},
		{
			name: "Service in a folder the service imports, which the built-in one cannot be in",
			cfg:  strings.Replace(storeConfig, "service: ./api/service.go", "service: ./models/service.go", 1),
			kind: ScaffoldService,
			text: "\n// {{.Name}} serves {{len .Operations}} operations.\ntype {{.Name}} struct{}\n",
			want: "package models\n\n// Pets serves 6 operations.\ntype Pets struct{}\n",
		},
		{
			name: "Middleware that names one package of five",
			kind: ScaffoldMiddleware,
			text: "\nfunc Wrap(next {{.HTTP}}.Handler) {{.HTTP}}.Handler { return next }\n",
			want: "package app\n\nimport \"net/http\"\n\nfunc Wrap(next http.Handler) http.Handler { return next }\n",
		},
		{
			name: "Main that names the router and the service",
			kind: ScaffoldMain,
			text: "\nfunc main() {\n\t_ = {{.NewRouter}}({{.NewService}}())\n}\n",
			want: "package main\n\nimport (\n\t\"example.com/work/api\"\n\t\"example.com/work/app\"\n)\n\nfunc main() {\n\t_ = api.NewRouter(app.NewPets())\n}\n",
		},
		{
			name: "Main that names a function the view does not hold",
			kind: ScaffoldMain,
			text: "\nfunc main() {\n\t_ = {{.NewRouter}}({{.NewService}}(), {{symbol `server.adapter` `WithErrorHandler`}}(nil))\n}\n",
			want: "package main\n\nimport (\n\t\"example.com/work/api\"\n\t\"example.com/work/app\"\n)\n\nfunc main() {\n\t_ = api.NewRouter(app.NewPets(), api.WithErrorHandler(nil))\n}\n",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			p := &fakePlugin{name: "sample", contribute: func(*API) (*Contribution, error) {
				return &Contribution{Parts: []PartSource{{Name: "register"}}, Scaffolds: map[ScaffoldKind]string{tc.kind: tc.text}}, nil
			}}
			res, err := generate(t, cmp.Or(tc.cfg, apart), p)

			require.NoError(t, err)
			i := slices.IndexFunc(res.Files, func(f File) bool { return slices.Equal(f.Parts, []string{"server.scaffold." + tc.kind.String()}) })
			require.GreaterOrEqual(t, i, 0)
			assert.Equal(t, "// Code generated by mockzilla-codegen. DO NOT EDIT.\n\n"+tc.want, string(res.Files[i].Content))
		})
	}
}

// A framework that an http.Server does not serve has packages of its own in place of HTTP.
func TestGenerateWithReplacedMainOfEveryFramework(t *testing.T) {
	t.Parallel()

	const text = "\n// HTTP {{.HTTP}}, Framework {{.Framework}}, Packages {{.Packages}}\nfunc main() {}\n"
	own := map[string]string{
		"fasthttp": "// HTTP , Framework , Packages map[fasthttp:fasthttp]",
		"fiber":    "// HTTP , Framework fiber, Packages map[]",
		"goframe":  "// HTTP , Framework ghttp, Packages map[]",
		"hertz":    "// HTTP , Framework server, Packages map[]",
	}

	for _, name := range slices.Sorted(maps.Keys(server.Frameworks())) {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			p := &fakePlugin{name: "sample", contribute: func(*API) (*Contribution, error) {
				return &Contribution{Scaffolds: map[ScaffoldKind]string{ScaffoldMain: text}}, nil
			}}
			res, err := generate(t, "package: api\noutput: {file: ./api/gen.go}\n"+
				"server: {framework: "+name+", scaffold: {service: ./app/service.go, main: ./cmd/main.go}}\n", p)

			require.NoError(t, err)
			i := slices.IndexFunc(res.Files, func(f File) bool { return slices.Equal(f.Parts, []string{"server.scaffold.main"}) })
			require.GreaterOrEqual(t, i, 0)
			assert.Equal(t, "// Code generated by mockzilla-codegen. DO NOT EDIT.\n\npackage main\n\n"+
				cmp.Or(own[name], "// HTTP http, Framework , Packages map[]")+"\nfunc main() {}\n", string(res.Files[i].Content))
		})
	}
}

func TestGenerateWithPluginErrors(t *testing.T) {
	t.Parallel()

	const brokenTemplate = "\n// Register mounts the routes.\nfunc Register( {\n}\n"
	const brokenMessage = "./api/register.go: plugin sample: parse generated code: plugin.sample.register:3:16: expected ')', found '{' (and 1 more errors)\n" +
		"     1 | \n     2 | // Register mounts the routes.\n>    3 | func Register( {\n     4 | }\n     5 | \n"
	giving := func(c *Contribution) func(*API) (*Contribution, error) {
		return func(*API) (*Contribution, error) { return c, nil }
	}
	onListPets := func(fields ...FieldSpec) func(*API) (*Contribution, error) {
		return giving(&Contribution{RequestOptionFields: map[string][]FieldSpec{"ListPets": fields}})
	}
	unknownOps := make(map[string][]FieldSpec)
	for i := 1; i < 9; i++ {
		unknownOps["listPets"+strconv.Itoa(i)] = nil
	}
	unknownKinds := make(map[ScaffoldKind]string)
	for kind := ScaffoldKind(9); kind < 17; kind++ {
		unknownKinds[kind] = "x"
	}
	tests := []struct {
		name    string
		cfg     string
		plugins []Plugin
		wantErr error
		wantMsg string
	}{
		{
			name:    "Draft layout that fails",
			cfg:     strings.Replace(storeConfig, "scaffold: {service: ./api/service.go}", "scaffold: {service: ./api/service.go, main: ./api/main.go}", 1),
			plugins: []Plugin{&fakePlugin{name: "sample"}},
			wantErr: layout.ErrPackageConflict,
			wantMsg: "two packages in one folder: ./api/main.go is package main next to ./api/gen.go, package api",
		},
		{
			name:    "Name that is no segment",
			plugins: []Plugin{&fakePlugin{name: "Sample"}},
			wantErr: ErrPlugin,
			wantMsg: `plugin "Sample": the name must match [a-z][a-z0-9]*`,
		},
		{
			name:    "Name used twice",
			plugins: []Plugin{&fakePlugin{name: "sample"}, &fakePlugin{name: "sample"}},
			wantErr: ErrPlugin,
			wantMsg: "plugin sample: the name is used twice",
		},
		{
			name:    "Reserved name that is no identifier",
			plugins: []Plugin{&fakePlugin{name: "sample", res: Reservations{Idents: []string{"Routes", "my-routes"}}}},
			wantErr: ErrPlugin,
			wantMsg: `plugin sample: reserved name "my-routes" is no identifier`,
		},
		{
			name:    "Field that is no exported identifier",
			plugins: []Plugin{&fakePlugin{name: "sample", contribute: onListPets(FieldSpec{Name: "generate", Type: TypeRef{Name: "int"}})}},
			wantErr: ErrPlugin,
			wantMsg: `plugin sample: request option field "generate" of ListPets is no exported identifier`,
		},
		{
			name:    "Field that is no exported identifier, in a config without a server",
			cfg:     "output: {file: ./api/gen.go}\n",
			plugins: []Plugin{&fakePlugin{name: "sample", contribute: onListPets(FieldSpec{Name: "generate", Type: TypeRef{Name: "int"}})}},
			wantErr: ErrPlugin,
			wantMsg: `plugin sample: request option field "generate" of ListPets is no exported identifier`,
		},
		{
			name:    "Field without a type",
			plugins: []Plugin{&fakePlugin{name: "sample", contribute: onListPets(FieldSpec{Name: "Generate"})}},
			wantErr: ErrPlugin,
			wantMsg: "plugin sample: request option field Generate of ListPets has no type",
		},
		{
			name:    "Field type with a package and no import path",
			plugins: []Plugin{&fakePlugin{name: "sample", contribute: onListPets(FieldSpec{Name: "Span", Type: TypeRef{Name: "*Span", Package: "trace"}})}},
			wantErr: ErrPlugin,
			wantMsg: "plugin sample: request option field Span of ListPets has a type of package trace without an import path",
		},
		{
			name:    "Field type that is no type",
			plugins: []Plugin{&fakePlugin{name: "sample", contribute: onListPets(FieldSpec{Name: "Generate", Type: TypeRef{Name: "func( any"}})}},
			wantErr: ErrPlugin,
			wantMsg: `plugin sample: request option field Generate of ListPets: type "func( any" is no Go type`,
		},
		{
			name: "Field type that cannot be written with its package",
			plugins: []Plugin{&fakePlugin{name: "sample", contribute: onListPets(
				FieldSpec{Name: "Maybe", Type: TypeRef{Name: "Option[Pet]", Package: "opt", ImportPath: "example.com/opt"}},
			)}},
			wantErr: ErrPlugin,
			wantMsg: `plugin sample: request option field Maybe of ListPets: type "Option[Pet]" of example.com/opt is no identifier, nor a pointer, slice, array, map or channel around one`,
		},
		{
			name: "Field type of a package that is no identifier",
			plugins: []Plugin{&fakePlugin{name: "sample", contribute: onListPets(
				FieldSpec{Name: "Span", Type: TypeRef{Name: "*Span", Package: "open-trace", ImportPath: "example.com/trace"}},
			)}},
			wantErr: ErrPlugin,
			wantMsg: `plugin sample: request option field Span of ListPets: type "*Span" of example.com/trace: "open-trace" is no package name`,
		},
		{
			name:    "Field the request options declare",
			plugins: []Plugin{&fakePlugin{name: "sample", contribute: onListPets(FieldSpec{Name: "RawRequest", Type: TypeRef{Name: "int"}})}},
			wantErr: ErrPlugin,
			wantMsg: "plugin sample: request option field RawRequest of ListPets is one the request options declare themselves",
		},
		{
			name: "Field added to one operation twice",
			plugins: []Plugin{&fakePlugin{name: "sample", contribute: onListPets(
				FieldSpec{Name: "Generate", Type: TypeRef{Name: "int"}},
				FieldSpec{Name: "Generate", Type: TypeRef{Name: "string"}},
			)}},
			wantErr: ErrPlugin,
			wantMsg: "plugin sample: request option field Generate of ListPets is added twice",
		},
		{
			name: "Field added to one operation by two plugins",
			plugins: []Plugin{
				&fakePlugin{name: "a", contribute: onListPets(FieldSpec{Name: "Generate", Type: TypeRef{Name: "int"}})},
				&fakePlugin{name: "b", contribute: onListPets(FieldSpec{Name: "Generate", Type: TypeRef{Name: "int"}})},
			},
			wantErr: ErrPlugin,
			wantMsg: "plugin b: request option field Generate of ListPets is added twice",
		},
		{
			name:    "Fields for operations the API does not have, the lowest named",
			plugins: []Plugin{&fakePlugin{name: "sample", contribute: giving(&Contribution{RequestOptionFields: unknownOps})}},
			wantErr: ErrPlugin,
			wantMsg: `plugin sample: request option fields for "listPets1", which is no operation ID of the API`,
		},
		{
			name: "Fields for an operation the plugin gave another ID in its API",
			plugins: []Plugin{&fakePlugin{name: "sample", contribute: func(api *API) (*Contribution, error) {
				api.Operations[0].ID = "Pets"
				return &Contribution{RequestOptionFields: map[string][]FieldSpec{"Pets": {{Name: "Generate", Type: TypeRef{Name: "int"}}}}}, nil
			}}},
			wantErr: ErrPlugin,
			wantMsg: `plugin sample: request option fields for "Pets", which is no operation ID of the API`,
		},
		{
			name:    "Contribute fails",
			plugins: []Plugin{&fakePlugin{name: "sample", contribute: func(*API) (*Contribution, error) { return nil, errContribute }}},
			wantErr: ErrPlugin,
			wantMsg: "plugin sample: nothing to add",
		},
		{
			name:    "Func that no template takes",
			plugins: []Plugin{&fakePlugin{name: "sample", contribute: giving(&Contribution{Funcs: template.FuncMap{"shout": "loudly"}})}},
			wantErr: ErrPlugin,
			wantMsg: "plugin sample: template func: value for shout not a function",
		},
		{
			name:    "Part name that is no segment",
			plugins: []Plugin{&fakePlugin{name: "sample", contribute: giving(&Contribution{Parts: []PartSource{{Name: "register.go"}}})}},
			wantErr: ErrPlugin,
			wantMsg: `plugin sample: the part name "register.go" must match [a-z][a-z0-9]*`,
		},
		{
			name:    "Part contributed twice",
			plugins: []Plugin{&fakePlugin{name: "sample", contribute: giving(&Contribution{Parts: []PartSource{{Name: "register"}, {Name: "register"}}})}},
			wantErr: ErrPlugin,
			wantMsg: "plugin sample: the part register is contributed twice",
		},
		{
			name: "Part import under an alias that is no name",
			plugins: []Plugin{&fakePlugin{name: "sample", contribute: giving(&Contribution{Parts: []PartSource{{
				Name:    "register",
				Imports: []Import{{Path: "embed", Alias: "_"}, {Path: "net/http", Alias: "net-http"}},
			}}})}},
			wantErr: ErrPlugin,
			wantMsg: `plugin sample: the part register: import of net/http: the alias "net-http" is not _, . or an identifier`,
		},
		{
			name:    "Part import without a path",
			plugins: []Plugin{&fakePlugin{name: "sample", contribute: giving(&Contribution{Parts: []PartSource{{Name: "register", Imports: []Import{{Alias: "http"}}}}})}},
			wantErr: ErrPlugin,
			wantMsg: "plugin sample: the part register: import without a path",
		},
		{
			name:    "Scaffold kinds that do not exist, the lowest named",
			plugins: []Plugin{&fakePlugin{name: "sample", contribute: giving(&Contribution{Scaffolds: unknownKinds})}},
			wantErr: ErrPlugin,
			wantMsg: "plugin sample: 9 is no scaffold kind",
		},
		{
			name: "Scaffold replaced by two plugins",
			plugins: []Plugin{
				&fakePlugin{name: "a", contribute: giving(&Contribution{Scaffolds: map[ScaffoldKind]string{ScaffoldService: "x"}})},
				&fakePlugin{name: "b", contribute: giving(&Contribution{Scaffolds: map[ScaffoldKind]string{ScaffoldService: "y"}})},
			},
			wantErr: ErrPlugin,
			wantMsg: "plugin b: the service scaffold is already replaced by a",
		},
		{
			name:    "Part template that does not parse",
			plugins: []Plugin{&fakePlugin{name: "sample", contribute: giving(&Contribution{Parts: []PartSource{{Name: "register", Template: "{{if}}"}}})}},
			wantErr: ErrPlugin,
			wantMsg: "./api/register.go: plugin sample: load templates: plugin.sample.register: template: plugin.sample.register:1: missing value for if",
		},
		{
			name: "Scaffold template that does not parse",
			plugins: []Plugin{&fakePlugin{name: "sample", contribute: giving(&Contribution{
				Parts:     []PartSource{{Name: "register"}},
				Scaffolds: map[ScaffoldKind]string{ScaffoldService: "{{if}}"},
			})}},
			wantErr: ErrPlugin,
			wantMsg: "./api/service.go: plugin sample: load templates: server.scaffold.service: template: server.scaffold.service:1: missing value for if",
		},
		{
			name: "Part that asks expr for a type that cannot be written with its package",
			plugins: []Plugin{&fakePlugin{name: "sample", contribute: giving(&Contribution{Parts: []PartSource{{
				Name:     "register",
				Template: "{{expr .}}",
				Data:     TypeRef{Name: "Option[Pet]", Package: "api", ImportPath: "example.com/work/api"},
			}}})}},
			wantErr: ErrPlugin,
			wantMsg: `./api/register.go: plugin sample: render: template: plugin.sample.register:1:2: executing "plugin.sample.register" at <expr .>: error calling expr: ` +
				`type "Option[Pet]" of example.com/work/api is no identifier, nor a pointer, slice, array, map or channel around one`,
		},
		{
			name: "Part that asks expr for a type of a package that is no identifier",
			plugins: []Plugin{&fakePlugin{name: "sample", contribute: giving(&Contribution{Parts: []PartSource{{
				Name:     "register",
				Template: "{{expr .}}",
				Data:     TypeRef{Name: "*Span", Package: "_", ImportPath: "example.com/trace"},
			}}})}},
			wantErr: ErrPlugin,
			wantMsg: `./api/register.go: plugin sample: render: template: plugin.sample.register:1:2: executing "plugin.sample.register" at <expr .>: error calling expr: ` +
				`type "*Span" of example.com/trace: "_" is no package name`,
		},
		{
			name:    "Part that asks import for no path",
			plugins: []Plugin{&fakePlugin{name: "sample", contribute: giving(&Contribution{Parts: []PartSource{{Name: "register", Template: "{{import .}}", Data: ""}}})}},
			wantErr: ErrPlugin,
			wantMsg: `./api/register.go: plugin sample: render: template: plugin.sample.register:1:2: executing "plugin.sample.register" at <import .>: error calling import: ` +
				"import without a path",
		},
		{
			name:    "Part that asks symbol for a part the config does not write",
			plugins: []Plugin{&fakePlugin{name: "sample", contribute: giving(&Contribution{Parts: []PartSource{{Name: "register", Template: `{{symbol "client.core" "Client"}}`}}})}},
			wantErr: ErrPlugin,
			wantMsg: `./api/register.go: plugin sample: render: template: plugin.sample.register:1:2: executing "plugin.sample.register" at <symbol "client.core" "Client">: ` +
				`error calling symbol: symbol "Client": the config writes no part "client.core"`,
		},
		{
			name:    "Part that asks symbol for a name that is no identifier",
			plugins: []Plugin{&fakePlugin{name: "sample", contribute: giving(&Contribution{Parts: []PartSource{{Name: "register", Template: `{{symbol "server.router" "NewRouter()"}}`}}})}},
			wantErr: ErrPlugin,
			wantMsg: `./api/register.go: plugin sample: render: template: plugin.sample.register:1:2: executing "plugin.sample.register" at <symbol "server.router" "NewRouter()">: ` +
				`error calling symbol: symbol "NewRouter()" of server.router is no identifier`,
		},
		{
			name:    "Part that asks symbol for an unexported name of another folder",
			plugins: []Plugin{&fakePlugin{name: "sample", contribute: giving(&Contribution{Parts: []PartSource{{Name: "register", Template: `{{symbol "models.types" "pet"}}`}}})}},
			wantErr: ErrPlugin,
			wantMsg: `./api/register.go: plugin sample: render: template: plugin.sample.register:1:2: executing "plugin.sample.register" at <symbol "models.types" "pet">: ` +
				"error calling symbol: symbol pet of models.types is not exported, so no file outside the folder of ./models/models.go can use it",
		},
		{
			name:    "Part that writes no Go code",
			plugins: []Plugin{&fakePlugin{name: "sample", contribute: giving(&Contribution{Parts: []PartSource{{Name: "register", Template: brokenTemplate}}})}},
			wantErr: ErrPlugin,
			wantMsg: brokenMessage,
		},
		{
			name:    "Part that writes no Go code, in a file that is not formatted",
			cfg:     rawConfig,
			plugins: []Plugin{&fakePlugin{name: "sample", contribute: giving(&Contribution{Parts: []PartSource{{Name: "register", Template: brokenTemplate}}})}},
			wantErr: ErrPlugin,
			wantMsg: brokenMessage,
		},
		{
			name: "Part that leaves a declaration open, before another part of its file",
			cfg:  strings.Replace(storeConfig, "[plugin.sample.register]", "[plugin.sample]", 1),
			plugins: []Plugin{&fakePlugin{name: "sample", contribute: giving(&Contribution{Parts: []PartSource{
				{Name: "open", Template: "\nfunc Register() {\n"},
				{Name: "closed", Template: "\nfunc Mount() {}\n"},
			}})}},
			wantErr: ErrPlugin,
			wantMsg: "./api/register.go: plugin sample: parse generated code: plugin.sample.open:2:19: expected '}', found 'EOF'\n" +
				"     1 | \n>    2 | func Register() {\n     3 | \n",
		},
		{
			name: "Scaffold that writes no Go code",
			plugins: []Plugin{&fakePlugin{name: "sample", contribute: giving(&Contribution{
				Parts:     []PartSource{{Name: "register"}},
				Scaffolds: map[ScaffoldKind]string{ScaffoldService: "\ntype {{.Name}} struct {\n"},
			})}},
			wantErr: ErrPlugin,
			wantMsg: "./api/service.go: plugin sample: parse generated code: server.scaffold.service:2:20: expected '}', found 'EOF'\n" +
				"     1 | \n>    2 | type Pets struct {\n     3 | \n",
		},
		{
			name: "Part that uses a type of a folder which imports its own",
			cfg:  strings.Replace(storeConfig, "./api/register.go", "./models/register.go", 1),
			plugins: []Plugin{&fakePlugin{name: "sample", contribute: func(api *API) (*Contribution, error) {
				return &Contribution{Parts: []PartSource{{Name: "register", Template: "\nvar Options {{expr .}}\n", Data: api.Operations[0].RequestOptions}}}, nil
			}}},
			wantErr: layout.ErrImportCycle,
			wantMsg: "import cycle: api -> models -> api (models.responses uses example.com/work/models, plugin.sample.register uses example.com/work/api)",
		},
		{
			name: "Replaced scaffold that uses a type of a folder which imports its own",
			cfg:  strings.Replace(storeConfig, "service: ./api/service.go", "service: ./models/service.go", 1),
			plugins: []Plugin{&fakePlugin{name: "sample", contribute: giving(&Contribution{
				Parts:     []PartSource{{Name: "register"}},
				Scaffolds: map[ScaffoldKind]string{ScaffoldService: "\nvar _ {{.Interface}} = (*{{.Name}})(nil)\n\ntype {{.Name}} struct{}\n"},
			})}},
			wantErr: layout.ErrImportCycle,
			wantMsg: "import cycle: api -> models -> api (models.responses uses example.com/work/models, server.scaffold.service uses example.com/work/api)",
		},
		{
			name: "Field of a type from a folder which imports the service's",
			cfg:  strings.Replace(storeConfig, "service: ./api/service.go", "service: ./app/service.go", 1),
			plugins: []Plugin{&fakePlugin{name: "sample", contribute: giving(&Contribution{
				Parts:               []PartSource{{Name: "register"}},
				RequestOptionFields: map[string][]FieldSpec{"ListPets": {{Name: "Owner", Type: TypeRef{Name: "*Pets", Package: "app", ImportPath: "example.com/work/app"}}}},
			})}},
			wantErr: layout.ErrImportCycle,
			wantMsg: "import cycle: api -> app -> api (server.service uses example.com/work/app, server.scaffold.service uses example.com/work/api)",
		},
		{
			name:    "Built-in scaffold in a folder the service imports",
			cfg:     strings.Replace(storeConfig, "service: ./api/service.go", "service: ./models/service.go", 1),
			plugins: []Plugin{&fakePlugin{name: "sample", contribute: giving(&Contribution{Parts: []PartSource{{Name: "register"}}})}},
			wantErr: layout.ErrImportCycle,
			wantMsg: "import cycle: api -> models -> api (models.responses uses models.types, server.scaffold.service uses server.service)",
		},
		{
			name:    "Part that imports its own package",
			plugins: []Plugin{&fakePlugin{name: "sample", contribute: giving(&Contribution{Parts: []PartSource{{Name: "register", Template: "\nvar New = {{import `example.com/work/api`}}.NewRouter\n"}}})}},
			wantErr: layout.ErrImportCycle,
			wantMsg: "import cycle: api -> api (plugin.sample.register uses example.com/work/api)",
		},
		{
			name: "Part that names a function of a folder which imports its own",
			cfg:  strings.Replace(storeConfig, "./api/register.go", "./models/register.go", 1),
			plugins: []Plugin{&fakePlugin{name: "sample", contribute: giving(&Contribution{Parts: []PartSource{{
				Name:     "register",
				Template: "\nvar New = {{symbol `server.router` `NewRouter`}}\n",
			}}})}},
			wantErr: layout.ErrImportCycle,
			wantMsg: "import cycle: api -> models -> api (models.responses uses example.com/work/models, plugin.sample.register uses example.com/work/api)",
		},
		{
			name:    "Selector for a part the plugin does not add",
			plugins: []Plugin{&fakePlugin{name: "sample", contribute: giving(&Contribution{Parts: []PartSource{{Name: "other", Template: ""}}})}},
			wantErr: layout.ErrUnknownSelector,
			wantMsg: `unknown selector "plugin.sample.register" in ./api/register.go; the parts are models.types, models.enums, models.unions, models.params, models.bodies, models.responses, ` +
				"server.service, server.errors, server.adapter, server.router, server.scaffold.service, plugin.sample.other",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			res, err := generate(t, cmp.Or(tc.cfg, storeConfig), tc.plugins...)

			require.ErrorIs(t, err, tc.wantErr)
			require.EqualError(t, err, tc.wantMsg)
			assert.Nil(t, res)
			assert.Equal(t, errors.Is(tc.wantErr, ErrPlugin), errors.Is(err, ErrPlugin), "only what a plugin gives is a plugin error")
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
