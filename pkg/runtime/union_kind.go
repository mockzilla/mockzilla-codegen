// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

// The JSON kind of a value, which tells union variants apart.

package runtime

import (
	"bytes"
	"strings"
)

// Kind is a set of JSON value kinds. A union variant lists the kinds it takes; a value has one.
type Kind uint8

const (
	KindNull Kind = 1 << iota
	KindBool
	// KindInteger is a number without a fraction or exponent.
	KindInteger
	KindNumber
	KindString
	KindArray
	KindObject

	KindAny = KindNull | KindBool | KindInteger | KindNumber | KindString | KindArray | KindObject
)

var kindNames = []string{"null", "boolean", "integer", "number", "string", "array", "object"}

func (k Kind) String() string {
	var names []string
	for i, name := range kindNames {
		if k&(1<<i) != 0 {
			names = append(names, name)
		}
	}
	return strings.Join(names, " or ")
}

// jsonKind returns the kind of the JSON value data holds, 0 when it holds none.
func jsonKind(data []byte) Kind {
	data = bytes.TrimSpace(data)
	if len(data) == 0 {
		return 0
	}

	switch c := data[0]; {
	case c == '{':
		return KindObject
	case c == '[':
		return KindArray
	case c == '"':
		return KindString
	case c == 't' || c == 'f':
		return KindBool
	case c == 'n':
		return KindNull
	case c == '-' || '0' <= c && c <= '9':
		if bytes.ContainsAny(data, ".eE") {
			return KindNumber
		}
		return KindInteger
	}
	return 0
}
