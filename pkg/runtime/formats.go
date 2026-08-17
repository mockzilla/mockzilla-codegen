// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package runtime

import (
	"net/mail"
	"net/netip"
	"net/url"
	"strings"
	"time"
)

const (
	maxHostname = 253
	maxLabel    = 63
	hostChars   = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789-"
)

// IsUUID reports a UUID written as 8-4-4-4-12 hex digits.
func IsUUID(s string) bool {
	if len(s) != 36 {
		return false
	}
	for i, c := range s {
		switch i {
		case 8, 13, 18, 23:
			if c != '-' {
				return false
			}
		default:
			if !strings.ContainsRune(hexDigits, c) {
				return false
			}
		}
	}
	return true
}

// IsURI reports an absolute URI: one with a scheme.
func IsURI(s string) bool {
	u, err := url.Parse(s)
	return err == nil && u.Scheme != ""
}

// IsURIReference reports a URI or a relative reference.
func IsURIReference(s string) bool {
	_, err := url.Parse(s)
	return err == nil
}

// IsIPv4 reports an IPv4 address in dotted decimal.
func IsIPv4(s string) bool {
	a, err := netip.ParseAddr(s)
	return err == nil && a.Is4()
}

// IsIPv6 reports an IPv6 address.
func IsIPv6(s string) bool {
	a, err := netip.ParseAddr(s)
	return err == nil && a.Is6() && a.Zone() == ""
}

// IsHostname reports a host name as RFC 1123 has it: dot-separated labels of letters, digits and
// inner hyphens.
func IsHostname(s string) bool {
	if s == "" || len(s) > maxHostname {
		return false
	}
	for label := range strings.SplitSeq(s, ".") {
		isBadEnd := label == "" || label[0] == '-' || label[len(label)-1] == '-'
		if isBadEnd || len(label) > maxLabel || strings.Trim(label, hostChars) != "" {
			return false
		}
	}
	return true
}

// IsEmail reports a bare address: "a@example.com", not "A <a@example.com>".
func IsEmail(s string) bool {
	a, err := mail.ParseAddress(s)
	return err == nil && a.Name == "" && a.Address == s
}

// IsDate reports a calendar date: 2006-01-02.
func IsDate(s string) bool {
	_, err := time.Parse(time.DateOnly, s)
	return err == nil
}

// IsDateTime reports an RFC 3339 date and time.
func IsDateTime(s string) bool {
	_, err := time.Parse(time.RFC3339, s)
	return err == nil
}
