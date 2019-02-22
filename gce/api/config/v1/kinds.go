// Copyright 2018 The LUCI Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package config

import (
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/config/validation"
)

// Map returns a map of name to VM kind.
// For each name, only the last kind is included in the map.
// Use Validate to ensure names are unique.
func (kinds *Kinds) Map() map[string]*Kind {
	m := make(map[string]*Kind, len(kinds.GetKind()))
	for _, k := range kinds.GetKind() {
		m[k.Name] = k
	}
	return m
}

// Names returns the names of VM kinds.
func (kinds *Kinds) Names() []string {
	names := make([]string, len(kinds.GetKind()))
	for i, k := range kinds.GetKind() {
		names[i] = k.Name
	}
	return names
}

// Validate validates VM kinds.
func (kinds *Kinds) Validate(c *validation.Context) {
	names := stringset.New(len(kinds.GetKind()))
	for i, k := range kinds.GetKind() {
		c.Enter("kind %d", i)
		switch {
		case k.Name == "":
			c.Errorf("name is required")
		case !names.Add(k.Name):
			c.Errorf("duplicate name %q", k.Name)
		}
		c.Exit()
	}
}
