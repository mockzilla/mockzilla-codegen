// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package framework

import "errors"

var (
	ErrPattern = errors.New("the router rejects the path")
	ErrMethod  = errors.New("the router does not take the method")
)
