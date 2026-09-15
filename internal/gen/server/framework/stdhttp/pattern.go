// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package stdhttp

import (
	"net/url"
	"strings"
)

// relation is how the requests two patterns match relate, as net/http tells them apart.
type relation int

const (
	// equivalent patterns match the same requests.
	equivalent relation = iota
	// moreGeneral patterns match every request the other does, and more.
	moreGeneral
	// moreSpecific patterns match some of the requests the other does, and no other.
	moreSpecific
	// overlaps patterns match some requests in common, and neither is more specific.
	overlaps
	// disjoint patterns match no request in common.
	disjoint
)

// pattern is a route as ServeMux takes it apart: the method and the segments of the path.
type pattern struct {
	method   string
	segments []segment
}

// segment is one segment of a pattern: a literal, / for {$}, or a wildcard by name, which
// takes the rest of the path when it is isMulti.
type segment struct {
	literal string
	isWild  bool
	isMulti bool
}

// parse takes a pattern RoutePattern wrote apart. Literals are unescaped, as ServeMux compares
// them.
func parse(route string) pattern {
	method, path, _ := strings.Cut(route, " ")
	p := pattern{method: method}
	for _, seg := range strings.Split(path[1:], "/") {
		switch {
		case seg == "{$}":
			p.segments = append(p.segments, segment{literal: "/"})
		case strings.HasPrefix(seg, "{"):
			name, isMulti := strings.CutSuffix(seg[1:len(seg)-1], "...")
			p.segments = append(p.segments, segment{literal: name, isWild: true, isMulti: isMulti})
		default:
			p.segments = append(p.segments, segment{literal: unescape(seg)})
		}
	}
	return p
}

// isMulti reports whether the pattern ends in a wildcard that takes the rest of the path.
func (p pattern) isMulti() bool {
	return p.segments[len(p.segments)-1].isMulti
}

// compare tells how the requests p and q match relate, by the rules of ServeMux.
func (p pattern) compare(q pattern) relation {
	rel := compareMethods(p.method, q.method)
	if rel == disjoint {
		return disjoint
	}
	return combine(rel, comparePaths(p, q))
}

// compareMethods relates two methods: GET matches HEAD requests too.
func compareMethods(m1, m2 string) relation {
	switch {
	case m1 == m2:
		return equivalent
	case m1 == "GET" && m2 == "HEAD":
		return moreGeneral
	case m1 == "HEAD" && m2 == "GET":
		return moreSpecific
	}
	return disjoint
}

// comparePaths relates the paths of two patterns segment by segment. A pattern that does not end
// in a multi wildcard matches paths of its own segment count alone; one that does takes the rest
// of a longer pattern.
func comparePaths(p, q pattern) relation {
	if len(p.segments) != len(q.segments) && !p.isMulti() && !q.isMulti() {
		return disjoint
	}

	rel := equivalent
	n := min(len(p.segments), len(q.segments))
	for i := range n {
		rel = combine(rel, compareSegments(p.segments[i], q.segments[i]))
		if rel == disjoint {
			return disjoint
		}
	}
	switch {
	case len(p.segments) == len(q.segments):
		return rel
	case len(p.segments) < len(q.segments) && p.isMulti():
		return combine(rel, moreGeneral)
	case len(q.segments) < len(p.segments) && q.isMulti():
		return combine(rel, moreSpecific)
	}
	return disjoint
}

// compareSegments relates two segments: a multi wildcard is more general than anything, a
// wildcard more general than a literal, and a wildcard never matches the trailing slash of {$}.
func compareSegments(s1, s2 segment) relation {
	switch {
	case s1.isMulti && s2.isMulti:
		return equivalent
	case s1.isMulti:
		return moreGeneral
	case s2.isMulti:
		return moreSpecific
	case s1.isWild && s2.isWild:
		return equivalent
	case s1.isWild && s2.literal == "/", s2.isWild && s1.literal == "/":
		return disjoint
	case s1.isWild:
		return moreGeneral
	case s2.isWild:
		return moreSpecific
	case s1.literal == s2.literal:
		return equivalent
	}
	return disjoint
}

// combine is the relation of two patterns from the relations of their parts.
func combine(r1, r2 relation) relation {
	switch {
	case r1 == equivalent:
		return r2
	case r1 == disjoint || r2 == disjoint:
		return disjoint
	case r1 == overlaps || r2 == equivalent:
		return r1
	case r1 == moreGeneral && r2 == moreSpecific, r1 == moreSpecific && r2 == moreGeneral:
		return overlaps
	}
	return r2
}

// unescape is a literal segment as ServeMux compares it: a segment that is not a valid escape
// stays as it is.
func unescape(seg string) string {
	if u, err := url.PathUnescape(seg); err == nil {
		return u
	}
	return seg
}
