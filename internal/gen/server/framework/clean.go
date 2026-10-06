// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

// The clean-path check of routers that clean a path before they match it.

package framework

import (
	"fmt"
	"path"
	"strings"
)

// CheckClean fails on a path that is not clean, such as /a//b; a trailing slash is clean.
func CheckClean(p string) error {
	clean := path.Clean(p)
	if strings.HasSuffix(p, "/") && clean != "/" {
		clean += "/"
	}
	if clean != p {
		return fmt.Errorf("%w: it is not a clean path", ErrPattern)
	}
	return nil
}
