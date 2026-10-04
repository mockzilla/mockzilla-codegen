// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package codegen

import "errors"

var (
	ErrWrite        = errors.New("write generated file")
	ErrFramework    = errors.New("no router for the framework")
	ErrTemplateFile = errors.New("read template file")
	ErrExtraFile    = errors.New("extra file")
	ErrNameClash    = errors.New("name declared twice")

	errTypeRef = errors.New("type")
	errImport  = errors.New("import")
	errSymbol  = errors.New("symbol")
)
