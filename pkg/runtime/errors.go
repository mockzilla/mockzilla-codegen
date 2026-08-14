// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package runtime

import "errors"

var (
	ErrInvalidDate        = errors.New("invalid date")
	ErrInvalidEmail       = errors.New("invalid email address")
	ErrAdditionalProperty = errors.New("invalid additional property")

	ErrNoVariant            = errors.New("no union variant matches")
	ErrAmbiguous            = errors.New("more than one union variant matches")
	ErrUnknownDiscriminator = errors.New("unknown discriminator value")
	ErrNotObject            = errors.New("not a JSON object")
)
