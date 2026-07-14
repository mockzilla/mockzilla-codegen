// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package libopenapi

import (
	"io"
	"log/slog"
)

type Option func(*Provider)

// WithDebug sends libopenapi's own logs to w.
func WithDebug(w io.Writer) Option {
	return func(p *Provider) {
		p.logger = slog.New(slog.NewTextHandler(w, &slog.HandlerOptions{Level: slog.LevelDebug}))
	}
}
