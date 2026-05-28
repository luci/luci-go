// Copyright 2026 The LUCI Authors.
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

package standard

import (
	"os"

	"github.com/BurntSushi/toml"

	"go.chromium.org/luci/common/errors"
)

// ProjectSpec represents the standard environment metadata extracted from a vpython.toml file.
type ProjectSpec struct {
	Name           string
	RequiresPython string
	Dependencies   []string
}

// ParseVPythonTOML loads and parses a standard vpython.toml spec file.
// It returns an error if the file has a missing or invalid [project] table.
func ParseVPythonTOML(path string) (*ProjectSpec, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, errors.Fmt("failed to read vpython.toml at: %s: %w", path, err)
	}
	return parseVPythonTOMLContent(content)
}

// parseVPythonTOMLContent parses the byte slice content of a vpython.toml spec file.
func parseVPythonTOMLContent(content []byte) (*ProjectSpec, error) {
	// Decode into standard structured schema.
	var schema struct {
		Project *struct {
			Name           string   `toml:"name"`
			RequiresPython string   `toml:"requires-python"`
			Dependencies   []string `toml:"dependencies"`
		} `toml:"project"`
	}
	if err := toml.Unmarshal(content, &schema); err != nil {
		return nil, errors.Fmt("failed to parse TOML structured schema: %w", err)
	}

	if schema.Project == nil {
		return nil, errors.New("missing [project] table")
	}

	spec := &ProjectSpec{
		Name:           schema.Project.Name,
		RequiresPython: schema.Project.RequiresPython,
		Dependencies:   schema.Project.Dependencies,
	}

	// Double check: if the [project] table was present but is completely empty/invalid
	if spec.Name == "" && spec.RequiresPython == "" && len(spec.Dependencies) == 0 {
		return nil, errors.New("empty [project] table")
	}

	return spec, nil
}
