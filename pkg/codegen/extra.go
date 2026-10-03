// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

// The extra files of the config, rendered with the expr, import and symbol funcs.

package codegen

import (
	"fmt"
	"go/token"
	"path/filepath"
	"text/template"

	"github.com/mockzilla/mockzilla-codegen/internal/gocode"
	"github.com/mockzilla/mockzilla-codegen/internal/gomodel"
	"github.com/mockzilla/mockzilla-codegen/internal/layout"
	"github.com/mockzilla/mockzilla-codegen/internal/render"
)

// renderExtra runs text, the template of the extra file id, on data and checks that it wrote Go
// declarations. Its expr, import and symbol funcs write for the file of s.
func renderExtra(id layout.PartID, text string, data any, s *gocode.Scope) ([]byte, error) {
	funcs := template.FuncMap{
		"expr": func(t TypeRef) (string, error) {
			if err := t.check(); err != nil {
				return "", err
			}
			return s.Qualified(t.Name, gomodel.Import{Path: t.ImportPath, Alias: t.Package}), nil
		},
		"import": func(path string) (string, error) {
			if path == "" {
				return "", fmt.Errorf("%w without a path", errImport)
			}
			return s.Import(gomodel.Import{Path: path}), nil
		},
		"symbol": func(part, name string) (string, error) {
			return symbol(s, layout.PartID(part), name)
		},
	}

	out, err := render.RenderSource(render.Source{Name: string(id), Text: text, Funcs: funcs}, data)
	if err == nil {
		err = gocode.CheckDecls(string(id), out)
	}
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrExtraFile, err)
	}
	return out, nil
}

// symbol writes name, which the code of part declares, as the file of s spells it. Whether part
// declares name is left to the compiler.
func symbol(s *gocode.Scope, part layout.PartID, name string) (string, error) {
	f := s.Layout.FileOf(part)
	switch {
	case f == nil:
		return "", fmt.Errorf("%w %q: the config writes no part %q", errSymbol, name, part)
	case !token.IsIdentifier(name):
		return "", fmt.Errorf("%w %q of %s is no identifier", errSymbol, name, part)
	case !token.IsExported(name) && filepath.Dir(f.Path) != filepath.Dir(s.File.Path):
		return "", fmt.Errorf("%w %s of %s is not exported, so no file outside the folder of %s can use it", errSymbol, name, part, f.Rel)
	}
	return s.Symbol(part, name), nil
}
