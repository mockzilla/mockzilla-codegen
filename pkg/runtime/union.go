// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package runtime

import (
	"bytes"
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"math/bits"
	"slices"
	"strings"
)

// Variant describes one member of a union to UnmarshalUnion: the JSON kinds it takes, the
// discriminator values that pick it, and for objects its property names (Known nil takes any key).
type Variant struct {
	Name      string
	Kind      Kind
	Values    []string
	IsDefault bool
	Required  []string
	Known     []string
	IsClosed  bool
	Into      func(data []byte) error
}

// Union describes a union to UnmarshalUnion. Shared are the property names the union holds next
// to its variants, which no variant counts as unknown.
type Union struct {
	IsAnyOf       bool
	Discriminator string
	Shared        []string
	Variants      []Variant
}

// candidate is a variant that can take a value; for objects, score counts the required properties
// present less the unknown keys.
type candidate struct {
	index     int
	score     int
	isMatch   bool
	isPerfect bool
}

// Into returns a decoder that sets *dst only when data decodes.
func Into[T any](dst *T) func(data []byte) error {
	return func(data []byte) error {
		var v T
		if err := json.Unmarshal(data, &v); err != nil {
			return err
		}
		*dst = v
		return nil
	}
}

// UnmarshalUnion decodes data into the variants of u it matches: one for oneOf, every match for
// anyOf. docs/types.md in the mockzilla-codegen repository has the order variants are tried in.
func UnmarshalUnion(data []byte, u Union) error {
	kind := JSONKind(data)
	switch kind {
	case 0:
		return fmt.Errorf("%w: %q is no JSON value", ErrNoVariant, data)
	case KindNull:
		return nil
	default:
	}

	var obj map[string]json.RawMessage
	if kind == KindObject {
		if err := json.Unmarshal(data, &obj); err != nil {
			return err
		}
	}

	pool := make([]int, len(u.Variants))
	for i := range pool {
		pool[i] = i
	}
	if u.Discriminator != "" && obj != nil {
		picked, rest, err := u.discriminate(obj)
		switch {
		case err != nil:
			return err
		case picked >= 0:
			return u.Variants[picked].Into(data)
		}
		pool = rest
	}

	cands := u.candidates(pool, kind, obj)
	if u.IsAnyOf {
		return u.decodeAll(data, kind, cands)
	}
	return u.decodeOne(data, kind, cands)
}

// UnmarshalUnionText decodes raw with a union's UnmarshalJSON, as a number or boolean first.
func UnmarshalUnionText(raw []byte, decode func(data []byte) error) error {
	quoted, _ := json.Marshal(string(raw))
	if !isLiteral(raw) {
		return decode(quoted)
	}

	err := decode(raw)
	if err == nil || decode(quoted) == nil {
		return nil
	}
	return err
}

// discriminate returns the variant the discriminator value picks, else -1 and the variants left to
// match by shape: all of them without the property, those without values for an unknown value.
func (u Union) discriminate(obj map[string]json.RawMessage) (int, []int, error) {
	raw, hasValue := obj[u.Discriminator]
	value := discriminatorValue(raw)

	picked := -1
	var all, open []int
	var allowed []string
	for i, v := range u.Variants {
		if hasValue && slices.Contains(v.Values, value) {
			return i, nil, nil
		}
		if v.IsDefault {
			picked = i
		}
		if len(v.Values) == 0 {
			open = append(open, i)
		}
		all = append(all, i)
		allowed = append(allowed, v.Values...)
	}

	switch {
	case picked >= 0:
		return picked, nil, nil
	case !hasValue:
		return -1, all, nil
	case len(open) > 0:
		return -1, open, nil
	}
	return -1, nil, fmt.Errorf("%w %q, want one of %s", ErrUnknownDiscriminator, value, strings.Join(allowed, ", "))
}

func (u Union) candidates(pool []int, kind Kind, obj map[string]json.RawMessage) []candidate {
	var out []candidate
	for _, i := range pool {
		v := u.Variants[i]
		if v.Kind&kind == 0 {
			continue
		}

		c := candidate{index: i, isMatch: true}
		if kind == KindObject {
			missing := 0
			for _, name := range v.Required {
				if _, found := obj[name]; !found {
					missing++
				}
			}
			unknown := 0
			for key := range obj {
				if v.Known != nil && !slices.Contains(v.Known, key) && !slices.Contains(u.Shared, key) {
					unknown++
				}
			}
			if v.IsClosed && unknown > 0 {
				continue
			}
			c.score = len(v.Required) - missing - unknown
			c.isMatch = missing == 0
			c.isPerfect = missing == 0 && unknown == 0
		}
		out = append(out, c)
	}

	slices.SortStableFunc(out, func(a, b candidate) int {
		return cmp.Or(cmp.Compare(b.score, a.score), cmp.Compare(u.width(a), u.width(b)))
	})
	return out
}

// width is the number of kinds a candidate takes: an int goes before a float64 for an integer.
func (u Union) width(c candidate) int {
	return bits.OnesCount8(uint8(u.Variants[c.index].Kind))
}

// decodeOne sets the first candidate that decodes. Two perfect object matches with the same score
// are ambiguous.
func (u Union) decodeOne(data []byte, kind Kind, cands []candidate) error {
	if len(cands) > 1 && cands[0].isPerfect && cands[1].isPerfect && cands[0].score == cands[1].score {
		return fmt.Errorf("%w: %s and %s", ErrAmbiguous, u.Variants[cands[0].index].Name, u.Variants[cands[1].index].Name)
	}
	return u.decodeFirst(data, kind, cands)
}

// decodeAll sets every matching candidate that decodes, and falls back to the first that decodes
// when none does.
func (u Union) decodeAll(data []byte, kind Kind, cands []candidate) error {
	isSet := false
	for _, c := range cands {
		if c.isMatch && u.Variants[c.index].Into(data) == nil {
			isSet = true
		}
	}
	if isSet {
		return nil
	}
	return u.decodeFirst(data, kind, cands)
}

func (u Union) decodeFirst(data []byte, kind Kind, cands []candidate) error {
	var errs []error
	for _, c := range cands {
		v := u.Variants[c.index]
		err := v.Into(data)
		if err == nil {
			return nil
		}
		errs = append(errs, fmt.Errorf("%s: %w", v.Name, err))
	}
	if len(errs) == 0 {
		return fmt.Errorf("%w for a JSON %s", ErrNoVariant, kind)
	}
	return fmt.Errorf("%w for a JSON %s: %w", ErrNoVariant, kind, errors.Join(errs...))
}

func discriminatorValue(raw json.RawMessage) string {
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	return string(bytes.TrimSpace(raw))
}

// isLiteral reports raw written as a JSON number or boolean, with no space around it.
func isLiteral(raw []byte) bool {
	switch JSONKind(raw) {
	case KindBool, KindInteger, KindNumber:
		return json.Valid(raw) && len(bytes.TrimSpace(raw)) == len(raw)
	default:
	}
	return false
}
