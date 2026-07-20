// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

// Package prepare turns the input spec into the one the generator reads: files bundled, overlays
// applied, then filtered, simplified and pruned.
package prepare

import (
	"context"
	"fmt"
	"strings"

	"github.com/mockzilla/codegen/internal/bundle"
	"github.com/mockzilla/codegen/internal/diag"
	"github.com/mockzilla/codegen/internal/oasdoc"
	"github.com/mockzilla/codegen/internal/provider"
	"github.com/mockzilla/codegen/internal/transform"
	"github.com/mockzilla/codegen/pkg/config"
)

// Input is the spec to prepare. Spec, when set, is used instead of reading the file; Path, when
// set, wins over the config's spec.path and still names the spec for refs and positions.
type Input struct {
	Spec   []byte
	Path   string
	Config *config.Config
}

// Output is the prepared spec. Positions, taken after bundling, map pointers to source files.
type Output struct {
	Bytes       []byte
	Positions   map[string]diag.Origin
	Diagnostics []diag.Diagnostic
}

type job struct {
	p         provider.Provider
	cfg       *config.Config
	loc       string
	doc       *oasdoc.Doc
	diags     diag.Collector
	isChanged bool
}

// Run prepares the spec. Bytes are the input untouched when no step changed anything.
func Run(ctx context.Context, p provider.Provider, in Input) (*Output, error) {
	j := &job{p: p, cfg: in.Config, loc: in.Path}
	if j.loc == "" && j.cfg.Spec.Path != "" {
		j.loc = j.cfg.Resolve(j.cfg.Spec.Path)
	}

	data, err := j.read(ctx, in.Spec)
	if err != nil {
		return nil, err
	}

	if err := j.bundle(ctx); err != nil {
		return nil, err
	}
	positions := j.doc.Positions()

	if err := j.overlays(ctx); err != nil {
		return nil, err
	}

	j.transform()
	return j.result(data, positions)
}

func (j *job) read(ctx context.Context, data []byte) ([]byte, error) {
	if data == nil {
		if j.loc == "" {
			return nil, ErrNoSpec
		}
		var err error
		if data, err = read(ctx, j.loc); err != nil {
			return nil, err
		}
	}
	return data, j.parse(data)
}

// parse replaces the document, keeping the layout of the one before, so the output keeps the
// source's indentation after an overlay.
func (j *job) parse(data []byte) error {
	doc, err := oasdoc.Parse(data, j.loc)
	if err != nil {
		return err
	}
	if _, err := doc.Version(); err != nil {
		return fmt.Errorf("%s: %w", j.loc, err)
	}
	if j.doc != nil {
		doc.CopyLayout(j.doc)
	}
	j.doc = doc
	return nil
}

// bundle resolves refs of an in-memory spec with no path against the config's folder.
func (j *job) bundle(ctx context.Context) error {
	in := bundle.Input{Doc: j.doc, Location: j.loc, Load: read}
	if j.loc == "" {
		in.BaseDir = j.cfg.Resolve(".")
	}

	isChanged, diags, err := bundle.Run(ctx, in)
	if err != nil {
		return err
	}
	j.diags.Append(diags...)
	j.isChanged = j.isChanged || isChanged
	return nil
}

func (j *job) overlays(ctx context.Context) error {
	if len(j.cfg.Spec.Overlays) == 0 {
		return nil
	}

	data, err := j.doc.Marshal()
	if err != nil {
		return err
	}
	for _, o := range j.cfg.Spec.Overlays {
		overlay, err := read(ctx, j.cfg.Resolve(o))
		if err != nil {
			return err
		}
		out, diags, err := j.p.ApplyOverlay(ctx, data, overlay)
		if err != nil {
			return fmt.Errorf("%s: %w", o, err)
		}
		j.diags.Append(diags...)
		data = out
	}

	j.isChanged = true
	return j.parse(data)
}

// transform filters, simplifies and prunes. A spec with no operations is models only, so it is
// not pruned: pruning would empty it.
func (j *job) transform() {
	hasOperations := transform.HasOperations(j.doc)
	isChanged, diags := transform.Filter(j.doc, j.cfg.Spec.Filter)
	j.diags.Append(diags...)
	j.isChanged = j.isChanged || isChanged

	if s := j.cfg.Spec.Simplify; s != nil {
		j.isChanged = transform.Simplify(j.doc, *s) || j.isChanged
	}

	if prune := j.cfg.Spec.Prune; prune != nil && !*prune {
		return
	}
	if !hasOperations {
		j.diags.Append(diag.Diagnostic{
			Severity: diag.Info,
			Code:     diag.CodePruneSkipped,
			Message:  "the spec has no operations, so nothing is pruned",
		})
		return
	}
	isChanged, diags = transform.Prune(j.doc)
	j.diags.Append(diags...)
	j.isChanged = j.isChanged || isChanged
}

// result is data when nothing changed, else the document written out.
func (j *job) result(data []byte, positions map[string]diag.Origin) (*Output, error) {
	out := &Output{Bytes: data, Positions: positions, Diagnostics: withOrigins(j.diags.List(), positions)}
	if !j.isChanged {
		return out, nil
	}

	var err error
	if out.Bytes, err = j.doc.Marshal(); err != nil {
		return nil, err
	}
	return out, nil
}

// withOrigins gives each diagnostic without a position the one of its pointer or nearest ancestor.
func withOrigins(diags []diag.Diagnostic, positions map[string]diag.Origin) []diag.Diagnostic {
	var c diag.Collector
	for _, d := range diags {
		for p := d.Pointer; d.Origin == (diag.Origin{}); {
			if at, ok := positions[p]; ok {
				d.Origin = at
				break
			}
			i := strings.LastIndex(p, "/")
			if i < 0 {
				break
			}
			p = p[:i]
		}
		c.Append(d)
	}
	return c.List()
}
