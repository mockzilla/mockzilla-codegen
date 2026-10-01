// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package config

import (
	"reflect"
	"strings"
)

type field struct {
	key  string
	desc string
	enum []string
	typ  reflect.Type
}

func fieldsOf(t reflect.Type) []field {
	fields := make([]field, 0, t.NumField())
	for f := range t.Fields() {
		key := f.Tag.Get("yaml")
		if !f.IsExported() || key == "-" {
			continue
		}

		var enum []string
		if tag := f.Tag.Get("enum"); tag != "" {
			enum = strings.Split(tag, ",")
		}
		fields = append(fields, field{
			key:  key,
			desc: f.Tag.Get("desc"),
			enum: enum,
			typ:  f.Type,
		})
	}
	return fields
}

func enumOf(t reflect.Type, name string) []string {
	f, _ := t.FieldByName(name)
	return strings.Split(f.Tag.Get("enum"), ",")
}
