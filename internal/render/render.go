// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

// Package render runs the templates of every generator and puts their output together into files.
package render

import (
	"bytes"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"slices"
	"strings"
	"text/template"
	"unicode"

	"github.com/mockzilla/mockzilla-codegen/internal/gocode"
	"github.com/mockzilla/mockzilla-codegen/internal/layout"
	"github.com/mockzilla/mockzilla-codegen/pkg/config"
)

const (
	fileTemplate = "render/file.tmpl"

	// missingKey makes a key a map does not have an error, where a template would write noValue.
	missingKey = "missingkey=error"

	// noValue is what a template writes for a value that is not set, such as a key the config
	// gives no value.
	noValue = "<no value>"
)

//go:embed *.tmpl
var templates embed.FS

// Set is the templates of one generator: the .tmpl files at the root of FS, named Name/<file>
// once loaded. Parts maps each part to the file that renders it; Blocks lists the blocks a
// config may override, which a template writes with the override func.
type Set struct {
	Name   string
	FS     fs.FS
	Parts  map[layout.PartID]string
	Blocks []string
}

// Source is a template given as text, such as a plugin's, with the funcs it may call on top of
// the engine's. Funcs has passed CheckFuncs; one named like a func of the engine replaces it.
// Name names the template in errors.
type Source struct {
	Name  string
	Text  string
	Funcs template.FuncMap
}

// Options are what the engine reads of a config. Templates and Format mirror its keys. Needs
// says, for each block of a part the config does not write, what the config has to set.
type Options struct {
	Templates map[string]string
	Needs     map[string]string
	Format    bool
}

// FileData is what a generated file holds around its parts. Header is plain text; Imports is the
// import declaration; Guard is the runtime constant the file refers to, if any. Parts holds the
// declarations of each part, with or without line breaks around them.
type FileData struct {
	Header  string
	Package string
	Imports string
	Guard   string
	Parts   []string
}

// Engine holds every template of a run. An override is a template apart from the others: one
// that could run the template that asks for it would never end.
type Engine struct {
	tmpl      *template.Template
	parts     map[layout.PartID]string
	overrides map[string]*template.Template
	format    bool
}

// New loads the templates of every set, then the block overrides. An override of a block no set
// declares, or one that does not parse, is a config error.
func New(sets []Set, opts Options) (*Engine, error) {
	e := &Engine{parts: make(map[layout.PartID]string), overrides: make(map[string]*template.Template), format: opts.Format}
	e.tmpl = template.New("").Option(missingKey).Funcs(funcs()).Funcs(template.FuncMap{"override": e.overrideLines})
	var blocks []string
	for _, set := range slices.Concat([]Set{{Name: "render", FS: templates}}, sets) {
		if err := e.load(set); err != nil {
			return nil, err
		}
		blocks = append(blocks, set.Blocks...)
	}

	if err := e.override(opts, blocks); err != nil {
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

// RenderFile puts a file together and formats it when the options ask for that. Each part with
// text goes on lines of its own below a blank line, so that none runs into the one before it.
func (e *Engine) RenderFile(d FileData) ([]byte, error) {
	parts := make([]string, 0, len(d.Parts))
	for _, part := range d.Parts {
		if text := trimBlank(part); text != "" {
			parts = append(parts, text)
		}
	}
	d.Parts = parts

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

func (e *Engine) override(opts Options, blocks []string) error {
	var issues []config.Issue
	for _, name := range slices.Sorted(maps.Keys(opts.Templates)) {
		if problem := e.define(name, opts, blocks); problem != "" {
			issues = append(issues, config.Issue{Key: "templates." + name, Message: problem})
		}
	}

	if len(issues) > 0 {
		return &config.ValidationError{Issues: issues}
	}
	return nil
}

// define makes the text opts has for block name its override. It returns what is wrong with the
// override, if anything.
func (e *Engine) define(name string, opts Options, blocks []string) string {
	switch key, isLeftOut := opts.Needs[name]; {
	case isLeftOut:
		return "needs " + key
	case !slices.Contains(blocks, name):
		return unknownBlock(slices.Concat(blocks, slices.Collect(maps.Keys(opts.Needs))))
	}

	t, err := template.New(name).Option(missingKey).Funcs(funcs()).Parse(opts.Templates[name])
	if err != nil {
		return err.Error()
	}
	for _, other := range t.Templates() {
		if other.Name() != name {
			return fmt.Sprintf("defines %q; an override replaces its own block only", other.Name())
		}
	}
	e.overrides[name] = t
	return ""
}

func (e *Engine) execute(name string, data any) ([]byte, error) {
	var b bytes.Buffer
	if err := e.tmpl.ExecuteTemplate(&b, name, data); err != nil {
		// An override that fails says where in its own text, not where a template asked for it.
		var failed *overrideError
		if errors.As(err, &failed) {
			err = failed.err
		}
		return nil, fmt.Errorf("%w: %w", ErrExecute, err)
	}
	return b.Bytes(), nil
}

// overrideLines is the override func of the templates. It runs the override of block on data and
// returns its text on lines of its own: a line break, then the text without the blank lines
// around it. A block the config leaves alone, and an override that writes nothing, give nothing.
// Text that holds noValue is an error.
func (e *Engine) overrideLines(block string, data any) (string, error) {
	t, ok := e.overrides[block]
	if !ok {
		return "", nil
	}

	var b bytes.Buffer
	if err := t.Execute(&b, data); err != nil {
		return "", &overrideError{err: err}
	}
	if strings.Contains(b.String(), noValue) {
		return "", &overrideError{err: fmt.Errorf("%s: %w", block, ErrNoValue)}
	}

	text := trimBlank(b.String())
	if text == "" {
		return "", nil
	}
	return "\n" + text, nil
}

// RenderSource parses src on its own, apart from the sets, and runs it on data.
func RenderSource(src Source, data any) ([]byte, error) {
	t, err := template.New(src.Name).Option(missingKey).Funcs(funcs()).Funcs(src.Funcs).Parse(src.Text)
	if err != nil {
		return nil, fmt.Errorf("%w: %s: %w", ErrTemplate, src.Name, err)
	}

	var b bytes.Buffer
	if err = t.Execute(&b, data); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrExecute, err)
	}
	return b.Bytes(), nil
}

func unknownBlock(blocks []string) string {
	if len(blocks) == 0 {
		return "unknown block; there is no block to override"
	}
	return "unknown block; the blocks are " + strings.Join(slices.Sorted(slices.Values(blocks)), ", ")
}

// trimBlank returns text without the blank lines around it. Its first line keeps its indent,
// which shows in output that is not formatted.
func trimBlank(text string) string {
	text = strings.TrimRightFunc(text, unicode.IsSpace)
	blank := len(text) - len(strings.TrimLeftFunc(text, unicode.IsSpace))
	return text[strings.LastIndexByte(text[:blank], '\n')+1:]
}
