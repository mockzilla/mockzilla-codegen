// Copyright 2026 Mockzilla
// SPDX-License-Identifier: MIT

package config

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
)

type sample struct {
	Kind  string `yaml:"kind" enum:"a,b" desc:"Which kind."`
	Count int    `yaml:"count"`
}

func TestFieldsOf(t *testing.T) {
	t.Parallel()

	assert.Equal(t, []field{
		{key: "kind", desc: "Which kind.", enum: []string{"a", "b"}, typ: reflect.TypeFor[string]()},
		{key: "count", typ: reflect.TypeFor[int]()},
	}, fieldsOf(reflect.TypeFor[sample]()))
}

func TestFieldsOfSkipsUnexported(t *testing.T) {
	t.Parallel()

	var keys []string
	for _, f := range fieldsOf(reflect.TypeFor[Config]()) {
		keys = append(keys, f.key)
	}

	assert.Equal(t, []string{
		"spec", "package", "header", "naming", "imports", "models",
		"server", "client", "mcp", "templates", "user-context", "output",
	}, keys)
}

func TestEnumOf(t *testing.T) {
	t.Parallel()

	assert.Equal(t, []string{"a", "b"}, enumOf(reflect.TypeFor[sample](), "Kind"))
}
