// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package codegen

import (
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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

// workDir is a module folder, so output in several folders can import across them.
func workDir(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/work\n"), 0o600))
	return dir
}

// run generates doc with cfg, a config in dir, step by step as Generate does, and keeps the run.
func run(t *testing.T, dir, cfg, doc string) *generation {
	t.Helper()

	c, err := config.Parse([]byte(cfg), dir)
	require.NoError(t, err)
	g := &generation{cfg: c, opts: newOptions([]Option{WithSpec([]byte(doc))})}
	require.NoError(t, g.model(t.Context()))
	require.NoError(t, g.place())
	require.NoError(t, g.load())
	require.NoError(t, g.render())
	return g
}

func TestDescribe(t *testing.T) {
	t.Parallel()

	g := run(t, workDir(t), `package: api
output: {file: ./api/gen.go, files: {./models/models.go: [models]}}
server: {framework: chi, name: Pets}
user-context: {owner: platform}
`, storeSpec)
	api := describe(g)

	inAPI, inModels := TypeRef{Package: "api", ImportPath: "example.com/work/api"}, TypeRef{Package: "models", ImportPath: "example.com/work/models"}
	named := func(base TypeRef, name string) TypeRef {
		base.Name = name
		return base
	}
	listPets := []Response{{
		Status: "200", Code: 200, ContentType: "application/json", Body: named(inModels, "ListPetsResponse200"),
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
			named(inModels, "Pet"), named(inModels, "ListPetsQuery"), named(inModels, "ListPetsResponse200"),
			named(inModels, "DeletePetPathParams"), named(inModels, "RemovePetPathParams"),
		},
		UserContext: map[string]any{"owner": "platform"},
	}, api)
	assert.Same(t, &api.Operations[1].Responses[0], api.Operations[1].Success, "Success points into Responses")

	i := slices.IndexFunc(g.files, func(f File) bool { return filepath.Base(f.Path) == "gen.go" })
	require.GreaterOrEqual(t, i, 0)
	gen := string(g.files[i].Content)
	for _, op := range api.Operations {
		for _, r := range op.Responses {
			assert.Contains(t, gen, "\nfunc "+r.Constructor.Name+"(", "the constructor of %s %s", op.ID, r.Status)
		}
	}
	assert.Contains(t, gen, "\nfunc NewCreatePetResponseDataDefault(status int, body *models.Pet) *CreatePetResponseData {\n")
	assert.Contains(t, gen, "\nfunc NewHealthResponseData(status int) *HealthResponseData {\n")
}

func TestDescribeWithoutServer(t *testing.T) {
	t.Parallel()

	api := describe(run(t, workDir(t), "output: {file: ./api/gen.go}\nclient:\n", storeSpec))

	listPets := []Response{{Status: "200", Code: 200, ContentType: "application/json", Body: TypeRef{Name: "ListPetsResponse200", Package: "api", ImportPath: "example.com/work/api"}}}
	assert.Equal(t, TypeRef{}, api.Service)
	assert.Equal(t, Operation{
		ID: "ListPets", Method: "GET", Path: "/pets", Summary: "List pets", Tags: []string{"pets"}, HasOptions: true,
		ClientRequestOptions: TypeRef{Name: "ListPetsRequestOptions", Package: "api", ImportPath: "example.com/work/api"},
		Responses:            listPets, Success: &listPets[0],
	}, api.Operations[0], "no constructor without a server")
	assert.Equal(t, []Response{{Status: "default"}}, api.Operations[4].Responses, "nor a status argument")
}

func TestDescribeClient(t *testing.T) {
	t.Parallel()

	inAPI := TypeRef{Package: "api", ImportPath: "example.com/work/api"}
	inTypes := TypeRef{Package: "types", ImportPath: "example.com/work/types"}
	named := func(base TypeRef, name string) TypeRef {
		base.Name = name
		return base
	}
	tests := []struct {
		name         string
		cfg          string
		wantOptions  TypeRef
		wantResponse TypeRef
	}{
		{
			name:        "Client without envelopes",
			cfg:         "output: {file: ./api/gen.go}\nclient:\n",
			wantOptions: named(inAPI, "ListPetsRequestOptions"),
		},
		{
			name:         "Client with envelopes in another folder",
			cfg:          "output: {file: ./api/gen.go, files: {./types/types.go: [client.options, client.responses, models]}}\nclient: {with-response: true}\n",
			wantOptions:  named(inTypes, "ListPetsRequestOptions"),
			wantResponse: named(inTypes, "ListPetsResponse"),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			api := describe(run(t, workDir(t), tc.cfg, storeSpec))

			assert.Equal(t, tc.wantOptions, api.Operations[0].ClientRequestOptions)
			assert.Equal(t, tc.wantResponse, api.Operations[0].ClientResponse)
			assert.Equal(t, TypeRef{}, api.Operations[5].ClientRequestOptions, "a webhook has no client method")
			assert.Equal(t, TypeRef{}, api.Operations[5].ClientResponse)
		})
	}
}

