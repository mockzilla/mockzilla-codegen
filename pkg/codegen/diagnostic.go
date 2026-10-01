// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package codegen

import "github.com/mockzilla/mockzilla-codegen/internal/diag"

// Diagnostic is a problem found in the spec, or in what the config asks of it. Pointer is a JSON
// pointer into the prepared spec; File, Line and Col say where it is in the source, Line and Col
// 0 when unknown. A problem that has no place in the spec has none of them.
type Diagnostic struct {
	Severity Severity
	Code     string
	Pointer  string
	File     string
	Line     int
	Col      int
	Message  string
}

func diagnostics(list []diag.Diagnostic) []Diagnostic {
	out := make([]Diagnostic, 0, len(list))
	for _, d := range list {
		out = append(out, Diagnostic{
			Severity: Severity(d.Severity),
			Code:     d.Code,
			Pointer:  d.Pointer,
			File:     d.Origin.File,
			Line:     d.Origin.Line,
			Col:      d.Origin.Col,
			Message:  d.Message,
		})
	}
	return out
}
