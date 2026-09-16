// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package layout

import "errors"

var (
	ErrUnknownSelector  = errors.New("unknown selector")
	ErrSelectorConflict = errors.New("selector conflict")
	ErrNoModule         = errors.New("no module path")
	ErrOutsideModule    = errors.New("outside the module")
	ErrImportCycle      = errors.New("import cycle")
	ErrModule           = errors.New("read go.mod")
	ErrPackageConflict  = errors.New("two packages in one folder")
	ErrSplitParts       = errors.New("parts that belong together are in different folders")

	errNoModuleLine = errors.New("no module line")
)
