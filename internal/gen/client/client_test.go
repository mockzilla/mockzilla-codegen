// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package client

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mockzilla/mockzilla-codegen/internal/diag"
	"github.com/mockzilla/mockzilla-codegen/internal/gen/models"
	"github.com/mockzilla/mockzilla-codegen/internal/gocode"
	"github.com/mockzilla/mockzilla-codegen/internal/gomodel"
	"github.com/mockzilla/mockzilla-codegen/internal/layout"
	"github.com/mockzilla/mockzilla-codegen/internal/naming"
	"github.com/mockzilla/mockzilla-codegen/internal/render"
	"github.com/mockzilla/mockzilla-codegen/internal/spec"
	"github.com/mockzilla/mockzilla-codegen/pkg/config"
)

// splitConfig places the request options and the envelopes in a folder of their own; plainConfig
// does the same for a client without envelopes.
const (
	splitConfig = "output:\n  file: ./api/gen.go\n  module: example.com/work\n  files:\n    ./types/types.go: [client.options, client.responses, models]\n"
	plainConfig = "output:\n  file: ./api/gen.go\n  module: example.com/work\n  files:\n    ./types/types.go: [client.options, models]\n"
)

func TestNew(t *testing.T) {
	t.Parallel()

	m := petModel()
	m.Operations = append(m.Operations, &gomodel.Operation{Name: "Hook", Spec: &spec.Operation{Method: "POST", Path: "/hook", IsWebhook: true}})

	g, diags := New(m, allOptions())

	names := make([]string, len(g.ops))
	for i, op := range g.ops {
		names[i] = op.Name
	}
	assert.Equal(t, []string{"ListPets", "CreatePet", "DeletePet", "Ping", "Query", "Chat", "Tail"}, names)
	assert.Empty(t, diags)
}

func TestNewWarnsAboutStreamOnlyOperationsWithoutStreams(t *testing.T) {
	t.Parallel()

	opts := allOptions()
	opts.HasStreams = false

	_, diags := New(petModel(), opts)

	assert.Equal(t, []diag.Diagnostic{{
		Severity: diag.Warning,
		Code:     diag.CodeStreamOnly,
		Pointer:  "/paths/~1tail/get",
		Origin:   diag.Origin{File: "api.yaml", Line: 40, Col: 5},
		Message:  "Tail answers only as application/x-ndjson, which Tail reads whole; set client.streaming to read it as it arrives",
	}}, diags)
}

func TestTemplates(t *testing.T) {
	t.Parallel()

	set := Templates()

	assert.Equal(t, "client", set.Name)
	assert.Equal(t, map[layout.PartID]string{PartCore: "core.tmpl", PartOptions: "options.tmpl", PartOperations: "operations.tmpl", PartResponses: "responses.tmpl"}, set.Parts)
	_, err := render.New([]render.Set{set}, render.Options{})
	require.NoError(t, err)
}

func TestParts(t *testing.T) {
	t.Parallel()

	requests := []layout.PartID{gomodel.PartParams, gomodel.PartTypes}
	responses := []layout.PartID{gomodel.PartResponses, gomodel.PartTypes}
	tests := []struct {
		name         string
		withResponse bool
		want         []layout.Part
	}{
		{
			name: "Plain methods",
			want: []layout.Part{
				{ID: PartCore},
				{ID: PartOptions, Uses: requests},
				{ID: PartOperations, Uses: []layout.PartID{PartCore, PartOptions, gomodel.PartResponses, gomodel.PartTypes}, Owner: PartCore},
			},
		},
		{
			name:         "With envelopes",
			withResponse: true,
			want: []layout.Part{
				{ID: PartCore},
				{ID: PartOptions, Uses: requests},
				{ID: PartOperations, Uses: []layout.PartID{PartCore, PartOptions, PartResponses, gomodel.PartResponses, gomodel.PartTypes}, Owner: PartCore},
				{ID: PartResponses, Uses: responses},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			opts := allOptions()
			opts.HasEnvelopes = tc.withResponse
			g, _ := New(petModel(), opts)

			assert.Equal(t, tc.want, g.Parts())
		})
	}
}

// TestViewRendersParts compares each part with testdata/<part>.golden, and the parts that differ
// without envelopes with testdata/<part>.plain.golden. UPDATE=1 writes them instead.
func TestViewRendersParts(t *testing.T) {
	t.Parallel()

	parts := []layout.PartID{PartCore, PartOptions, PartOperations, PartResponses}
	for _, part := range parts {
		t.Run(string(part), func(t *testing.T) {
			t.Parallel()

			m := petModel()
			g, _ := New(m, allOptions())
			got := fixture{m: m, g: g, cfg: splitConfig}.render(t, part)

			assertGolden(t, filepath.Join("testdata", string(part)+".golden"), got)
		})
	}
	t.Run(string(PartOperations)+" plain", func(t *testing.T) {
		t.Parallel()

		m := petModel()
		opts := allOptions()
		opts.HasEnvelopes = false
		g, _ := New(m, opts)
		got := fixture{m: m, g: g, cfg: plainConfig}.render(t, PartOperations)

		assertGolden(t, filepath.Join("testdata", string(PartOperations)+".plain.golden"), got)
	})
}

