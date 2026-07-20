// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package codegen

import (
	"context"

	"github.com/mockzilla/codegen/internal/prepare"
	"github.com/mockzilla/codegen/internal/provider/libopenapi"
	"github.com/mockzilla/codegen/pkg/config"
)

// Prepare returns the spec the generator reads: refs to other files bundled in, overlays applied,
// then filtered, simplified and pruned as cfg says. A spec no step changes comes back byte for byte.
func Prepare(ctx context.Context, cfg *config.Config, opts ...Option) ([]byte, []Diagnostic, error) {
	var o options
	for _, opt := range opts {
		opt(&o)
	}

	out, err := prepare.Run(ctx, libopenapi.New(), prepare.Input{Spec: o.spec, Config: cfg})
	if err != nil {
		return nil, nil, err
	}
	return out.Bytes, diagnostics(out.Diagnostics), nil
}
