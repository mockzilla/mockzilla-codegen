// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

// Package spec is the provider-neutral OpenAPI IR: ordered slices only, not changed after build.
package spec

type Version int

const (
	V30 Version = iota + 1
	V31
	V32
)

func (v Version) String() string {
	switch v {
	case V30:
		return "3.0"
	case V31:
		return "3.1"
	case V32:
		return "3.2"
	default:
		return "unknown"
	}
}

// Document is a parsed spec. Operations keep paths in source order and methods in MethodOrder.
type Document struct {
	Version    Version
	Info       Info
	Servers    []Server
	Operations []*Operation
	Webhooks   []*Operation
	Components Components
	Security   []SecurityRequirement
	Extensions []Extension
}

type Info struct {
	Title       string
	Summary     string
	Description string
	Version     string
	Extensions  []Extension
}

type Server struct {
	URL         string
	Name        string
	Description string
	Variables   []ServerVariable
}

type ServerVariable struct {
	Name        string
	Default     string
	Description string
	Enum        []string
}

// Components lists every reusable object in source order. Usages elsewhere share these pointers.
type Components struct {
	Schemas         []Named[*Schema]
	Responses       []Named[*Response]
	Parameters      []Named[*Parameter]
	RequestBodies   []Named[*RequestBody]
	Headers         []Named[*Header]
	SecuritySchemes []*SecurityScheme
}

// Named is a component with the key it has under components.
type Named[T any] struct {
	Name  string
	Value T
}

// SecurityRequirement is one alternative: every scheme in it applies. No schemes means anonymous.
type SecurityRequirement struct {
	Schemes []RequiredScheme
}

type RequiredScheme struct {
	Name   string
	Scopes []string
}

type SecurityScheme struct {
	Name              string
	Type              string
	Description       string
	In                string
	ParamName         string
	Scheme            string
	BearerFormat      string
	OpenIDConnectURL  string
	OAuth2MetadataURL string
	Flows             []OAuthFlow
	Deprecated        bool
	Extensions        []Extension
	Origin            Origin
}

// OAuthFlow is one oauth2 flow; Kind is its key, such as implicit or clientCredentials.
type OAuthFlow struct {
	Kind             string
	AuthorizationURL string
	TokenURL         string
	RefreshURL       string
	Scopes           []Scope
}

type Scope struct {
	Name        string
	Description string
}

// Origin is an object's JSON pointer in the parsed document and its position in the source file.
type Origin struct {
	Pointer string
	File    string
	Line    int
	Col     int
}

type Extension struct {
	Name  string
	Value Value
}