func TestDescribeOutsideModule(t *testing.T) {
	t.Parallel()

	api := describe(run(t, t.TempDir(), "package: api\noutput: {file: ./gen.go}\nserver: {framework: chi}\nclient: {with-response: true}\n", storeSpec))

	op := api.Operations[0]
	assert.Equal(t, "api", api.Package)
	assert.Equal(t, TypeRef{Name: "ServiceInterface"}, api.Service, "no package and no import path without a module")
	assert.Equal(t, TypeRef{Name: "Pet"}, api.Types[0])
	assert.Equal(t, TypeRef{Name: "ListPetsServiceRequestOptions"}, op.RequestOptions)
	assert.Equal(t, TypeRef{Name: "ListPetsResponseData"}, op.ResponseData)
	assert.Equal(t, TypeRef{Name: "ListPetsRequestOptions"}, op.ClientRequestOptions)
	assert.Equal(t, TypeRef{Name: "ListPetsResponse"}, op.ClientResponse)
	assert.Equal(t, TypeRef{Name: "ListPetsResponse200"}, op.Success.Body)
	assert.Equal(t, TypeRef{Name: "NewListPetsResponseData"}, op.Success.Constructor)
}

func TestDescribeService(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		cfg             string
		wantPackage     string
		want            TypeRef
		wantConstructor TypeRef
	}{
		{
			name:            "Interface named after the server, in the package of output.file",
			cfg:             "package: pets\noutput: {file: ./api/gen.go}\nserver: {framework: chi, name: Pets}\n",
			wantPackage:     "pets",
			want:            TypeRef{Name: "PetsInterface", Package: "pets", ImportPath: "example.com/work/api"},
			wantConstructor: TypeRef{Name: "NewListPetsResponseData", Package: "pets", ImportPath: "example.com/work/api"},
		},
		{
			name:            "Interface of a server without a name, in the package output.packages gives the folder",
			cfg:             "package: pets\noutput: {file: ./api/gen.go, packages: {./api: petapi}}\nserver: {framework: chi}\n",
			wantPackage:     "petapi",
			want:            TypeRef{Name: "ServiceInterface", Package: "petapi", ImportPath: "example.com/work/api"},
			wantConstructor: TypeRef{Name: "NewListPetsResponseData", Package: "petapi", ImportPath: "example.com/work/api"},
		},
		{
			name:            "Interface in the folder the config moves the service part to",
			cfg:             "package: pets\noutput: {file: ./api/gen.go, files: {./contract/service.go: [server.service, models]}}\nserver: {framework: chi, name: Pets}\n",
			wantPackage:     "pets",
			want:            TypeRef{Name: "PetsInterface", Package: "contract", ImportPath: "example.com/work/contract"},
			wantConstructor: TypeRef{Name: "NewListPetsResponseData", Package: "contract", ImportPath: "example.com/work/contract"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			api := describe(run(t, workDir(t), tc.cfg, storeSpec))

			assert.Equal(t, tc.wantPackage, api.Package)
			assert.Equal(t, tc.want, api.Service)
			assert.Equal(t, tc.wantConstructor, api.Operations[0].Responses[0].Constructor, "next to the interface")
		})
	}
}

func TestDescribeResponses(t *testing.T) {
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
			name:        "Key that is no status is left out",
			keys:        []string{"ok"},
			want:        []Response{},
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

			op := describe(run(t, workDir(t), "package: api\noutput: {file: ./api/gen.go}\nserver: {framework: chi}\n", doc)).Operations[0]

			assert.Equal(t, tc.want, op.Responses)
			if tc.wantSuccess < 0 {
				assert.Nil(t, op.Success)
				return
			}
			assert.Same(t, &op.Responses[tc.wantSuccess], op.Success)
		})
	}
}

