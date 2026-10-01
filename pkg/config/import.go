// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package config

import (
	"cmp"

	"github.com/mockzilla/mockzilla-codegen/internal/naming"
)

const (
	blankAlias = "_"
	dotAlias   = "."
)

// Import is a package that Go code the generator does not write itself refers to: the text of a
// template override, an x-go-type. A generated file imports it when its code names it.
type Import struct {
	Package string `yaml:"package" desc:"Import path."`
	Alias   string `yaml:"alias" desc:"Name the code refers to the package by. Defaults to the name the path gives. With _ every generated file imports the package, for its side effects."`
}

// Name is the name code refers to the package by: the alias, else the one the path gives. It is
// empty under _ and for a path that gives no name.
func (imp Import) Name() string {
	if imp.Alias == blankAlias {
		return ""
	}
	return cmp.Or(imp.Alias, naming.ImportName(imp.Package))
}
