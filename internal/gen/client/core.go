// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

// The data of core.tmpl: the client type, its options and the default timeout.

package client

import (
	"github.com/mockzilla/mockzilla-codegen/internal/gocode"
	"github.com/mockzilla/mockzilla-codegen/internal/gomodel"
)

// CoreView is the data of the core part: the client type, its option type, the names the
// packages are imported under, and the default timeout as a duration expression.
type CoreView struct {
	Name    string
	Option  string
	Context string
	HTTP    string
	URL     string
	Runtime string
	Timeout string
	User    map[string]any
}

func coreView(g *Generator, s *gocode.Scope) *CoreView {
	return &CoreView{
		Name:    g.opts.Name,
		Option:  g.opts.Name + "Option",
		Context: s.Import(gomodel.Import{Path: "context"}),
		HTTP:    s.Import(gomodel.Import{Path: "net/http"}),
		URL:     s.Import(gomodel.Import{Path: "net/url"}),
		Runtime: s.Import(gomodel.Import{Path: gomodel.RuntimePath}),
		Timeout: gocode.Duration(g.opts.Timeout, s.Import(gomodel.Import{Path: "time"})),
		User:    g.opts.User,
	}
}
