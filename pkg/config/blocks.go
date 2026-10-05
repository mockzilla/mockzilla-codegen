// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

// The blocks of the config file, one struct each.

package config

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
	Extensions       []string            `yaml:"extensions" desc:"x- keys on component schemas, such as x-internal: include keeps only these, exclude drops them."`
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

// Models controls model generation.
type Models struct {
	IntType      string            `yaml:"int-type" enum:"int,int32,int64" desc:"Go type for integers without a format. Defaults to int."`
	Descriptions *bool             `yaml:"descriptions" desc:"Copy schema descriptions into comments. Defaults to true."`
	ExtraTags    []string          `yaml:"extra-tags" desc:"Struct tags added next to json, such as yaml."`
	Validation   ModelValidation   `yaml:"validation" desc:"Generated Validate methods."`
	ErrorMapping map[string]string `yaml:"error-mapping" desc:"Error types, mapped to the path of their message field."`
	FormatTypes  map[string]GoType `yaml:"format-types" desc:"Go types by schema format, such as uuid, in place of the built-in ones. x-go-type on a schema wins."`
}

// GoType is a Go type and the package it comes from.
type GoType struct {
	Type   string `yaml:"type" desc:"Go type, such as uuid.UUID or int64."`
	Import string `yaml:"import" desc:"Import path of the type's package, found as for x-go-type when left out."`
}

// ModelValidation controls the generated Validate methods.
type ModelValidation struct {
	Skip     bool `yaml:"skip" desc:"Generate no Validate methods."`
	Response bool `yaml:"response" desc:"Also generate ValidateResponse where readOnly and writeOnly fields make a response check other things than a request."`
}

// Server controls server generation.
type Server struct {
	Framework          string           `yaml:"framework" enum:"beego,chi,echo,echo-v5,fasthttp,fiber,gin,go-zero,goframe,gorilla-mux,hertz,iris,kratos,std-http" desc:"HTTP framework to generate for."`
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
	Name         string    `yaml:"name" desc:"Name of the client type. Defaults to Client."`
	Timeout      *Duration `yaml:"timeout" desc:"How long a call may take, 0s for no limit. A stream method waits that long for the response headers only. Defaults to 3s."`
	WithResponse bool      `yaml:"with-response" desc:"Also generate methods that return the raw HTTP response."`
	Streaming    bool      `yaml:"streaming" desc:"Also generate Stream methods that read text/event-stream and line-delimited JSON responses frame by frame."`
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
