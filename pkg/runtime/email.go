// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package runtime

import "net/mail"

// Email holds an email address. Decoding accepts any string; Validate checks it.
type Email string

// Validate accepts a bare address only: "a@example.com", not "A <a@example.com>".
func (e Email) Validate() error {
	a, err := mail.ParseAddress(string(e))
	if err != nil || a.Name != "" || a.Address != string(e) {
		return ErrInvalidEmail
	}
	return nil
}
