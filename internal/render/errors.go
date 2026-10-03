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
	ErrNoValue     = errors.New("a value that is not set was written as <no value>")
)

// overrideError is how a block override failed, kept apart from what the template that ran the
// override adds to it.
type overrideError struct {
	err error
}

func (e *overrideError) Error() string {
	return e.err.Error()
}
