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

func TestPEP723ParserAndScanners(tT *testing.T) {
	tT.Parallel()

	ftt.Run("PEP 723 Script Metadata Parsing", tT, func(t *ftt.Test) {
		t.Run("Successfully parses valid script inline metadata", func(t *ftt.Test) {
			scriptContent := `#!/usr/bin/env vpython3
# /// script
# requires-python = ">=3.11"
# dependencies = [
#     "requests>=2.31.0",
#     "numpy>=1.24.0",
# ]
# ///

import requests
import numpy as np
print("Running standalone script!")
`
			spec, err := parseScriptMetadataContent([]byte(scriptContent))
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, spec, should.NotBeNil)
			assert.Loosely(t, spec.Name, should.BeEmpty) // Name remains empty for standalone scripts
			assert.Loosely(t, spec.RequiresPython, should.Equal(">=3.11"))
			assert.Loosely(t, spec.Dependencies, should.Match([]string{
				"requests>=2.31.0",
				"numpy>=1.24.0",
			}))
		})

		t.Run("Returns error when a non-comment line is found inside the metadata block", func(t *ftt.Test) {
			brokenScript := `
# /// script
# requires-python = ">=3.11"
this-line-is-not-commented-out = true
# dependencies = ["requests"]
# ///
`
			_, err := parseScriptMetadataContent([]byte(brokenScript))
			assert.Loosely(t, err, should.NotBeNil)
			assert.Loosely(t, err.Error(), should.ContainSubstring("non-comment line inside metadata block"))
		})

		t.Run("Returns error when closing sentinel is missing (unterminated block)", func(t *ftt.Test) {
			unterminatedScript := `
# /// script
# requires-python = ">=3.11"
# dependencies = ["requests"]
`
			_, err := parseScriptMetadataContent([]byte(unterminatedScript))
			assert.Loosely(t, err, should.NotBeNil)
			assert.Loosely(t, err.Error(), should.ContainSubstring("unterminated PEP 723 script metadata block (missing closing # /// sentinel)"))
		})

		t.Run("Silently ignores scripts missing a PEP 723 metadata block", func(t *ftt.Test) {
			plainScript := `
import os
print("No inline metadata here!")
`
			spec, err := parseScriptMetadataContent([]byte(plainScript))
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, spec, should.BeNil)
		})

		t.Run("Silently ignores empty blocks", func(t *ftt.Test) {
			emptyBlockScript := `
# /// script
# ///
`
			spec, err := parseScriptMetadataContent([]byte(emptyBlockScript))
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, spec, should.BeNil)
		})

		t.Run("Successfully loads and parses from a physical disk script file path", func(t *ftt.Test) {
			tmpPath := filepath.Join(tT.TempDir(), "script_test.py")
			content := `
# /// script
# requires-python = ">=3.11"
# dependencies = ["requests"]
# ///
`
			err := os.WriteFile(tmpPath, []byte(content), 0644)
			assert.Loosely(t, err, should.BeNil)

			spec, err := ParseScriptMetadata(tmpPath)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, spec, should.NotBeNil)
			assert.Loosely(t, spec.RequiresPython, should.Equal(">=3.11"))
			assert.Loosely(t, spec.Dependencies, should.Match([]string{"requests"}))
		})

		t.Run("Returns error when the physical file does not exist", func(t *ftt.Test) {
			nonExistentPath := filepath.Join(tT.TempDir(), "non_existent_script_file_path_xyz.py")
			_, err := ParseScriptMetadata(nonExistentPath)
			assert.Loosely(t, err, should.NotBeNil)
			assert.Loosely(t, err.Error(), should.ContainSubstring("failed to open script file at"))
		})

		t.Run("Returns error when standard field has a valid TOML syntax but invalid schema types", func(t *ftt.Test) {
			typeMismatchScript := `
# /// script
# requires-python = ">=3.11"
# dependencies = "should-be-a-list-but-is-a-string"
# ///
`
			_, err := parseScriptMetadataContent([]byte(typeMismatchScript))
			assert.Loosely(t, err, should.NotBeNil)
			assert.Loosely(t, err.Error(), should.ContainSubstring("failed to decode PEP 723 TOML schema"))
		})

		t.Run("Returns error when scanner read fails", func(t *ftt.Test) {
			_, err := parseScriptMetadataReader(errReader{err: os.ErrPermission})
			assert.Loosely(t, err, should.NotBeNil)
			assert.Loosely(t, err.Error(), should.ContainSubstring("error scanning script content"))
		})

		t.Run("Silently ignores blocks with unparsed metadata fields only", func(t *ftt.Test) {
			unparsedScript := `
# /// script
# my-custom-field = "hello"
# [some-other-table]
# foo = "bar"
# ///
`
			spec, err := parseScriptMetadataContent([]byte(unparsedScript))
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, spec, should.BeNil) // Silently skipped!
		})

		t.Run("Preserves significant trailing whitespace inside block lines", func(t *ftt.Test) {
			scriptWithTrailing := `
# /// script
# dependencies = [
#     "requests; sys_platform == 'win32'  ",
# ]
# ///
`
			spec, err := parseScriptMetadataContent([]byte(scriptWithTrailing))
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, spec, should.NotBeNil)
			assert.Loosely(t, spec.Dependencies, should.Match([]string{
				"requests; sys_platform == 'win32'  ",
			}))
		})

		t.Run("Returns error when a metadata block is placed after executable Python code statements", func(t *ftt.Test) {
			misplacedScript := `#!/usr/bin/env vpython3
import sys
print("Code before sentinel!")

# /// script
# dependencies = ["requests"]
# ///
`
			_, err := parseScriptMetadataContent([]byte(misplacedScript))
			assert.Loosely(t, err, should.NotBeNil)
			assert.Loosely(t, err.Error(), should.ContainSubstring("block must be placed before any executable code or docstring statements"))
		})

		t.Run("Returns error when a metadata block is placed after module-level docstrings", func(t *ftt.Test) {
			docstringScript := `"""Module-level docstring statement."""
# /// script
# requires-python = ">=3.11"
# ///
`
			_, err := parseScriptMetadataContent([]byte(docstringScript))
			assert.Loosely(t, err, should.NotBeNil)
			assert.Loosely(t, err.Error(), should.ContainSubstring("block must be placed before any executable code or docstring statements"))
		})
	})
}

type errReader struct {
	err error
}

func (r errReader) Read(p []byte) (int, error) {
	return 0, r.err
}
