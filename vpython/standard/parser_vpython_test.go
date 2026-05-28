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
[project]
name = "my-chrome-tool"
version = "1.0.0"
requires-python = ">=3.11"
dependencies = [
    "requests>=2.31.0",
    "numpy>=1.24.0",
]
`
			spec, err := parseVPythonTOMLContent([]byte(validTOML))
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, spec, should.NotBeNil)
			assert.Loosely(t, spec.Name, should.Equal("my-chrome-tool"))
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
			_, err := parseVPythonTOMLContent([]byte(linterTOML))
			assert.Loosely(t, err, should.NotBeNil)
			assert.Loosely(t, err.Error(), should.ContainSubstring("missing [project] table"))
		})

		t.Run("Fails strictly for empty or zeroed tables", func(t *ftt.Test) {
			emptyTOML := `
# Just comments and empty space
`
			_, err := parseVPythonTOMLContent([]byte(emptyTOML))
			assert.Loosely(t, err, should.NotBeNil)
			assert.Loosely(t, err.Error(), should.ContainSubstring("missing [project] table"))
		})

		t.Run("Returns error on invalid TOML syntax inside project block", func(t *ftt.Test) {
			invalidTOML := `
[project]
name = "broken"
dependencies = [
    "requests",  # Missing closing quote
`
			_, err := parseVPythonTOMLContent([]byte(invalidTOML))
			assert.Loosely(t, err, should.NotBeNil)
		})

		t.Run("Successfully loads and parses from a physical disk file path", func(t *ftt.Test) {
			tmpPath := filepath.Join(tT.TempDir(), "vpython.toml")
			content := `
[project]
name = "file-test"
requires-python = ">=3.11"
`
			err := os.WriteFile(tmpPath, []byte(content), 0644)
			assert.Loosely(t, err, should.BeNil)

			spec, err := ParseVPythonTOML(tmpPath)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, spec, should.NotBeNil)
			assert.Loosely(t, spec.Name, should.Equal("file-test"))
			assert.Loosely(t, spec.RequiresPython, should.Equal(">=3.11"))
		})

		t.Run("Returns error when physical file does not exist", func(t *ftt.Test) {
			nonExistentPath := filepath.Join(tT.TempDir(), "non_existent_file_path_xyz_123.toml")
			_, err := ParseVPythonTOML(nonExistentPath)
			assert.Loosely(t, err, should.NotBeNil)
		})

		t.Run("Returns error when [project] table has valid TOML but invalid schema types", func(t *ftt.Test) {
			typeMismatchTOML := `
[project]
name = "type-mismatch"
dependencies = "this-should-be-a-list-but-is-a-string"
`
			_, err := parseVPythonTOMLContent([]byte(typeMismatchTOML))
			assert.Loosely(t, err, should.NotBeNil)
		})
	})
}
