// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

// The data of core.tmpl: the client type, its options and the default timeout.

package client

import (
	"slices"

	"github.com/mockzilla/mockzilla-codegen/internal/gocode"
	"github.com/mockzilla/mockzilla-codegen/internal/gomodel"
)

// CoreView is the data of the core part: the client type, its option type, the names the
// packages are imported under, the default timeout as a duration expression, empty for none, and
// whether the client has a Stream method.
type CoreView struct {
	Name       string
	Option     string
	Context    string
	HTTP       string
	Time       string
	URL        string
	Runtime    string
	Timeout    string
	HasStreams bool
	User       map[string]any
}

func coreView(g *Generator, s *gocode.Scope) *CoreView {
	v := &CoreView{
		Name:       g.opts.Name,
		Option:     g.opts.Name + "Option",
		Context:    s.Import(gomodel.Import{Path: "context"}),
		HTTP:       s.Import(gomodel.Import{Path: "net/http"}),
		Time:       s.Import(gomodel.Import{Path: "time"}),
		URL:        s.Import(gomodel.Import{Path: "net/url"}),
		Runtime:    s.Import(gomodel.Import{Path: gomodel.RuntimePath}),
		HasStreams: g.opts.HasStreams && slices.ContainsFunc(g.ops, hasStream),
		User:       g.opts.User,
	}
	if g.opts.Timeout > 0 {
		v.Timeout = gocode.Duration(g.opts.Timeout, v.Time)
	}
	return v
}

func hasStream(op *gomodel.Operation) bool {
	_, _, ok := streamBody(op)
	return ok
}
