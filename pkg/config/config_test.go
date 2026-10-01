// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package config

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestResolve(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		dir  string
		path string
		want string
	}{
		{name: "Relative path joins the config dir", dir: "/work/api", path: "./openapi.yaml", want: "/work/api/openapi.yaml"},
		{name: "Parent path leaves the config dir", dir: "/work/api", path: "../specs/a.yml", want: "/work/specs/a.yml"},
		{name: "Absolute path stays", dir: "/work/api", path: "/specs/a.yml", want: "/specs/a.yml"},
		{name: "URL stays", dir: "/work/api", path: "https://example.com/a.yml", want: "https://example.com/a.yml"},
		{name: "Empty dir leaves the path relative", dir: "", path: "./openapi.yaml", want: "openapi.yaml"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			cfg := &Config{dir: tc.dir}
			assert.Equal(t, tc.want, cfg.Resolve(tc.path))
		})
	}
}

func TestValidate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		edit   func(c *Config)
		issues []Issue
	}{
		{
			name: "Defaulted config passes",
			edit: func(*Config) {},
		},
		{
			name: "Blocks left out are not checked",
			edit: func(c *Config) { c.Models = nil },
		},
		{
			name: "Every valid selector form passes",
			edit: func(c *Config) {
				c.Output.Files = map[string][]string{
					"a.go": {"models", "models.types", "server.router", "client", "mcp"},
					"b.go": {"plugin", "plugin.sample", "plugin.sample.register"},
				}
			},
		},
		{
			name:   "Invalid package name",
			edit:   func(c *Config) { c.Package = "pet-store" },
			issues: []Issue{{Key: "package", Message: `"pet-store" is not a valid Go package name`}},
		},
		{
			name:   "Keyword is not a package name",
			edit:   func(c *Config) { c.Package = "func" },
			issues: []Issue{{Key: "package", Message: `"func" is not a valid Go package name`}},
		},
		{
			name: "Imports under the name of the path, an alias and _ pass",
			edit: func(c *Config) {
				c.Imports = []Import{
					{Package: "github.com/google/uuid"},
					{Package: "example.com/shop/tenant", Alias: "tn"},
					{Package: "example.com/9lives", Alias: "lives"},
					{Package: "embed", Alias: "_"},
					{Package: "embed"},
					{Package: "net/http/pprof", Alias: "_"},
				}
			},
		},
		{
			name:   "Import needs a package",
			edit:   func(c *Config) { c.Imports = []Import{{Package: "a"}, {Alias: "b"}} },
			issues: []Issue{{Key: "imports[1].package", Message: "required"}},
		},
		{
			name: "Import under . is not supported",
			edit: func(c *Config) { c.Imports = []Import{{Package: "example.com/dsl", Alias: "."}} },
			issues: []Issue{{
				Key:     "imports[0].alias",
				Message: `"." is not supported: Go rejects a file that imports a package without using it, and under . the use cannot be checked`,
			}},
		},
		{
			name: "Import alias that is no identifier",
			edit: func(c *Config) {
				c.Imports = []Import{{Package: "example.com/a", Alias: "shop-models"}, {Package: "example.com/b", Alias: "type"}}
			},
			issues: []Issue{
				{Key: "imports[0].alias", Message: `"shop-models" is not a valid Go identifier`},
				{Key: "imports[1].alias", Message: `"type" is not a valid Go identifier`},
			},
		},
		{
			name:   "Import path that gives no name needs an alias",
			edit:   func(c *Config) { c.Imports = []Import{{Package: "example.com/9lives"}} },
			issues: []Issue{{Key: "imports[0].alias", Message: `required, "example.com/9lives" does not end in a Go name`}},
		},
		{
			name: "Import path listed twice under a name, or twice under _",
			edit: func(c *Config) {
				c.Imports = []Import{{Package: "embed", Alias: "_"}, {Package: "example.com/a"}, {Package: "embed", Alias: "_"}, {Package: "example.com/a", Alias: "b"}}
			},
			issues: []Issue{
				{Key: "imports[2].package", Message: `"embed" is already listed in imports[0]`},
				{Key: "imports[3].package", Message: `"example.com/a" is already listed in imports[1]`},
			},
		},
		{
			name: "Two imports with one name",
			edit: func(c *Config) {
				c.Imports = []Import{
					{Package: "example.com/a/models"},
					{Package: "embed", Alias: "_"},
					{Package: "example.com/b/models"},
					{Package: "net/http/pprof", Alias: "_"},
					{Package: "example.com/c", Alias: "models"},
				}
			},
			issues: []Issue{
				{Key: "imports[2].alias", Message: `"models" is the name of imports[0] too, name one of them differently`},
				{Key: "imports[4].alias", Message: `"models" is the name of imports[0] too, name one of them differently`},
			},
		},
		{
			name:   "Simplify without optional properties passes",
			edit:   func(c *Config) { c.Spec.Simplify = &Simplify{Unions: true} },
			issues: nil,
		},
		{
			name: "Negative optional properties min",
			edit: func(c *Config) {
				c.Spec.Simplify = &Simplify{OptionalProperties: &OptionalProperties{Min: -1, Max: 2}}
			},
			issues: []Issue{{Key: "spec.simplify.optional-properties.min", Message: "must not be negative"}},
		},
		{
			name: "Optional properties max below min",
			edit: func(c *Config) {
				c.Spec.Simplify = &Simplify{OptionalProperties: &OptionalProperties{Min: 3, Max: 1}}
			},
			issues: []Issue{{Key: "spec.simplify.optional-properties.max", Message: "must be at least min (3)"}},
		},
		{
			name:   "Unknown int type",
			edit:   func(c *Config) { c.Models.IntType = "uint" },
			issues: []Issue{{Key: "models.int-type", Message: `"uint" is not one of int, int32, int64`}},
		},
		{
			name:   "Server needs a framework",
			edit:   func(c *Config) { c.Server = &Server{} },
			issues: []Issue{{Key: "server.framework", Message: "required, one of beego, chi, echo, echo-v5, fasthttp, fiber, gin, go-zero, goframe, gorilla-mux, hertz, iris, kratos, std-http"}},
		},
		{
			name:   "Unknown framework",
			edit:   func(c *Config) { c.Server = &Server{Framework: "express"} },
			issues: []Issue{{Key: "server.framework", Message: `"express" is not one of beego, chi, echo, echo-v5, fasthttp, fiber, gin, go-zero, goframe, gorilla-mux, hertz, iris, kratos, std-http`}},
		},
		{
			name:   "The main scaffold needs the service scaffold",
			edit:   func(c *Config) { c.Server = &Server{Framework: "chi", Scaffold: Scaffold{Main: "cmd/server/main.go"}} },
			issues: []Issue{{Key: "server.scaffold.main", Message: "needs server.scaffold.service, which main starts"}},
		},
		{
			name:   "MCP needs a client",
			edit:   func(c *Config) { c.MCP = &MCP{} },
			issues: []Issue{{Key: "mcp", Message: "needs a client block"}},
		},
		{
			name: "MCP with a client passes",
			edit: func(c *Config) { c.MCP, c.Client = &MCP{}, &Client{} },
		},
		{
			name: "Templates as text and as a file pass",
			edit: func(c *Config) {
				c.Templates = map[string]Template{
					"server.service-header":        {File: "./templates/header.txt"},
					"server.request-options-extra": {Text: "tenant.Info"},
					"server.response-data-extra":   {Text: "// see templates/data.tmpl"},
					"server.router-extra":          {Text: "r.Get(\"/health\", health)\nr.Get(\"/ready\", ready)\n"},
				}
			},
		},
		{
			name: "Template without text or a file",
			edit: func(c *Config) {
				c.Templates = map[string]Template{"server.service-header": {}, "server.router-extra": {Text: " \n\t"}}
			},
			issues: []Issue{
				{Key: "templates.server.router-extra", Message: "is empty, want the text or {file: <path>}"},
				{Key: "templates.server.service-header", Message: "is empty, want the text or {file: <path>}"},
			},
		},
		{
			name: "Template with text and a file",
			edit: func(c *Config) {
				c.Templates = map[string]Template{"server.service-header": {Text: "// Owned by platform.", File: "./header.tmpl"}}
			},
			issues: []Issue{{Key: "templates.server.service-header", Message: "takes the text or a file, not both"}},
		},
		{
			name: "Template text that is a path",
			edit: func(c *Config) {
				c.Templates = map[string]Template{
					"server.service-header":        {Text: "./header.tmpl"},
					"server.request-options-extra": {Text: "templates/fields.txt"},
					"server.response-data-extra":   {Text: "/etc/fields\n"},
					"server.router-extra":          {Text: "../routes.tmpl"},
				}
			},
			issues: []Issue{
				{Key: "templates.server.request-options-extra", Message: `"templates/fields.txt" is a path, want {file: templates/fields.txt}`},
				{Key: "templates.server.response-data-extra", Message: `"/etc/fields" is a path, want {file: /etc/fields}`},
				{Key: "templates.server.router-extra", Message: `"../routes.tmpl" is a path, want {file: ../routes.tmpl}`},
				{Key: "templates.server.service-header", Message: `"./header.tmpl" is a path, want {file: ./header.tmpl}`},
			},
		},
		{
			name: "Two output files that clean to the same path",
			edit: func(c *Config) {
				c.Output.Files = map[string][]string{"./api/a.go": {"models"}, "api/a.go": {"server"}}
			},
			issues: []Issue{{Key: "output.files", Message: `"./api/a.go" and "api/a.go" are the same file`}},
		},
		{
			name: "Invalid selectors",
			edit: func(c *Config) {
				c.Output.Files = map[string][]string{
					"a.go": {"", "model", "models.", "models.Types", "models.types.x", "plugin.a.b.c"},
				}
			},
			issues: []Issue{
				{Key: `output.files["a.go"]`, Message: `invalid selector "", want a form like models, models.types or plugin.<name>.<part>`},
				{Key: `output.files["a.go"]`, Message: `invalid selector "model", want a form like models, models.types or plugin.<name>.<part>`},
				{Key: `output.files["a.go"]`, Message: `invalid selector "models.", want a form like models, models.types or plugin.<name>.<part>`},
				{Key: `output.files["a.go"]`, Message: `invalid selector "models.Types", want a form like models, models.types or plugin.<name>.<part>`},
				{Key: `output.files["a.go"]`, Message: `invalid selector "models.types.x", want a form like models, models.types or plugin.<name>.<part>`},
				{Key: `output.files["a.go"]`, Message: `invalid selector "plugin.a.b.c", want a form like models, models.types or plugin.<name>.<part>`},
			},
		},
		{
			name: "Selector listed in two files",
			edit: func(c *Config) {
				c.Output.Files = map[string][]string{"b.go": {"models.types"}, "a.go": {"models.types"}}
			},
			issues: []Issue{{Key: `output.files["b.go"]`, Message: `"models.types" is already listed in "a.go"`}},
		},
		{
			name: "Selector listed twice in one file",
			edit: func(c *Config) {
				c.Output.Files = map[string][]string{"a.go": {"server", "server"}}
			},
			issues: []Issue{{Key: `output.files["a.go"]`, Message: `"server" is already listed in "a.go"`}},
		},
		{
			name: "Same selector at different specificity passes",
			edit: func(c *Config) {
				c.Output.Files = map[string][]string{"a.go": {"models"}, "b.go": {"models.types"}}
			},
		},
		{
			name: "Two package folders that clean to the same path",
			edit: func(c *Config) {
				c.Output.Packages = map[string]string{"./models": "models", "models/": "types"}
			},
			issues: []Issue{{Key: "output.packages", Message: `"./models" and "models/" are the same folder`}},
		},
		{
			name: "Invalid package name for a folder",
			edit: func(c *Config) {
				c.Output.Packages = map[string]string{"./models": "go-models"}
			},
			issues: []Issue{{Key: `output.packages["./models"]`, Message: `"go-models" is not a valid Go package name`}},
		},
		{
			name: "Every problem is listed at once",
			edit: func(c *Config) {
				c.Package = ""
				c.Server = &Server{Framework: "express"}
				c.Output.Files = map[string][]string{"a.go": {"nope"}}
			},
			issues: []Issue{
				{Key: "package", Message: `"" is not a valid Go package name`},
				{Key: "server.framework", Message: `"express" is not one of beego, chi, echo, echo-v5, fasthttp, fiber, gin, go-zero, goframe, gorilla-mux, hertz, iris, kratos, std-http`},
				{Key: `output.files["a.go"]`, Message: `invalid selector "nope", want a form like models, models.types or plugin.<name>.<part>`},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			cfg := defaulted(Config{dir: "/work"})
			tc.edit(cfg)

			err := cfg.Validate()

			if tc.issues == nil {
				assert.NoError(t, err)
				return
			}
			assert.Equal(t, &ValidationError{Issues: tc.issues}, err)
		})
	}
}

