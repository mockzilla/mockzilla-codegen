// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package config

import "go.yaml.in/yaml/v4"

// Template is the override of one template block: Text, or File, the file that holds the text.
// In YAML it is the text itself, or a mapping with the key file.
type Template struct {
	Text string `yaml:"-"`
	File string `yaml:"file" desc:"File that holds the template text, relative to the config."`
}

// UnmarshalYAML reads a scalar as the text and a mapping as the file.
func (t *Template) UnmarshalYAML(n *yaml.Node) error {
	if n.Kind == yaml.ScalarNode {
		return n.Load(&t.Text)
	}

	// A type without the method, so that the mapping is read field by field.
	type template Template
	return n.Load((*template)(t), yaml.WithKnownFields())
}
