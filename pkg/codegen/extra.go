// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package codegen

import (
	"maps"
	"text/template"

	"github.com/mockzilla/mockzilla-codegen/internal/gocode"
	"github.com/mockzilla/mockzilla-codegen/internal/gomodel"
	"github.com/mockzilla/mockzilla-codegen/internal/layout"
	"github.com/mockzilla/mockzilla-codegen/internal/render"
)

// renderSource runs text, the template of part id, on data and checks that it wrote Go
// declarations. Its expr, import and symbol funcs write for the file of s; one of more with the
// same name replaces it.
func renderSource(id layout.PartID, text string, more template.FuncMap, data any, s *gocode.Scope) ([]byte, error) {
	funcs := make(template.FuncMap, len(more)+3)
	funcs["expr"] = func(t TypeRef) (string, error) {
		if err := t.check(); err != nil {
			return "", err
		}
		return s.Qualified(t.Name, gomodel.Import{Path: t.ImportPath, Alias: t.Package}), nil
	}
	funcs["import"] = func(path string) (string, error) {
		if err := (Import{Path: path}).check(); err != nil {
			return "", err
		}
		return s.Import(gomodel.Import{Path: path}), nil
	}
	funcs["symbol"] = func(part, name string) (string, error) {
		return symbol(s, layout.PartID(part), name)
	}
	maps.Copy(funcs, more)

	out, err := render.RenderSource(render.Source{Name: string(id), Text: text, Funcs: funcs}, data)
	if err == nil {
		err = gocode.CheckDecls(string(id), out)
	}
	if err != nil {
		return nil, err
	}
	return out, nil
}
