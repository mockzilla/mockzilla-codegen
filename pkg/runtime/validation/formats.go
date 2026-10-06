// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

// The formats Format knows: uuid, uri, ipv4, ipv6, hostname, email, date and date-time.

package validation

import (
	"net/netip"
	"net/url"
	"strings"
	"time"
)

const (
	maxHostname = 253
	maxLabel    = 63
	hostChars   = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789-"
	maxEmail    = 254
	maxLocal    = 64
	atext       = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789!#$%&'*+-/=?^_`{|}~"
	ipv6Tag     = "IPv6:"
)

// isUUID reports a UUID written as 8-4-4-4-12 hex digits.
func isUUID(s string) bool {
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

// isURI reports an absolute URI: one with a scheme.
func isURI(s string) bool {
	u, err := url.Parse(s)
	return err == nil && u.Scheme != ""
}

// isURIReference reports a URI or a relative reference.
func isURIReference(s string) bool {
	_, err := url.Parse(s)
	return err == nil
}

// isIPv4 reports an IPv4 address in dotted decimal.
func isIPv4(s string) bool {
	a, err := netip.ParseAddr(s)
	return err == nil && a.Is4()
}

// isIPv6 reports an IPv6 address.
func isIPv6(s string) bool {
	a, err := netip.ParseAddr(s)
	return err == nil && a.Is6() && a.Zone() == ""
}

// isHostname reports a host name as RFC 1123 has it: dot-separated labels of letters, digits and
// inner hyphens.
func isHostname(s string) bool {
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

// isEmail reports an address as RFC 5321 writes it, ASCII only: a@example.com, "a b"@example.com.
func isEmail(s string) bool {
	at := strings.LastIndexByte(s, '@')
	if at < 1 || at > maxLocal || len(s) > maxEmail {
		return false
	}
	local, domain := s[:at], s[at+1:]
	return (isDotAtom(local) || isQuoted(local)) && (isHostname(domain) || isAddressLiteral(domain))
}

// isDate reports a calendar date: 2006-01-02.
func isDate(s string) bool {
	_, err := time.Parse(time.DateOnly, s)
	return err == nil
}

// isDateTime reports an RFC 3339 date and time.
func isDateTime(s string) bool {
	_, err := time.Parse(time.RFC3339, s)
	return err == nil
}

// isDotAtom reports words of atext joined by single dots: first.last+tag.
func isDotAtom(s string) bool {
	for word := range strings.SplitSeq(s, ".") {
		if word == "" || strings.Trim(word, atext) != "" {
			return false
		}
	}
	return true
}

// isQuoted reports printable ASCII in double quotes, where a backslash escapes the next character.
func isQuoted(s string) bool {
	inner, isOpened := strings.CutPrefix(s, `"`)
	inner, isClosed := strings.CutSuffix(inner, `"`)
	if !isOpened || !isClosed {
		return false
	}
	isEscaped := false
	for _, c := range []byte(inner) {
		switch {
		case c < ' ' || c > '~':
			return false
		case isEscaped:
			isEscaped = false
		case c == '\\':
			isEscaped = true
		case c == '"':
			return false
		}
	}
	return !isEscaped
}

// isAddressLiteral reports an IP address in brackets: [10.0.0.1] or [IPv6:::1].
func isAddressLiteral(s string) bool {
	ip, isOpened := strings.CutPrefix(s, "[")
	ip, isClosed := strings.CutSuffix(ip, "]")
	switch {
	case !isOpened || !isClosed:
		return false
	case len(ip) > len(ipv6Tag) && strings.EqualFold(ip[:len(ipv6Tag)], ipv6Tag):
		return isIPv6(ip[len(ipv6Tag):])
	}
	return isIPv4(ip)
}
