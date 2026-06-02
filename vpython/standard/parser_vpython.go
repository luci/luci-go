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

	"github.com/pelletier/go-toml/v2"

	"go.chromium.org/luci/common/errors"
)

// ProjectSpec represents the standard environment metadata extracted from a vpython.toml file.
type ProjectSpec struct {
	RequiresPython string   `toml:"requires-python,omitempty"`
	Dependencies   []string `toml:"dependencies,omitempty" multiline:"true"`
}

// ParseVpythonTOML loads and parses a standard vpython.toml spec file.
func ParseVpythonTOML(path string) (*ProjectSpec, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, errors.Fmt("failed to read vpython.toml at: %s: %w", path, err)
	}
	return parseVpythonTOMLContent(content)
}

// parseVpythonTOMLContent parses the byte slice content of a flat vpython.toml spec file.
func parseVpythonTOMLContent(content []byte) (*ProjectSpec, error) {
	var spec ProjectSpec
	if err := toml.Unmarshal(content, &spec); err != nil {
		return nil, errors.Fmt("failed to parse TOML: %w", err)
	}

	// Double check: if the spec was present but is completely empty/invalid
	if spec.RequiresPython == "" && len(spec.Dependencies) == 0 {
		return nil, errors.New("empty spec file")
	}

	return &spec, nil
}
