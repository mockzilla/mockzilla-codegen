// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package prepare

import "errors"

var (
	ErrNoSpec = errors.New("no spec: set spec.path or pass the spec in memory")
	ErrRead   = errors.New("read")
)
