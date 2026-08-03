// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

// Package render runs the templates of every generator and puts their output together into files.
package render

import (
	"bytes"
	"embed"
	"fmt"
	"io/fs"
	"maps"
	"slices"
	"strings"
	"text/template"

	"github.com/mockzilla/codegen/internal/gocode"
	"github.com/mockzilla/codegen/internal/layout"
	"github.com/mockzilla/codegen/pkg/config"
)

const fileTemplate = "render/file.tmpl"

//go:embed *.tmpl
var templates embed.FS

// Set is the templates of one generator: the .tmpl files at the root of FS, named Name/<file>
// once loaded. Parts maps each part to the file that renders it; Blocks lists the blocks a
// config may override.
type Set struct {
	Name   string
	FS     fs.FS
	Parts  map[layout.PartID]string
	Blocks []string
}

// Options mirror the config keys the engine reads.
type Options struct {
	Templates map[string]string
	Format    bool
}

// FileData is what a generated file holds around its parts. Header is plain text; Imports is the
// import declaration; Guard is the runtime constant the file refers to, if any. Each part starts
// every declaration with a blank line.
type FileData struct {
	Header  string
	Package string
	Imports string
	Guard   string
	Parts   []string
}

// Engine holds every template of a run.
type Engine struct {
	tmpl   *template.Template
	parts  map[layout.PartID]string
	format bool
}

// New loads the templates of every set, then the block overrides. An override of a block no set
// declares, or one that does not parse, is a config error.
func New(sets []Set, opts Options) (*Engine, error) {
	e := &Engine{tmpl: template.New("").Funcs(funcs()), parts: make(map[layout.PartID]string), format: opts.Format}
	var blocks []string
	for _, set := range slices.Concat([]Set{{Name: "render", FS: templates}}, sets) {
		if err := e.load(set); err != nil {
			return nil, err
		}
		blocks = append(blocks, set.Blocks...)
	}

	if err := e.override(opts.Templates, blocks); err != nil {
		return nil, err
	}
	return e, nil
}

// RenderPart runs the template of part on data.
func (e *Engine) RenderPart(part layout.PartID, data any) ([]byte, error) {
	name, ok := e.parts[part]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrUnknownPart, part)
	}
	return e.execute(name, data)
}

// RenderFile puts a file together and formats it when the options ask for that.
func (e *Engine) RenderFile(d FileData) ([]byte, error) {
	out, err := e.execute(fileTemplate, d)
	if err != nil || !e.format {
		return out, err
	}
	return gocode.Format(out)
}

func (e *Engine) load(set Set) error {
	entries, err := fs.ReadDir(set.FS, ".")
	if err != nil {
		return fmt.Errorf("%w: %s: %w", ErrTemplate, set.Name, err)
	}

	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".tmpl") {
			continue
		}
		name := set.Name + "/" + entry.Name()
		var text []byte
		if text, err = fs.ReadFile(set.FS, entry.Name()); err == nil {
			_, err = e.tmpl.New(name).Parse(string(text))
		}
		if err != nil {
			return fmt.Errorf("%w: %s: %w", ErrTemplate, name, err)
		}
	}

	for part, file := range set.Parts {
		e.parts[part] = set.Name + "/" + file
	}
	return nil
}

func (e *Engine) override(texts map[string]string, blocks []string) error {
	var issues []config.Issue
	for _, name := range slices.Sorted(maps.Keys(texts)) {
		if problem := e.define(name, texts[name], blocks); problem != "" {
			issues = append(issues, config.Issue{Key: "templates." + name, Message: problem})
		}
	}

	if len(issues) > 0 {
		return &config.ValidationError{Issues: issues}
	}
	return nil
}

// define replaces block name with text. It returns what is wrong with the override, if anything.
func (e *Engine) define(name, text string, blocks []string) string {
	if !slices.Contains(blocks, name) {
		return unknownBlock(blocks)
	}

	t, err := template.New(name).Funcs(funcs()).Parse(text)
	if err != nil {
		return err.Error()
	}
	for _, other := range t.Templates() {
		if other.Name() != name {
			return fmt.Sprintf("defines %q; an override replaces its own block only", other.Name())
		}
	}
	// text/template's AddParseTree never returns an error.
	_, _ = e.tmpl.AddParseTree(name, t.Tree)
	return ""
}

func (e *Engine) execute(name string, data any) ([]byte, error) {
	var b bytes.Buffer
	if err := e.tmpl.ExecuteTemplate(&b, name, data); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrExecute, err)
	}
	return b.Bytes(), nil
}

func unknownBlock(blocks []string) string {
	if len(blocks) == 0 {
		return "unknown block; no block can be overridden yet"
	}
	return "unknown block; the blocks are " + strings.Join(slices.Sorted(slices.Values(blocks)), ", ")
}
