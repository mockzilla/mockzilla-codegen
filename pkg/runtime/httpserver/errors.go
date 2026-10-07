// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package httpserver

import "errors"

var (
	ErrNoResponse  = errors.New("the service returned no response")
	ErrResponseCut = errors.New("response cut short")
	ErrPanic       = errors.New("panic")
)
