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
	"path/filepath"
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestTOMLParser(tT *testing.T) {
	tT.Parallel()

	ftt.Run("Standard vpython.toml Parsing", tT, func(t *ftt.Test) {
		t.Run("Successfully parses valid dependency-active projects", func(t *ftt.Test) {
			validTOML := `
requires-python = ">=3.11"
dependencies = [
    "requests>=2.31.0",
    "numpy>=1.24.0",
]
`
			spec, err := parseVpythonTOMLContent([]byte(validTOML))
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, spec, should.NotBeNil)
			assert.Loosely(t, spec.RequiresPython, should.Equal(">=3.11"))
			assert.Loosely(t, spec.Dependencies, should.Match([]string{
				"requests>=2.31.0",
				"numpy>=1.24.0",
			}))
		})

		t.Run("Fails strictly for linter-only / tool-only files", func(t *ftt.Test) {
			linterTOML := `
[tool.black]
line-length = 80
include = '\.py$'

[tool.ruff]
select = ["E", "F"]
ignore = ["E501"]
`
			_, err := parseVpythonTOMLContent([]byte(linterTOML))
			assert.Loosely(t, err, should.NotBeNil)
			assert.Loosely(t, err.Error(), should.ContainSubstring("empty spec file"))
		})

		t.Run("Fails strictly for empty or zeroed tables", func(t *ftt.Test) {
			emptyTOML := `
# Just comments and empty space
`
			_, err := parseVpythonTOMLContent([]byte(emptyTOML))
			assert.Loosely(t, err, should.NotBeNil)
			assert.Loosely(t, err.Error(), should.ContainSubstring("empty spec file"))
		})

		t.Run("Returns error on invalid TOML syntax", func(t *ftt.Test) {
			invalidTOML := `
dependencies = [
    "requests",  # Missing closing quote
`
			_, err := parseVpythonTOMLContent([]byte(invalidTOML))
			assert.Loosely(t, err, should.NotBeNil)
		})

		t.Run("Successfully loads and parses from a physical disk file path", func(t *ftt.Test) {
			tmpPath := filepath.Join(tT.TempDir(), "vpython.toml")
			content := `
requires-python = ">=3.11"
`
			err := os.WriteFile(tmpPath, []byte(content), 0644)
			assert.Loosely(t, err, should.BeNil)

			spec, err := ParseVpythonTOML(tmpPath)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, spec, should.NotBeNil)
			assert.Loosely(t, spec.RequiresPython, should.Equal(">=3.11"))
		})

		t.Run("Returns error when physical file does not exist", func(t *ftt.Test) {
			nonExistentPath := filepath.Join(tT.TempDir(), "non_existent_file_path_xyz_123.toml")
			_, err := ParseVpythonTOML(nonExistentPath)
			assert.Loosely(t, err, should.NotBeNil)
		})

		t.Run("Returns error when valid TOML has invalid schema types", func(t *ftt.Test) {
			typeMismatchTOML := `
dependencies = "this-should-be-a-list-but-is-a-string"
`
			_, err := parseVpythonTOMLContent([]byte(typeMismatchTOML))
			assert.Loosely(t, err, should.NotBeNil)
		})
	})
}
