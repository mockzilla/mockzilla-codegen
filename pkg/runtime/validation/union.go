// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

// The checks of oneOf and anyOf unions: how many variants are set, and whether one passes.

package validation

import "strconv"

// VariantErrors are the failed checks of one union member, and whether its variant is set.
type VariantErrors struct {
	IsSet bool
	Errs  Errors
}

// ExactlyOne is the check of a oneOf union: one variant set.
func ExactlyOne(set ...bool) error {
	if n := count(set); n != 1 {
		return Error{Message: "exactly one variant must be set, found " + strconv.Itoa(n), Rule: RuleOneOf}
	}
	return nil
}

// AtMostOne is the check of a nullable oneOf union, where nothing set is null.
func AtMostOne(set ...bool) error {
	return atMostOne(count(set))
}

// AtLeastOne is the check of an anyOf union.
func AtLeastOne(set ...bool) error {
	if count(set) == 0 {
		return Error{Message: "at least one variant must be set", Rule: RuleAnyOf}
	}
	return nil
}

// ExactlyOneOf is the check of a oneOf of required lists: the value has exactly one of them.
func ExactlyOneOf(members string, has ...bool) error {
	if n := count(has); n != 1 {
		return Error{Message: "exactly one of " + members + " must be set, found " + strconv.Itoa(n), Rule: RuleOneOf}
	}
	return nil
}

// AtLeastOneOf is the check of an anyOf of required lists: the value has one or more of them.
func AtLeastOneOf(members string, has ...bool) error {
	if count(has) == 0 {
		return Error{Message: "at least one of " + members + " must be set", Rule: RuleAnyOf}
	}
	return nil
}

// AnyValid is the check of an anyOf union: nil when a set variant passes, else all their errors.
func AnyValid(variants ...VariantErrors) error {
	var errs Errors
	for _, v := range variants {
		switch {
		case !v.IsSet:
		case len(v.Errs) == 0:
			return nil
		default:
			errs = append(errs, v.Errs...)
		}
	}
	return errs.Err()
}

// OneValid is the check of oneOf members that share a variant: nil when exactly one passes.
func OneValid(members ...VariantErrors) error {
	var errs Errors
	n := 0
	for _, m := range members {
		switch {
		case !m.IsSet:
		case len(m.Errs) == 0:
			n++
		default:
			errs = append(errs, m.Errs...)
		}
	}

	switch {
	case n == 1:
		return nil
	case n > 1:
		return Error{Message: "exactly one variant must match, found " + strconv.Itoa(n), Rule: RuleOneOf}
	}
	return errs.Err()
}

func atMostOne(n int) error {
	if n > 1 {
		return Error{Message: "at most one variant may be set, found " + strconv.Itoa(n), Rule: RuleOneOf}
	}
	return nil
}

func count(set []bool) int {
	n := 0
	for _, isSet := range set {
		if isSet {
			n++
		}
	}
	return n
}
