// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package config

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestApplyDefaults(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		cfg  Config
		want Config
	}{
		{
			name: "Empty config gets every default",
			cfg:  Config{dir: "/work/pets"},
			want: Config{
				Spec:    Spec{Prune: new(true)},
				Package: "pets",
				Naming:  Naming{EnumPrefix: new(true)},
				Models:  &Models{IntType: "int", Descriptions: new(true)},
				Output:  Output{File: "./gen.go", Format: new(true)},
				dir:     "/work/pets",
			},
		},
		{
			name: "Package comes from the folder of the output file",
			cfg:  Config{Output: Output{File: "./internal/api-v2/gen.go"}, dir: "/work"},
			want: Config{
				Spec:    Spec{Prune: new(true)},
				Package: "apiv2",
				Naming:  Naming{EnumPrefix: new(true)},
				Models:  &Models{IntType: "int", Descriptions: new(true)},
				Output:  Output{File: "./internal/api-v2/gen.go", Format: new(true)},
				dir:     "/work",
			},
		},
		{
			name: "Server and client blocks get their defaults",
			cfg:  Config{Server: &Server{Framework: "chi"}, Client: &Client{}, dir: "/work"},
			want: Config{
				Spec:    Spec{Prune: new(true)},
				Package: "work",
				Naming:  Naming{EnumPrefix: new(true)},
				Models:  &Models{IntType: "int", Descriptions: new(true)},
				Server: &Server{
					Framework:          "chi",
					Name:               "Service",
					MultipartMaxMemory: 32 << 20,
					Scaffold:           Scaffold{Port: 8080, Timeout: Duration(30 * time.Second)},
				},
				Client: &Client{Name: "Client", Timeout: Duration(3 * time.Second)},
				Output: Output{File: "./gen.go", Format: new(true)},
				dir:    "/work",
			},
		},
		{
			name: "Set values are kept",
			cfg: Config{
				Spec:    Spec{Prune: new(false)},
				Package: "petstore",
				Naming:  Naming{EnumPrefix: new(false)},
				Models:  &Models{IntType: "int64", Descriptions: new(false)},
				Server: &Server{
					Name:               "Pets",
					MultipartMaxMemory: 1024,
					Scaffold:           Scaffold{Port: 9090, Timeout: Duration(time.Second)},
				},
				Client: &Client{Name: "PetClient", Timeout: Duration(time.Minute)},
				Output: Output{File: "./api/pets.go", Format: new(false)},
				dir:    "/work",
			},
			want: Config{
				Spec:    Spec{Prune: new(false)},
				Package: "petstore",
				Naming:  Naming{EnumPrefix: new(false)},
				Models:  &Models{IntType: "int64", Descriptions: new(false)},
				Server: &Server{
					Name:               "Pets",
					MultipartMaxMemory: 1024,
					Scaffold:           Scaffold{Port: 9090, Timeout: Duration(time.Second)},
				},
				Client: &Client{Name: "PetClient", Timeout: Duration(time.Minute)},
				Output: Output{File: "./api/pets.go", Format: new(false)},
				dir:    "/work",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			tc.cfg.applyDefaults()
			assert.Equal(t, tc.want, tc.cfg)
		})
	}
}

func TestPackageName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		dir  string
		want string
	}{
		{name: "Plain folder name is kept", dir: "/work/api", want: "api"},
		{name: "Upper case is lowered", dir: "/work/PetStore", want: "petstore"},
		{name: "Separators are dropped", dir: "/work/pet-store_v2.go", want: "petstorev2go"},
		{name: "Leading digits are dropped", dir: "/work/2fa", want: "fa"},
		{name: "Only digits falls back", dir: "/work/2026", want: "api"},
		{name: "Non-ASCII letters are dropped", dir: "/work/café", want: "caf"},
		{name: "Keyword falls back", dir: "/work/type", want: "api"},
		{name: "Current dir falls back", dir: ".", want: "api"},
		{name: "Root dir falls back", dir: "/", want: "api"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.want, packageName(tc.dir))
		})
	}
}
