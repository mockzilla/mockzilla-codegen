// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package config

import (
	"cmp"
	"go/token"
	"path/filepath"
	"strings"
	"time"
)

const fallbackPackage = "api"

func (c *Config) applyDefaults() {
	c.Output.File = cmp.Or(c.Output.File, "./gen.go")
	c.Output.Format = cmp.Or(c.Output.Format, new(true))
	c.Package = cmp.Or(c.Package, packageName(c.Resolve(filepath.Dir(c.Output.File))))
	c.Spec.Prune = cmp.Or(c.Spec.Prune, new(true))
	c.Naming.EnumPrefix = cmp.Or(c.Naming.EnumPrefix, new(true))

	c.Models = cmp.Or(c.Models, &Models{})
	c.Models.IntType = cmp.Or(c.Models.IntType, "int")
	c.Models.Descriptions = cmp.Or(c.Models.Descriptions, new(true))

	if s := c.Server; s != nil {
		s.Name = cmp.Or(s.Name, "Service")
		s.MultipartMaxMemory = cmp.Or(s.MultipartMaxMemory, 32<<20)
		s.Scaffold.Port = cmp.Or(s.Scaffold.Port, 8080)
		s.Scaffold.Timeout = cmp.Or(s.Scaffold.Timeout, Duration(30*time.Second))
	}

	if cl := c.Client; cl != nil {
		cl.Name = cmp.Or(cl.Name, "Client")
		cl.Timeout = cmp.Or(cl.Timeout, Duration(3*time.Second))
	}
}

func packageName(dir string) string {
	name := strings.Map(func(r rune) rune {
		if 'a' <= r && r <= 'z' || '0' <= r && r <= '9' {
			return r
		}
		return -1
	}, strings.ToLower(filepath.Base(dir)))
	name = strings.TrimLeft(name, "0123456789")

	if name == "" || token.IsKeyword(name) {
		return fallbackPackage
	}
	return name
}
