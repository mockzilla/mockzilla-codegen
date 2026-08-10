// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package itest

import "errors"

var (
	ErrSpecNotFound = errors.New("spec not found")
	ErrCommand      = errors.New("command failed")
	ErrTimeout      = errors.New("timed out")
	ErrCache        = errors.New("read cache")
	ErrKnownLine    = errors.New("bad known-failures line")
)
