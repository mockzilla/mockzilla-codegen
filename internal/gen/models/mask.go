// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package models

import (
	"strconv"

	"github.com/mockzilla/mockzilla-codegen/internal/gocode"
	"github.com/mockzilla/mockzilla-codegen/internal/gomodel"
)

// maskFuncs are the runtime functions that mask a string, by mask kind.
var maskFuncs = map[gomodel.MaskKind]string{
	gomodel.MaskFull:    "MaskFull",
	gomodel.MaskRegex:   "MaskRegex",
	gomodel.MaskHash:    "MaskHash",
	gomodel.MaskPartial: "MaskPartial",
}

// MaskView is what Masked and LogValue need. Slog is the name log/slog is imported under.
type MaskView struct {
	Receiver string
	Runtime  string
	Slog     string
	Fields   []MaskFieldView
}

// MaskFieldView sets Target to Value, Go expressions, when Guard holds or is empty.
type MaskFieldView struct {
	Target string
	Guard  string
	Value  string
}

func maskView(d *gomodel.Decl, s *gocode.Scope) *MaskView {
	rt := s.Import(gomodel.Import{Path: gomodel.RuntimePath})
	v := &MaskView{Receiver: receiver(d.Name), Runtime: rt, Slog: s.Import(gomodel.Import{Path: "log/slog"})}
	for _, m := range d.Masks {
		v.Fields = append(v.Fields, maskFieldView(m, v.Receiver, rt))
	}
	return v
}

func maskFieldView(m *gomodel.Mask, r, rt string) MaskFieldView {
	target := r
	if m.Field != "" {
		target = gocode.Selector(r, m.Field)
	}
	value := target
	if m.IsPointer {
		value = gocode.Deref(target)
	}

	var out string
	switch m.Kind {
	case gomodel.MaskZero:
		return MaskFieldView{Target: target, Value: gocode.Call(gocode.Selector(rt, "Zero"), target)}
	case gomodel.MaskItems:
		return MaskFieldView{Target: target, Value: gocode.Call(gocode.Selector(rt, "MaskSlice"), target)}
	case gomodel.MaskValues:
		return MaskFieldView{Target: target, Value: gocode.Call(gocode.Selector(rt, "MaskMap"), target)}
	case gomodel.MaskNested:
		out = gocode.Call(gocode.Selector(target, "Masked"))
	case gomodel.MaskRegex:
		out = gocode.Call(gocode.Selector(rt, maskFuncs[m.Kind]), value, m.Pattern.Name)
	case gomodel.MaskPartial:
		out = gocode.Call(gocode.Selector(rt, maskFuncs[m.Kind]), value, strconv.Itoa(m.KeepPrefix), strconv.Itoa(m.KeepSuffix))
	case gomodel.MaskFull, gomodel.MaskHash:
		out = gocode.Call(gocode.Selector(rt, maskFuncs[m.Kind]), value)
	}

	if !m.IsPointer {
		return MaskFieldView{Target: target, Value: out}
	}
	return MaskFieldView{Target: target, Guard: gocode.NotNil(target), Value: gocode.Call(gocode.Selector(rt, "Ptr"), out)}
}
