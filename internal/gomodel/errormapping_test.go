// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package gomodel

import "testing"

func TestErrorsInModel(t *testing.T) {
	t.Parallel()

	opts := testOptions()
	opts.ErrorMapping = map[string]string{
		"ErrorResponse":          "error.message",
		"ErrorResponseError":     "code",
		"DetailError":            "details[].text",
		"ListedError":            "problems[].text",
		"Problem":                "detail",
		"Plain":                  "error",
		"DetailErrorDetailsItem": "text[]",
		"ProblemOption2":         "nope",
		"Status":                 "x",
		"Missing":                "x",
		"StrictError":            "error.message",
		"StrictList":             "items[].text",
	}

	checkGolden(t, "errors", "errors", opts)
}
