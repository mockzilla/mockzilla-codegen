// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package httpclient

import "errors"

var (
	ErrBaseURL   = errors.New("invalid base URL")
	ErrFrame     = errors.New("invalid stream frame")
	ErrFrameSize = errors.New("stream frame too large")
)