func TestDescribeStreamResponse(t *testing.T) {
	t.Parallel()

	doc := "openapi: 3.1.0\ninfo: {title: ping, version: \"1\"}\npaths:\n  /ping:\n    get:\n      operationId: ping\n      responses:\n" +
		"        \"200\": {description: answer, content: {text/event-stream: {schema: {type: integer}}}}\n"

	op := describe(run(t, workDir(t), "package: api\noutput: {file: ./api/gen.go}\nserver: {framework: chi}\n", doc)).Operations[0]

	assert.Equal(t, []Response{{
		Status:      "200",
		Code:        200,
		ContentType: "text/event-stream",
		Body:        TypeRef{Name: "PingResponseItem", Package: "api", ImportPath: "example.com/work/api"},
		IsStream:    true,
		Constructor: TypeRef{Name: "NewPingResponseData", Package: "api", ImportPath: "example.com/work/api"},
	}}, op.Responses)
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

func TestTypeRefElem(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		typ  TypeRef
		want TypeRef
	}{
		{
			name: "Pointer to a type of a package",
			typ:  TypeRef{Name: "*Pet", Package: "models", ImportPath: "example.com/work/models"},
			want: TypeRef{Name: "Pet", Package: "models", ImportPath: "example.com/work/models"},
		},
		{name: "Pointer to a type that needs no import", typ: TypeRef{Name: "*string"}, want: TypeRef{Name: "string"}},
		{
			name: "Pointer to a pointer",
			typ:  TypeRef{Name: "**Pet", Package: "models", ImportPath: "example.com/work/models"},
			want: TypeRef{Name: "*Pet", Package: "models", ImportPath: "example.com/work/models"},
		},
		{name: "Pointer to a slice", typ: TypeRef{Name: "*[]byte"}, want: TypeRef{Name: "[]byte"}},
		{name: "Slice of pointers", typ: TypeRef{Name: "[]*Pet", Package: "models", ImportPath: "example.com/work/models"}},
		{name: "Named type", typ: TypeRef{Name: "Pet", Package: "models", ImportPath: "example.com/work/models"}},
		{name: "No type", typ: TypeRef{}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.want, tc.typ.Elem())
		})
	}
}

func TestTypeRefCheck(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		typ     TypeRef
		wantMsg string
	}{
		{name: "Type that needs no import, whatever it is", typ: TypeRef{Name: "func(Pet) Option[Pet]"}},
		{
			name:    "Package without an import path",
			typ:     TypeRef{Name: "*Pet", Package: "api"},
			wantMsg: `type "*Pet" of package api has no import path`,
		},
		{name: "Map of slices of pointers", typ: TypeRef{Name: "map[string][]*Pet", Package: "models", ImportPath: "example.com/work/models"}},
		{
			name:    "Generic type with an import path",
			typ:     TypeRef{Name: "Option[Pet]", Package: "opt", ImportPath: "example.com/opt"},
			wantMsg: `type "Option[Pet]" of example.com/opt is no identifier, nor a pointer, slice, array, map or channel around one`,
		},
		{
			name:    "Qualified name with an import path",
			typ:     TypeRef{Name: "models.Pet", ImportPath: "example.com/work/models"},
			wantMsg: `type "models.Pet" of example.com/work/models is no identifier, nor a pointer, slice, array, map or channel around one`,
		},
		{name: "Import path without a package", typ: TypeRef{Name: "*Span", ImportPath: "example.com/trace"}},
		{
			name:    "Package that is no identifier",
			typ:     TypeRef{Name: "*Span", Package: "open-trace", ImportPath: "example.com/trace"},
			wantMsg: `type "*Span" of example.com/trace: "open-trace" is no package name`,
		},
		{
			name:    "Package that no type can be written with",
			typ:     TypeRef{Name: "*Span", Package: "_", ImportPath: "example.com/trace"},
			wantMsg: `type "*Span" of example.com/trace: "_" is no package name`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := tc.typ.check()

			if tc.wantMsg == "" {
				require.NoError(t, err)
				return
			}
			require.ErrorIs(t, err, errTypeRef)
			assert.EqualError(t, err, tc.wantMsg)
		})
	}
}
