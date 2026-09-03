// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

// Package sample is a plugin that adds a GenerateResponse field to every request options struct,
// a part that lists the routes and makes empty success bodies, and a service scaffold whose
// operations answer with what GenerateResponse makes.
package sample

import (
	_ "embed"
	"text/template"

	"github.com/mockzilla/mockzilla-codegen/pkg/codegen"
)

// defaultStatus is what the scaffold answers for an operation without a 2xx response.
const defaultStatus = 200

//go:embed register.tmpl
var registerTemplate string

//go:embed service.tmpl
var serviceTemplate string

var _ codegen.Plugin = Plugin{}

// route is one routed operation as the register part lists it. Body is the type of its success
// body, which HasBody says is set.
type route struct {
	ID      string
	Method  string
	Path    string
	Status  int
	HasBody bool
	Body    codegen.TypeRef
}

// Plugin is the sample plugin.
type Plugin struct{}

func (Plugin) Name() string {
	return "sample"
}

// Reserve names what the register part declares and the field every request options struct gets.
func (Plugin) Reserve() codegen.Reservations {
	return codegen.Reservations{
		Idents: []string{"Route", "Routes", "Bodies", "Register"},
		RequestOptionFields: []codegen.FieldSpec{{
			Name: "GenerateResponse",
			Type: codegen.TypeRef{Name: "func() any"},
			Doc:  "GenerateResponse makes the body of the response, when the service is asked for one.",
		}},
	}
}

// Contribute lists the routed operations for the register part and replaces the service
// scaffold, whose template asks the status func for the status of an operation.
func (Plugin) Contribute(api *codegen.API) (*codegen.Contribution, error) {
	var routes []route
	statuses := make(map[string]int, len(api.Operations))
	for _, op := range api.Operations {
		statuses[op.ID] = defaultStatus
		if op.Success != nil {
			statuses[op.ID] = op.Success.Status
		}
		if !op.IsRouted {
			continue
		}

		r := route{ID: op.ID, Method: op.Method, Path: op.Path, Status: statuses[op.ID]}
		if op.Success != nil && op.Success.Body.Name != "" {
			r.HasBody, r.Body = true, op.Success.Body
		}
		routes = append(routes, r)
	}

	return &codegen.Contribution{
		Parts:     []codegen.PartSource{{Name: "register", Template: registerTemplate, Data: routes}},
		Scaffolds: map[codegen.ScaffoldKind]string{codegen.ScaffoldService: serviceTemplate},
		Funcs:     template.FuncMap{"status": func(id string) int { return statuses[id] }},
	}, nil
}