func TestViewWithoutOperations(t *testing.T) {
	t.Parallel()

	m := &gomodel.Model{}
	opts := allOptions()
	opts.Timeout = 0
	g, _ := New(m, opts)
	f := fixture{m: m, g: g, cfg: splitConfig}

	core := string(f.render(t, PartCore))
	assert.Contains(t, core, "func NewPetClient(baseURL string, opts ...PetClientOption) (*PetClient, error)")
	assert.Contains(t, core, "// WithHTTPClient sets another. A call has no time limit unless WithTimeout sets one.\n")
	assert.Contains(t, core, "c := &PetClient{baseURL: u, doer: &http.Client{}}\n")
	assert.Contains(t, core, "// WithTimeout sets how long a call may take, 0 for no limit.\nfunc WithTimeout(d time.Duration) PetClientOption {\n")
	assert.Equal(t, "package types\n", string(f.render(t, PartOptions)))
	assert.Equal(t, "package types\n", string(f.render(t, PartResponses)))
	assert.Equal(t, "package api\n\n// PetClientInterface is what PetClient implements: one method per operation, so a test double can\n"+
		"// stand in for the client.\ntype PetClientInterface interface {\n}\n\nvar _ PetClientInterface = (*PetClient)(nil)\n", string(f.render(t, PartOperations)))
}

// assertGolden compares got with the file at path, or writes it when UPDATE is set.
func assertGolden(t *testing.T, path string, got []byte) {
	t.Helper()

	if os.Getenv("UPDATE") != "" {
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, got, 0o644))
		return
	}
	want, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, string(want), string(got))
}

func allOptions() Options {
	return Options{
		Name:         "PetClient",
		Namer:        naming.New(nil),
		Timeout:      5 * time.Second,
		HasEnvelopes: true,
		HasStreams:   true,
		User:         map[string]any{"owner": "platform"},
	}
}

// fixture is a model, its generator and the config that lays the parts out.
type fixture struct {
	m   *gomodel.Model
	g   *Generator
	cfg string
}

// render renders one part into the file the layout gives it.
func (f fixture) render(t *testing.T, part layout.PartID) []byte {
	t.Helper()

	e, err := render.New([]render.Set{Templates()}, render.Options{Format: true})
	require.NoError(t, err)
	s := f.scope(t, part)
	out, err := e.RenderPart(part, f.g.View(part, s))
	require.NoError(t, err)
	got, err := e.RenderFile(render.FileData{Package: s.File.Package, Imports: s.Imports.Decl(), Parts: []string{string(out)}})
	require.NoError(t, err)
	return got
}

// scope is the scope of the file the layout gives part.
func (f fixture) scope(t *testing.T, part layout.PartID) *gocode.Scope {
	t.Helper()

	cfg, err := config.Parse([]byte(f.cfg), "/work")
	require.NoError(t, err)
	l, err := layout.Plan(cfg, append(models.New(f.m).Parts(), f.g.Parts()...), layout.Module{Path: "example.com/work", Dir: "/work"})
	require.NoError(t, err)
	return gocode.NewScope(l.FileOf(part), l)
}

