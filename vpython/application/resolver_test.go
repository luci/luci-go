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

package application

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	"go.chromium.org/luci/vpython/python"
)

func TestResolveFlow_ScriptTarget(t *testing.T) {
	ctx := context.Background()

	ftt.Run("Test ResolveFlow script-specific targets", t, func(t *ftt.Test) {
		tempDir := t.TempDir()

		t.Run("Resolves to FlowUV when PEP 723 script shebang is present", func(t *ftt.Test) {
			scriptPath := filepath.Join(tempDir, "script.py")
			scriptContent := `
# /// script
# requires-python = ">=3.11"
# dependencies = ["requests>=2.0"]
# ///
print("OK")
`
			err := os.WriteFile(scriptPath, []byte(strings.TrimSpace(scriptContent)), 0755)
			assert.NoErr(t, err)

			res, err := ResolveFlow(ctx, python.ScriptTarget{Path: scriptPath}, "", "", ".vpython3", filepath.Dir(scriptPath))
			assert.NoErr(t, err)
			assert.Loosely(t, res.Flow, should.Equal(FlowUV))
			assert.Loosely(t, res.StandardSpec.RequiresPython, should.Equal(">=3.11"))
			assert.Loosely(t, res.StandardSpec.Dependencies, should.Resemble([]string{"requests>=2.0"}))
			assert.Loosely(t, res.ProjectRoot, should.Equal(tempDir))
		})

		t.Run("Resolves to FlowLegacy when inline wheels spec is present", func(t *ftt.Test) {
			scriptPath := filepath.Join(tempDir, "script.py")
			scriptContent := `
# [VPYTHON:BEGIN]
# python_version: "3.8"
# [VPYTHON:END]
print("OK")
`
			err := os.WriteFile(scriptPath, []byte(strings.TrimSpace(scriptContent)), 0755)
			assert.NoErr(t, err)

			res, err := ResolveFlow(ctx, python.ScriptTarget{Path: scriptPath}, "", "", ".vpython3", filepath.Dir(scriptPath))
			assert.NoErr(t, err)
			assert.Loosely(t, res.Flow, should.Equal(FlowLegacy))
			assert.Loosely(t, res.VpythonSpec.PythonVersion, should.Equal("3.8"))
		})

		t.Run("Resolves to FlowLegacy when adjacent companion spec is present", func(t *ftt.Test) {
			scriptPath := filepath.Join(tempDir, "script.py")
			scriptContent := `print("OK")`
			err := os.WriteFile(scriptPath, []byte(strings.TrimSpace(scriptContent)), 0755)
			assert.NoErr(t, err)

			// Adjacent spec file
			specPath := scriptPath + ".vpython3"
			specContent := `python_version: "3.11"`
			err = os.WriteFile(specPath, []byte(strings.TrimSpace(specContent)), 0644)
			assert.NoErr(t, err)

			res, err := ResolveFlow(ctx, python.ScriptTarget{Path: scriptPath}, "", "", ".vpython3", filepath.Dir(scriptPath))
			assert.NoErr(t, err)
			assert.Loosely(t, res.Flow, should.Equal(FlowLegacy))
			assert.Loosely(t, res.VpythonSpec.PythonVersion, should.Equal("3.11"))
		})
	})
}

func TestResolveFlow_Climbing(t *testing.T) {
	ctx := context.Background()

	ftt.Run("Test ResolveFlow parent climbing traversal", t, func(t *ftt.Test) {
		root := t.TempDir()
		subDir := filepath.Join(root, "level1", "level2")
		err := os.MkdirAll(subDir, 0755)
		assert.NoErr(t, err)

		scriptPath := filepath.Join(subDir, "script.py")
		err = os.WriteFile(scriptPath, []byte(`print("OK")`), 0755)
		assert.NoErr(t, err)

		t.Run("Climbs up to find a valid vpython.toml environment spec", func(t *ftt.Test) {
			// Place vpython.toml at root
			tomlPath := filepath.Join(root, "vpython.toml")
			tomlContent := `
requires-python = ">=3.8"
dependencies = ["numpy"]
`
			err := os.WriteFile(tomlPath, []byte(strings.TrimSpace(tomlContent)), 0644)
			assert.NoErr(t, err)

			res, err := ResolveFlow(ctx, python.ScriptTarget{Path: scriptPath}, "", "", ".vpython3", filepath.Dir(scriptPath))
			assert.NoErr(t, err)
			assert.Loosely(t, res.Flow, should.Equal(FlowUV))
			assert.Loosely(t, res.StandardSpec.RequiresPython, should.Equal(">=3.8"))
			assert.Loosely(t, res.ProjectRoot, should.Equal(root))
			assert.Loosely(t, res.FromVPythonTOML, should.BeTrue)
		})

		t.Run("Stops climbing at .gclient stop barrier and falls back cleanly", func(t *ftt.Test) {
			// Place .gclient stop barrier at level1
			gclientPath := filepath.Join(root, "level1", ".gclient")
			err := os.WriteFile(gclientPath, []byte(""), 0644)
			assert.NoErr(t, err)

			// Place valid vpython.toml at root (above stop barrier)
			tomlPath := filepath.Join(root, "vpython.toml")
			tomlContent := `
requires-python = ">=3.8"
`
			err = os.WriteFile(tomlPath, []byte(strings.TrimSpace(tomlContent)), 0644)
			assert.NoErr(t, err)

			res, err := ResolveFlow(ctx, python.ScriptTarget{Path: scriptPath}, "", "", ".vpython3", filepath.Dir(scriptPath))
			assert.NoErr(t, err)
			assert.Loosely(t, res.Flow, should.Equal(FlowLegacy))            // Bypassed root UV spec due to stop barrier
			assert.Loosely(t, res.VpythonSpec.PythonVersion, should.BeEmpty) // Mapped to clean default spec
		})

		t.Run("Does not hang and exits cleanly when starting search from filesystem root", func(t *ftt.Test) {
			rootPath := "/"
			if runtime.GOOS == "windows" {
				rootPath = "C:\\"
			}
			res, err := ResolveFlow(ctx, python.NoTarget{}, "", "", ".vpython3", rootPath)
			assert.NoErr(t, err)
			assert.Loosely(t, res.Flow, should.Equal(FlowLegacy))
			assert.Loosely(t, res.VpythonSpec.PythonVersion, should.BeEmpty)
		})

		t.Run("Fails with clear error for invalid explicit TOML spec", func(t *ftt.Test) {
			tempDir := t.TempDir()
			tomlPath := filepath.Join(tempDir, "invalid.toml")
			tomlContent := `
[tool.some-tool]
key = "value"
`
			err := os.WriteFile(tomlPath, []byte(strings.TrimSpace(tomlContent)), 0644)
			assert.NoErr(t, err)

			_, err = ResolveFlow(ctx, python.NoTarget{}, tomlPath, "", ".vpython3", tempDir)
			assert.Loosely(t, err, should.NotBeNil)
			assert.Loosely(t, err.Error(), should.ContainSubstring("is invalid: empty spec file"))
		})
	})
}
