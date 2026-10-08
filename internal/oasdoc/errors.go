// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package oasdoc

import "errors"

var (
	ErrParse              = errors.New("parse spec")
	ErrNotObject          = errors.New("spec root is not an object")
	ErrManyDocuments      = errors.New("spec holds more than one YAML document")
	ErrUnsupportedVersion = errors.New("unsupported OpenAPI version")
	ErrPointer            = errors.New("invalid JSON pointer")
	ErrNotFound           = errors.New("JSON pointer not found")
	ErrMarshal            = errors.New("marshal spec")
)
