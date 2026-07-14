// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package provider

import "errors"

var (
	ErrProviderPanic      = errors.New("provider panic")
	ErrParse              = errors.New("parse spec")
	ErrUnsupportedVersion = errors.New("unsupported OpenAPI version")
	ErrBundle             = errors.New("bundle spec")
	ErrOverlay            = errors.New("apply overlay")
)
