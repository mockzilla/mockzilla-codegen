// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

// Package sample is a plugin that adds a GenerateResponse field to the request options of every
// operation, typed with the response data of that operation, a part that lists the routes and
// makes empty success bodies, a part that wraps the service to set the field, and a service
// scaffold whose operations answer with what GenerateResponse makes.
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

//go:embed wrapper.tmpl
var wrapperTemplate string

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

// wrapper is what the wrapper part runs on: the service interface and every method of it.
type wrapper struct {
	Service codegen.TypeRef
	Calls   []call
}

// call is one method of the service. HasBody says whether the register part makes the body of
// its response.
type call struct {
	ID      string
	Options codegen.TypeRef
	Data    codegen.TypeRef
	HasBody bool
}

// Plugin is the sample plugin.
type Plugin struct{}

func (Plugin) Name() string {
	return "sample"
}

// Reserve names what the parts declare.
func (Plugin) Reserve() codegen.Reservations {
	return codegen.Reservations{Idents: []string{"Route", "Routes", "Bodies", "Register", "WithBodies"}}
}

// Contribute lists the routed operations for the register part and replaces the service scaffold.
// When the config has a server, it also gives every operation its GenerateResponse field and
// wraps the service to set it: that template asks the status func for the status of an operation.
func (Plugin) Contribute(api *codegen.API) (*codegen.Contribution, error) {
	var routes []route
	wrap := wrapper{Service: api.Service, Calls: make([]call, 0, len(api.Operations))}
	fields := make(map[string][]codegen.FieldSpec, len(api.Operations))
	statuses := make(map[string]int, len(api.Operations))
	for _, op := range api.Operations {
		statuses[op.ID] = defaultStatus
		if op.Success != nil {
			statuses[op.ID] = op.Success.Status
		}

		hasBody := op.IsRouted && op.Success != nil && op.Success.Body.Name != ""
		wrap.Calls = append(wrap.Calls, call{ID: op.ID, Options: op.RequestOptions, Data: op.ResponseData, HasBody: hasBody})
		fields[op.ID] = []codegen.FieldSpec{{
			Name: "GenerateResponse",
			Type: codegen.TypeRef{Name: "func() (*" + op.ResponseData.Name + ", error)"},
			Doc:  "GenerateResponse makes the response, when the service is asked for one.",
		}}
		if !op.IsRouted {
			continue
		}

		r := route{ID: op.ID, Method: op.Method, Path: op.Path, Status: statuses[op.ID]}
		if hasBody {
			r.HasBody, r.Body = true, op.Success.Body
		}
		routes = append(routes, r)
	}

	c := &codegen.Contribution{
		Parts:     []codegen.PartSource{{Name: "register", Template: registerTemplate, Data: routes}},
		Scaffolds: map[codegen.ScaffoldKind]string{codegen.ScaffoldService: serviceTemplate},
		Funcs:     template.FuncMap{"status": func(id string) int { return statuses[id] }},
	}
	if api.Service.Name != "" {
		c.Parts = append(c.Parts, codegen.PartSource{Name: "wrapper", Template: wrapperTemplate, Data: wrap})
		c.RequestOptionFields = fields
	}
	return c, nil
}