func TestApplyDefaults(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		cfg  Config
		want Config
	}{
		{
			name: "Empty config gets every default",
			cfg:  Config{dir: "/work/pets"},
			want: Config{
				Spec:    Spec{Prune: new(true)},
				Package: "pets",
				Header:  defaultHeader,
				Naming:  Naming{EnumPrefix: new(true)},
				Models:  &Models{IntType: "int", Descriptions: new(true)},
				Output:  Output{File: "./gen.go", Format: new(true)},
				dir:     "/work/pets",
			},
		},
		{
			name: "Package comes from the folder of the output file",
			cfg:  Config{Output: Output{File: "./internal/api-v2/gen.go"}, dir: "/work"},
			want: Config{
				Spec:    Spec{Prune: new(true)},
				Package: "apiv2",
				Header:  defaultHeader,
				Naming:  Naming{EnumPrefix: new(true)},
				Models:  &Models{IntType: "int", Descriptions: new(true)},
				Output:  Output{File: "./internal/api-v2/gen.go", Format: new(true)},
				dir:     "/work",
			},
		},
		{
			name: "Server and client blocks get their defaults",
			cfg:  Config{Server: &Server{Framework: "chi"}, Client: &Client{}, dir: "/work"},
			want: Config{
				Spec:    Spec{Prune: new(true)},
				Package: "work",
				Header:  defaultHeader,
				Naming:  Naming{EnumPrefix: new(true)},
				Models:  &Models{IntType: "int", Descriptions: new(true)},
				Server: &Server{
					Framework:          "chi",
					Name:               "Service",
					MultipartMaxMemory: 32 << 20,
					Scaffold:           Scaffold{Port: 8080, Timeout: Duration(30 * time.Second)},
				},
				Client: &Client{Name: "Client", Timeout: Duration(3 * time.Second)},
				Output: Output{File: "./gen.go", Format: new(true)},
				dir:    "/work",
			},
		},
		{
			name: "Set values are kept",
			cfg: Config{
				Spec:    Spec{Prune: new(false)},
				Package: "petstore",
				Header:  "Copyright 2026 Acme.",
				Naming:  Naming{EnumPrefix: new(false)},
				Models:  &Models{IntType: "int64", Descriptions: new(false)},
				Server: &Server{
					Name:               "Pets",
					MultipartMaxMemory: 1024,
					Scaffold:           Scaffold{Port: 9090, Timeout: Duration(time.Second)},
				},
				Client: &Client{Name: "PetClient", Timeout: Duration(time.Minute)},
				Output: Output{File: "./api/pets.go", Format: new(false)},
				dir:    "/work",
			},
			want: Config{
				Spec:    Spec{Prune: new(false)},
				Package: "petstore",
				Header:  "Copyright 2026 Acme.",
				Naming:  Naming{EnumPrefix: new(false)},
				Models:  &Models{IntType: "int64", Descriptions: new(false)},
				Server: &Server{
					Name:               "Pets",
					MultipartMaxMemory: 1024,
					Scaffold:           Scaffold{Port: 9090, Timeout: Duration(time.Second)},
				},
				Client: &Client{Name: "PetClient", Timeout: Duration(time.Minute)},
				Output: Output{File: "./api/pets.go", Format: new(false)},
				dir:    "/work",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			tc.cfg.applyDefaults()
			assert.Equal(t, tc.want, tc.cfg)
		})
	}
}
