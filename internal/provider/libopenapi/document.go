// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package libopenapi

import (
	"slices"

	"github.com/pb33f/libopenapi/datamodel/high/base"
	v3 "github.com/pb33f/libopenapi/datamodel/high/v3"

	"github.com/mockzilla/mockzilla-codegen/internal/oasdoc"
	"github.com/mockzilla/mockzilla-codegen/internal/spec"
)

func oauthFlow(kind string, f *v3.OAuthFlow) spec.OAuthFlow {
	out := spec.OAuthFlow{
		Kind:             kind,
		AuthorizationURL: f.AuthorizationUrl,
		TokenURL:         f.TokenUrl,
		RefreshURL:       f.RefreshUrl,
	}
	for name, desc := range f.Scopes.FromOldest() {
		out.Scopes = append(out.Scopes, spec.Scope{Name: name, Description: desc})
	}
	return out
}

func servers(list []*v3.Server) []spec.Server {
	var out []spec.Server
	for _, s := range list {
		srv := spec.Server{URL: s.URL, Name: s.Name, Description: s.Description}
		for name, v := range s.Variables.FromOldest() {
			srv.Variables = append(srv.Variables, spec.ServerVariable{
				Name:        name,
				Default:     v.Default,
				Description: v.Description,
				Enum:        slices.Clone(v.Enum),
			})
		}
		out = append(out, srv)
	}
	return out
}

// securityRequirements keeps an explicit empty list non-nil: it switches security off.
func securityRequirements(list []*base.SecurityRequirement) []spec.SecurityRequirement {
	if list == nil {
		return nil
	}

	out := make([]spec.SecurityRequirement, 0, len(list))
	for _, r := range list {
		var req spec.SecurityRequirement
		for name, scopes := range r.Requirements.FromOldest() {
			req.Schemes = append(req.Schemes, spec.RequiredScheme{Name: name, Scopes: slices.Clone(scopes)})
		}
		out = append(out, req)
	}
	return out
}

func componentPointer(kind, name string) string {
	return "/components/" + kind + "/" + oasdoc.Escape(name)
}
