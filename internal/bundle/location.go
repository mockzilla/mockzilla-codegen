// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

// Where the files of a split spec live: a $ref resolved against the file that holds it, and the
// name suffixes a file or folder gives a component copied in from it.

package bundle

import (
	"net/url"
	"path"
	"path/filepath"
	"strings"
	"unicode"
)

// IsURL reports whether a spec location is fetched over HTTP rather than read from disk.
func IsURL(loc string) bool {
	return strings.HasPrefix(loc, "http://") || strings.HasPrefix(loc, "https://")
}

// join resolves ref against base: a folder for files, the referring document itself for URLs.
func join(base, ref string) (string, error) {
	if IsURL(ref) {
		return ref, nil
	}
	if IsURL(base) {
		b, err := url.Parse(base)
		if err != nil {
			return "", err
		}
		r, err := url.Parse(ref)
		if err != nil {
			return "", err
		}
		return b.ResolveReference(r).String(), nil
	}

	if decoded, err := url.PathUnescape(ref); err == nil {
		ref = decoded
	}
	ref = filepath.FromSlash(ref)
	if filepath.IsAbs(ref) {
		return filepath.Clean(ref), nil
	}
	return filepath.Join(base, ref), nil
}

// baseOf is what refs inside the document at loc resolve against.
func baseOf(loc string) string {
	if IsURL(loc) {
		return loc
	}
	return filepath.Dir(loc)
}

// stem is the file name without its extension; URLs use their last path segment.
func stem(loc string) string {
	name := filepath.Base(loc)
	if IsURL(loc) {
		if u, err := url.Parse(loc); err == nil {
			name = path.Base(u.Path)
		}
	}
	return strings.TrimSuffix(name, path.Ext(name))
}

// folders lists the folders of loc from the nearest outwards. For files it stops at base, the
// root spec's folder, so a name never depends on where the spec sits on disk.
func folders(loc, base string) []string {
	var dir string
	if IsURL(loc) {
		u, err := url.Parse(loc)
		if err != nil {
			return nil
		}
		dir = path.Dir(u.Path)
	} else {
		rel, err := filepath.Rel(base, filepath.Dir(loc))
		if err != nil {
			return nil
		}
		dir = rel
	}

	var out []string
	parts := strings.FieldsFunc(filepath.ToSlash(dir), func(r rune) bool { return r == '/' })
	for i := len(parts) - 1; i >= 0; i-- {
		if parts[i] != "." && parts[i] != ".." {
			out = append(out, parts[i])
		}
	}
	return out
}

// title turns a file or folder name into a name suffix: common -> Common, pet-types -> PetTypes.
func title(s string) string {
	var b strings.Builder
	isWordStart := true
	for _, r := range s {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) {
			isWordStart = true
			continue
		}
		if isWordStart {
			r = unicode.ToUpper(r)
		}
		b.WriteRune(r)
		isWordStart = false
	}
	return b.String()
}
