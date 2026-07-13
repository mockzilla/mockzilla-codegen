// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package config

import (
	"fmt"
	"time"
)

// Duration is a time.Duration written as text, such as 30s or 1m30s.
type Duration time.Duration

// UnmarshalText parses a duration such as 30s or 1m30s. Negative durations are rejected.
func (d *Duration) UnmarshalText(text []byte) error {
	v, err := time.ParseDuration(string(text))
	if err != nil || v < 0 {
		return fmt.Errorf("%w %q, want a value such as 30s or 1m30s", ErrDuration, text)
	}

	*d = Duration(v)
	return nil
}
