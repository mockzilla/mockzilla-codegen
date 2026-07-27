// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package runtime

import (
	"encoding/json"
	"fmt"
	"time"
)

// DateFormat is the layout of an OpenAPI date (RFC 3339 full-date).
const DateFormat = time.DateOnly

// Date is a calendar date, written as 2006-01-02 in JSON and text.
type Date struct {
	time.Time
}

// NewDate returns the date at midnight UTC.
func NewDate(year int, month time.Month, day int) Date {
	return Date{Time: time.Date(year, month, day, 0, 0, 0, 0, time.UTC)}
}

func (d Date) String() string {
	return d.Format(DateFormat)
}

func (d Date) MarshalText() ([]byte, error) {
	return []byte(d.String()), nil
}

func (d *Date) UnmarshalText(b []byte) error {
	t, err := time.Parse(DateFormat, string(b))
	if err != nil {
		return fmt.Errorf("%w: %q", ErrInvalidDate, b)
	}
	d.Time = t
	return nil
}

// MarshalJSON is needed because the embedded time.Time has its own.
func (d Date) MarshalJSON() ([]byte, error) {
	return json.Marshal(d.String())
}

// UnmarshalJSON leaves d unchanged on null, like encoding/json does for other types.
func (d *Date) UnmarshalJSON(b []byte) error {
	if string(b) == "null" {
		return nil
	}

	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return fmt.Errorf("%w: %s", ErrInvalidDate, b)
	}
	return d.UnmarshalText([]byte(s))
}