// petModel is a model with every shape the client writes: parameters of each location, one or
// several bodies of every kind, bodies without a schema, responses with and without bodies,
// ranges, default, typed headers, error types, a method net/http has no constant for, a streamed
// response next to a JSON one, and one that streams alone.
func petModel() *gomodel.Model {
	str := gomodel.Builtin{Name: "string"}
	pet := &gomodel.Decl{Name: "Pet", Part: gomodel.PartTypes, Kind: gomodel.KindStruct, Struct: &gomodel.Struct{}, Validation: &gomodel.Validation{}}
	problem := &gomodel.Decl{Name: "Problem", Part: gomodel.PartTypes, Kind: gomodel.KindStruct, Struct: &gomodel.Struct{}, Error: &gomodel.ErrorMessage{Path: "detail"}}
	failure := &gomodel.Decl{Name: "Failure", Part: gomodel.PartTypes, Kind: gomodel.KindAlias, Target: gomodel.DeclRef{Decl: problem}}
	locked := &gomodel.Decl{Name: "Locked", Part: gomodel.PartTypes, Kind: gomodel.KindStruct, Struct: &gomodel.Struct{}}
	query := &gomodel.Decl{Name: "ListPetsQuery", Part: gomodel.PartParams, Kind: gomodel.KindStruct, Validation: &gomodel.Validation{}, Struct: &gomodel.Struct{
		Fields: []*gomodel.Field{{Name: "Limit", Type: gomodel.Pointer{Elem: gomodel.Builtin{Name: "int"}}}, {Name: "Filter", Type: gomodel.Map{Key: str, Elem: str}}},
	}}
	headers := &gomodel.Decl{Name: "ListPetsHeaders", Part: gomodel.PartParams, Kind: gomodel.KindStruct, Struct: &gomodel.Struct{
		Fields: []*gomodel.Field{{Name: "XTrace", Type: gomodel.Pointer{Elem: str}}},
	}}
	cookies := &gomodel.Decl{Name: "ListPetsCookies", Part: gomodel.PartParams, Kind: gomodel.KindStruct, Struct: &gomodel.Struct{
		Fields: []*gomodel.Field{{Name: "Session", Type: str}},
	}}
	querystring := &gomodel.Decl{Name: "ListPetsQueryString", Part: gomodel.PartParams, Kind: gomodel.KindStruct, Struct: &gomodel.Struct{}}
	path := &gomodel.Decl{Name: "DeletePetPathParams", Part: gomodel.PartParams, Kind: gomodel.KindStruct, Struct: &gomodel.Struct{
		Fields: []*gomodel.Field{{Name: "ID", Type: str}},
	}}
	respHeaders := &gomodel.Decl{Name: "ListPetsResponse200Headers", Part: gomodel.PartResponses, Kind: gomodel.KindStruct, Struct: &gomodel.Struct{}}
	errHeaders := &gomodel.Decl{Name: "ListPetsResponseDefaultHeaders", Part: gomodel.PartResponses, Kind: gomodel.KindStruct, Struct: &gomodel.Struct{}}
	pets := &gomodel.Decl{Name: "Pets", Part: gomodel.PartResponses, Kind: gomodel.KindDefined, Target: gomodel.Slice{Elem: gomodel.DeclRef{Decl: pet}}}
	note := &gomodel.Decl{Name: "Note", Part: gomodel.PartTypes, Kind: gomodel.KindDefined, Target: str}
	upload := &gomodel.Decl{Name: "Upload", Part: gomodel.PartTypes, Kind: gomodel.KindStruct, Struct: &gomodel.Struct{}}
	file := gomodel.Qualified{Import: gomodel.Import{Path: gomodel.RuntimePath}, Name: "File"}
	chunk := &gomodel.Decl{Name: "ChatResponseItem", Part: gomodel.PartResponses, Kind: gomodel.KindStruct, Struct: &gomodel.Struct{}}

	list := &gomodel.Operation{
		Name: "ListPets",
		Spec: &spec.Operation{Method: "GET", Path: "/pets", Summary: "List pets", Description: "Returns pets."},
		Params: []gomodel.ParamGroup{
			{In: spec.InQuery, Decl: query, Params: []*spec.Parameter{
				{Name: "limit", In: spec.InQuery, Style: "form", Explode: true},
				{Name: "filter", In: spec.InQuery, Style: "deepObject", Explode: true, Required: true},
			}},
			{In: spec.InHeader, Decl: headers, Params: []*spec.Parameter{
				{Name: "X-Trace", In: spec.InHeader, Contents: []*spec.MediaType{{Name: "application/json"}}},
			}},
			{In: spec.InCookie, Decl: cookies, Params: []*spec.Parameter{
				{Name: "session", In: spec.InCookie, Style: "form", Required: true},
			}},
			{In: spec.InQueryString, Decl: querystring},
		},
		Responses: []gomodel.Response{
			{Status: "200", Contents: []gomodel.Content{{MediaType: "application/xml", Type: str}, {MediaType: "application/json", Type: gomodel.DeclRef{Decl: pets}}}, Headers: respHeaders},
			{Status: "default", Contents: []gomodel.Content{{MediaType: "application/json", Type: gomodel.DeclRef{Decl: problem}}}, Headers: errHeaders},
		},
	}
	create := &gomodel.Operation{
		Name: "CreatePet",
		Spec: &spec.Operation{Method: "POST", Path: "/pets", Summary: "Same", Description: "Same", Deprecated: true, Body: &spec.RequestBody{Required: true}},
		Bodies: []gomodel.Content{
			{MediaType: "application/json", Type: gomodel.DeclRef{Decl: pet}},
			{MediaType: "application/x-www-form-urlencoded", Type: gomodel.DeclRef{Decl: pet}},
			{MediaType: "multipart/form-data", Type: gomodel.DeclRef{Decl: upload}},
			{MediaType: "text/plain"},
			{MediaType: "text/markdown", Type: gomodel.DeclRef{Decl: note}},
			{MediaType: "image/png"},
			{MediaType: "image/jpeg", Type: file},
			{MediaType: "application/xml", Type: gomodel.DeclRef{Decl: pet}},
		},
		Responses: []gomodel.Response{
			{Status: "202", Contents: []gomodel.Content{{MediaType: "application/json", Type: gomodel.DeclRef{Decl: note}}}},
			{Status: "201", Contents: []gomodel.Content{{MediaType: "application/json", Type: gomodel.Pointer{Elem: gomodel.DeclRef{Decl: pet}}}, {MediaType: "application/xml", Type: gomodel.DeclRef{Decl: pet}}}, Headers: respHeaders},
			{Status: "409", Contents: []gomodel.Content{{MediaType: "application/json", Type: gomodel.DeclRef{Decl: locked}}}},
			{Status: "4XX", Contents: []gomodel.Content{{MediaType: "application/problem+json", Type: gomodel.Pointer{Elem: gomodel.DeclRef{Decl: failure}}}}},
			{Status: "5XX", Contents: []gomodel.Content{{MediaType: "application/problem+json", Type: gomodel.DeclRef{Decl: problem}}}},
		},
	}
	del := &gomodel.Operation{
		Name:   "DeletePet",
		Spec:   &spec.Operation{Method: "DELETE", Path: "/pets/{id}", Deprecated: true, Body: &spec.RequestBody{}},
		Params: []gomodel.ParamGroup{{In: spec.InPath, Decl: path, Params: []*spec.Parameter{{Name: "id", In: spec.InPath, Style: "simple", Required: true}}}},
		Bodies: []gomodel.Content{{MediaType: "application/json", Type: gomodel.Map{Key: str, Elem: str}}},
		Responses: []gomodel.Response{
			{Status: "204"},
			{Status: "404", Contents: []gomodel.Content{{MediaType: "application/json", Type: gomodel.DeclRef{Decl: problem}}}},
		},
	}
	ping := &gomodel.Operation{
		Name:   "Ping",
		Spec:   &spec.Operation{Method: "GET", Path: "/ping"},
		Bodies: []gomodel.Content{{MediaType: "*/*", Type: gomodel.Slice{Elem: gomodel.DeclRef{Decl: pet}}}},
		Responses: []gomodel.Response{
			{Status: "200", Contents: []gomodel.Content{{MediaType: "text/plain"}, {MediaType: "text/xml", Type: str}, {MediaType: "image/*"}}},
			{Status: "default", Contents: []gomodel.Content{{MediaType: "application/json"}}},
		},
	}
	queryOp := &gomodel.Operation{
		Name:      "Query",
		Spec:      &spec.Operation{Method: "QUERY", Path: "/pets/query", Body: &spec.RequestBody{}},
		Bodies:    []gomodel.Content{{MediaType: "text/plain", Type: str}},
		Responses: []gomodel.Response{{Status: "200"}},
	}
	chat := &gomodel.Operation{
		Name:   "Chat",
		Spec:   &spec.Operation{Method: "POST", Path: "/chat", Body: &spec.RequestBody{Required: true}},
		Bodies: []gomodel.Content{{MediaType: "application/json", Type: gomodel.DeclRef{Decl: pet}}},
		Responses: []gomodel.Response{
			{Status: "200", Contents: []gomodel.Content{{MediaType: "application/json", Type: gomodel.DeclRef{Decl: pet}}, {MediaType: "text/event-stream", Item: gomodel.DeclRef{Decl: chunk}}}},
			{Status: "default", Contents: []gomodel.Content{{MediaType: "application/json", Type: gomodel.DeclRef{Decl: problem}}}},
		},
	}
	tail := &gomodel.Operation{
		Name:      "Tail",
		Spec:      &spec.Operation{Method: "GET", Path: "/tail", Origin: spec.Origin{Pointer: "/paths/~1tail/get", File: "api.yaml", Line: 40, Col: 5}},
		Responses: []gomodel.Response{{Status: "200", Contents: []gomodel.Content{{MediaType: "application/x-ndjson", Type: str, Item: str}}}},
	}
	return &gomodel.Model{
		Decls:      []*gomodel.Decl{pet, problem, failure, locked, query, headers, cookies, querystring, path, respHeaders, errHeaders, pets, note, upload, chunk},
		Operations: []*gomodel.Operation{list, create, del, ping, queryOp, chat, tail},
	}
}
