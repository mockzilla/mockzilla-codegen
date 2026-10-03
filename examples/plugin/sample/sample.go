// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

// Package sample is a plugin that adds a GenerateResponse field to the request options of every
// operation, typed with the response data of that operation, a part that lists the routes, a part
// that wraps the service to set the field to the success response with an empty body, and a
// service scaffold whose operations answer with what GenerateResponse makes.
package sample

import (
	_ "embed"
	"strings"

	"github.com/mockzilla/mockzilla-codegen/pkg/codegen"
)

// defaultStatus is what an operation without a 2xx response answers, without a body.
const defaultStatus = 200

//go:embed register.tmpl
var registerTemplate string

//go:embed service.tmpl
var serviceTemplate string

//go:embed wrapper.tmpl
var wrapperTemplate string

var _ codegen.Plugin = Plugin{}

// route is one routed operation as the register part lists it.
type route struct {
	ID     string
	Method string
	Path   string
	Status int
}

// wrapper is what the wrapper part runs on: the service interface and every method of it.
type wrapper struct {
	Service codegen.TypeRef
	Calls   []call
}

// call is one method of the service and the response it is set to answer: made by Constructor
// when the operation has a success response, else Status alone. Body is the type the constructor
// takes, or the type it points to when IsPointer is set.
type call struct {
	ID           string
	Options      codegen.TypeRef
	Data         codegen.TypeRef
	Status       int
	Constructor  codegen.TypeRef
	HasStatusArg bool
	Body         codegen.TypeRef
	IsPointer    bool
}

// Plugin is the sample plugin.
type Plugin struct{}

func (Plugin) Name() string {
	return "sample"
}

// Reserve names what the parts declare.
func (Plugin) Reserve() codegen.Reservations {
	return codegen.Reservations{Idents: []string{"Route", "Routes", "Register", "WithBodies"}}
}

// Contribute lists the routed operations for the register part and replaces the service scaffold.
// When the config has a server, it also gives every operation its GenerateResponse field and
// wraps the service to set it.
func (Plugin) Contribute(api *codegen.API) (*codegen.Contribution, error) {
	var routes []route
	wrap := wrapper{Service: api.Service, Calls: make([]call, 0, len(api.Operations))}
	fields := make(map[string][]codegen.FieldSpec, len(api.Operations))
	for _, op := range api.Operations {
		answer := newCall(op)
		wrap.Calls = append(wrap.Calls, answer)
		fields[op.ID] = []codegen.FieldSpec{{
			Name: "GenerateResponse",
			Type: codegen.TypeRef{Name: "func() (*" + op.ResponseData.Name + ", error)"},
			Doc:  "GenerateResponse makes the response, when the service is asked for one.",
		}}
		if op.IsRouted {
			routes = append(routes, route{ID: op.ID, Method: op.Method, Path: op.Path, Status: answer.Status})
		}
	}

	c := &codegen.Contribution{
		Parts:     []codegen.PartSource{{Name: "register", Template: registerTemplate, Data: routes}},
		Scaffolds: map[codegen.ScaffoldKind]string{codegen.ScaffoldService: serviceTemplate},
	}
	if api.Service.Name != "" {
		c.Parts = append(c.Parts, codegen.PartSource{Name: "wrapper", Template: wrapperTemplate, Data: wrap})
		c.RequestOptionFields = fields
	}
	return c, nil
}

func newCall(op codegen.Operation) call {
	c := call{ID: op.ID, Options: op.RequestOptions, Data: op.ResponseData, Status: defaultStatus}
	if s := op.Success; s != nil {
		c.Status, c.Constructor, c.HasStatusArg, c.Body = s.Code, s.Constructor, s.HasStatusArg, s.Body
	}

	// For a pointer body keep the type it points to, so the wrapper passes new(T) and not nil.
	if elem, ok := strings.CutPrefix(c.Body.Name, "*"); ok {
		c.Body.Name, c.IsPointer = elem, true
	}
	return c
}
