// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package main

import "errors"

var (
	errNoMode       = errors.New("profile has no mode line")
	errBadLine      = errors.New("malformed profile line")
	errNoModule     = errors.New("go.mod has no module line")
	errBadPattern   = errors.New("bad ignore pattern")
	errBelowMinimum = errors.New("coverage below minimum")
)
