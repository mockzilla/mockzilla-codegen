// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package gocode

import "errors"

var (
	ErrFormat = errors.New("format generated code")
	ErrParse  = errors.New("parse generated code")
)
