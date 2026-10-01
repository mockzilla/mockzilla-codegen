// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package render

import "errors"

var (
	ErrTemplate    = errors.New("load templates")
	ErrUnknownPart = errors.New("no template for part")
	ErrExecute     = errors.New("render")
	ErrFunc        = errors.New("template func")
)
