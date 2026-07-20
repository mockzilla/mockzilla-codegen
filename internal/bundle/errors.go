// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package bundle

import "errors"

var (
	ErrLoad     = errors.New("load referenced file")
	ErrTarget   = errors.New("ref target not found")
	ErrFragment = errors.New("ref fragment is not a JSON pointer")
	ErrCycle    = errors.New("ref cycle")
	ErrInline   = errors.New("inlined ref target is not an object")
)
