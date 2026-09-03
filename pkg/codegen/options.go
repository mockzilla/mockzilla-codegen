// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package codegen

import (
	"github.com/mockzilla/mockzilla-codegen/internal/provider"
	"github.com/mockzilla/mockzilla-codegen/internal/provider/libopenapi"
)

type Option func(*options)

type options struct {
	spec     []byte
	provider provider.Provider
	plugins  []Plugin
}

// WithSpec passes the spec in memory instead of reading spec.path. When spec.path is set it still
// names the spec, so refs to other files resolve next to it; otherwise they resolve against the
// config's folder.
func WithSpec(data []byte) Option {
	return func(o *options) {
		o.spec = data
	}
}

// WithPlugins adds plugins, which reserve names and contribute code in the order given.
func WithPlugins(plugins ...Plugin) Option {
	return func(o *options) {
		o.plugins = append(o.plugins, plugins...)
	}
}

// withProvider replaces the spec reader; tests use it to reach failures a real spec cannot cause.
func withProvider(p provider.Provider) Option {
	return func(o *options) {
		o.provider = p
	}
}

func newOptions(opts []Option) options {
	o := options{provider: libopenapi.New()}
	for _, opt := range opts {
		opt(&o)
	}
	return o
}
