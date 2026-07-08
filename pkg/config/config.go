// Copyright 2026 Mockzilla
// SPDX-License-Identifier: MIT

// Package config loads, checks and describes the codegen config file.
package config

// Config is the whole codegen config file.
type Config struct {
	Spec        Spec              `yaml:"spec" desc:"Input spec and how to prepare it before generation."`
	Package     string            `yaml:"package" desc:"Go package of the default output file. Defaults to its folder name."`
	Header      string            `yaml:"header" desc:"Text put as a comment at the top of every generated file."`
	Naming      Naming            `yaml:"naming" desc:"How Go names are built."`
	Imports     []Import          `yaml:"imports" desc:"Extra imports added to every generated file."`
	Models      *Models           `yaml:"models" desc:"Models generation. Always on."`
	Server      *Server           `yaml:"server" desc:"Server generation. Present means generate."`
	Client      *Client           `yaml:"client" desc:"Client generation. Present means generate."`
	MCP         *MCP              `yaml:"mcp" desc:"MCP server generation. Present means generate. Needs client."`
	Templates   map[string]string `yaml:"templates" desc:"Template overrides, keyed by block name."`
	UserContext map[string]any    `yaml:"user-context" desc:"Free-form values passed to templates."`
	Output      Output            `yaml:"output" desc:"Where generated files go."`

	dir string
}

// Spec is the input spec and the steps that prepare it.
type Spec struct {
	Path     string    `yaml:"path" desc:"Path or URL of the OpenAPI spec. A CLI argument or in-memory spec overrides it."`
	Overlays []string  `yaml:"overlays" desc:"Overlay files or URLs, applied in order."`
	Filter   Filter    `yaml:"filter" desc:"Parts of the spec to keep or drop."`
	Prune    *bool     `yaml:"prune" desc:"Remove components nothing refers to. Defaults to true."`
	Simplify *Simplify `yaml:"simplify" desc:"Rewrites that make the spec simpler to generate from."`
}

// Filter selects the parts of the spec to keep. Exclude wins over include.
type Filter struct {
	Include FilterSet `yaml:"include" desc:"Keep only what matches."`
	Exclude FilterSet `yaml:"exclude" desc:"Drop what matches. Wins over include."`
}

// FilterSet lists what a filter matches.
type FilterSet struct {
	Paths            []string            `yaml:"paths" desc:"Operation paths."`
	Tags             []string            `yaml:"tags" desc:"Operation tags."`
	OperationIDs     []string            `yaml:"operation-ids" desc:"Operation IDs."`
	Webhooks         []string            `yaml:"webhooks" desc:"Webhook names."`
	Extensions       []string            `yaml:"extensions" desc:"Extension names, such as x-internal."`
	SchemaProperties map[string][]string `yaml:"schema-properties" desc:"Properties per component schema name."`
}

// Simplify lists the spec rewrites to apply.
type Simplify struct {
	Unions             bool                `yaml:"unions" desc:"Collapse each oneOf and anyOf to one variant."`
	OptionalProperties *OptionalProperties `yaml:"optional-properties" desc:"Cap the optional properties kept per schema."`
}

// OptionalProperties sets how many optional properties survive simplification.
type OptionalProperties struct {
	Min  int   `yaml:"min" desc:"Fewest optional properties kept per schema."`
	Max  int   `yaml:"max" desc:"Most optional properties kept per schema."`
	Seed int64 `yaml:"seed" desc:"Seed for picking a count between min and max."`
}

// Naming controls how Go names are built.
type Naming struct {
	Initialisms []string `yaml:"initialisms" desc:"Extra initialisms kept upper case, such as PSP."`
	EnumPrefix  *bool    `yaml:"enum-prefix" desc:"Prefix enum constants with their type name. Defaults to true."`
}

// Import is one extra import for generated files.
type Import struct {
	Package string `yaml:"package" desc:"Import path."`
	Alias   string `yaml:"alias" desc:"Import alias. Empty means none."`
}

// Models controls model generation.
type Models struct {
	IntType      string            `yaml:"int-type" enum:"int,int32,int64" desc:"Go type for integers without a format. Defaults to int."`
	Descriptions *bool             `yaml:"descriptions" desc:"Copy schema descriptions into comments. Defaults to true."`
	ExtraTags    []string          `yaml:"extra-tags" desc:"Struct tags added next to json, such as yaml."`
	Validation   ModelValidation   `yaml:"validation" desc:"Generated Validate methods."`
	ErrorMapping map[string]string `yaml:"error-mapping" desc:"Error types, mapped to the path of their message field."`
}

// ModelValidation controls the generated Validate methods.
type ModelValidation struct {
	Skip     bool `yaml:"skip" desc:"Generate no Validate methods."`
	Response bool `yaml:"response" desc:"Also validate response-only types."`
}

// Server controls server generation.
type Server struct {
	Framework          string           `yaml:"framework" enum:"chi,std-http" desc:"HTTP framework to generate for."`
	Name               string           `yaml:"name" desc:"Name of the service interface. Defaults to Service."`
	Validation         ServerValidation `yaml:"validation" desc:"Checks the server runs on requests and responses."`
	MultipartMaxMemory ByteSize         `yaml:"multipart-max-memory" desc:"Memory for multipart forms before spilling to disk. Defaults to 32MB."`
	Scaffold           Scaffold         `yaml:"scaffold" desc:"Starter files, written once."`
}

// ServerValidation controls what the server validates.
type ServerValidation struct {
	Request  bool `yaml:"request" desc:"Validate requests before calling the service."`
	Response bool `yaml:"response" desc:"Validate responses before writing them."`
}

// Scaffold lists the starter files and their settings.
type Scaffold struct {
	Service    string   `yaml:"service" desc:"File for the service implementation stub."`
	Middleware string   `yaml:"middleware" desc:"File for the middleware stub."`
	Main       string   `yaml:"main" desc:"File for the main package that runs the server."`
	Overwrite  bool     `yaml:"overwrite" desc:"Write scaffold files even when they exist."`
	Port       int      `yaml:"port" desc:"Port the scaffolded server listens on. Defaults to 8080."`
	Timeout    Duration `yaml:"timeout" desc:"Request timeout of the scaffolded server. Defaults to 30s."`
}

// Client controls client generation.
type Client struct {
	Name         string   `yaml:"name" desc:"Name of the client type. Defaults to Client."`
	Timeout      Duration `yaml:"timeout" desc:"Default request timeout. Defaults to 3s."`
	WithResponse bool     `yaml:"with-response" desc:"Also generate methods that return the raw HTTP response."`
	Streaming    bool     `yaml:"streaming" desc:"Generate streaming methods for event-stream responses."`
}

// MCP controls MCP server generation.
type MCP struct {
	DefaultSkip bool `yaml:"default-skip" desc:"Skip operations unless x-mcp turns them on."`
}

// Output says where generated files go.
type Output struct {
	File     string              `yaml:"file" desc:"Default file for every part. Defaults to ./gen.go."`
	Files    map[string][]string `yaml:"files" desc:"Parts moved to other files, keyed by file path. The most specific selector wins."`
	Packages map[string]string   `yaml:"packages" desc:"Package names keyed by folder. Other folders use their own name."`
	Module   string              `yaml:"module" desc:"Module path, only needed when no go.mod is found."`
	Format   *bool               `yaml:"format" desc:"Format generated files with go/format. Defaults to true."`
}
