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

// ProjectSpec represents the standard environment metadata extracted from a pyproject.toml file.
type ProjectSpec struct {
	Name           string
	RequiresPython string
	Dependencies   []string
}

// ParsePyProject loads and parses a standard pyproject.toml file.
// It applies a strict heuristic filter: it returns (nil, nil) if the file is a generic/non-environment
// configuration file (such as tool-only/linter configurations like those in depot_tools).
// It returns an error if the file has a valid [project] table but is syntactically invalid or fails to parse.
func ParsePyProject(path string) (*ProjectSpec, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, errors.Fmt("failed to read pyproject.toml at: %s: %w", path, err)
	}
	return parsePyProjectContent(string(content))
}

// parsePyProjectContent parses the string content of a pyproject.toml file with the heuristic check.
func parsePyProjectContent(content string) (*ProjectSpec, error) {
	// Decode into standard structured schema using a pointer for single-pass heuristic checks.
	var schema struct {
		Project *struct {
			Name           string   `toml:"name"`
			RequiresPython string   `toml:"requires-python"`
			Dependencies   []string `toml:"dependencies"`
		} `toml:"project"`
	}
	if err := toml.Unmarshal([]byte(content), &schema); err != nil {
		return nil, errors.Fmt("failed to parse TOML structured schema: %w", err)
	}

	// Heuristic Check: If the [project] table is completely missing, ignore silently (non-environment config like Ruff/Black).
	if schema.Project == nil {
		return nil, nil
	}

	spec := &ProjectSpec{
		Name:           schema.Project.Name,
		RequiresPython: schema.Project.RequiresPython,
		Dependencies:   schema.Project.Dependencies,
	}

	// Double check: if the [project] table was present but is completely empty/invalid
	if spec.Name == "" && spec.RequiresPython == "" && len(spec.Dependencies) == 0 {
		return nil, nil
	}

	return spec, nil
}
