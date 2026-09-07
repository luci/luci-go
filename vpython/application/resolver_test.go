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

		t.Run("Resolves to companion spec of symlink before inline spec of target (Chromite pattern)", func(t *ftt.Test) {
			if runtime.GOOS == "windows" {
				t.Skip("Symlinks are not reliably supported on Windows tests")
			}

			targetPath := filepath.Join(tempDir, "vpython_wrapper.py")
			targetContent := `
# [VPYTHON:BEGIN]
# python_version: "3.8"
# [VPYTHON:END]
print("OK")
`
			err := os.WriteFile(targetPath, []byte(strings.TrimSpace(targetContent)), 0755)
			assert.NoErr(t, err)

			symlinkPath := filepath.Join(tempDir, "run_tests")
			err = os.Symlink(targetPath, symlinkPath)
			assert.NoErr(t, err)

			specPath := symlinkPath + ".vpython3"
			specContent := `python_version: "3.11"`
			err = os.WriteFile(specPath, []byte(strings.TrimSpace(specContent)), 0644)
			assert.NoErr(t, err)

			res, err := ResolveFlow(ctx, python.ScriptTarget{Path: symlinkPath}, "", "", ".vpython3", tempDir)
			assert.NoErr(t, err)
			assert.Loosely(t, res.Flow, should.Equal(FlowLegacy))
			assert.Loosely(t, res.VpythonSpec.PythonVersion, should.Equal("3.11"))
		})

		t.Run("Resolves to companion spec of target if symlink has no companion spec", func(t *ftt.Test) {
			if runtime.GOOS == "windows" {
				t.Skip("Symlinks are not reliably supported on Windows tests")
			}

			targetPath := filepath.Join(tempDir, "target.py")
			targetContent := `print("OK")`
			err := os.WriteFile(targetPath, []byte(strings.TrimSpace(targetContent)), 0755)
			assert.NoErr(t, err)

			targetSpecPath := targetPath + ".vpython3"
			targetSpecContent := `python_version: "3.11"`
			err = os.WriteFile(targetSpecPath, []byte(strings.TrimSpace(targetSpecContent)), 0644)
			assert.NoErr(t, err)

			symlinkPath := filepath.Join(tempDir, "symlink")
			err = os.Symlink(targetPath, symlinkPath)
			assert.NoErr(t, err)

			res, err := ResolveFlow(ctx, python.ScriptTarget{Path: symlinkPath}, "", "", ".vpython3", tempDir)
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
			assert.Loosely(t, res.FromVpythonTOML, should.BeTrue)
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

func TestResolveFlow_CommandTarget(t *testing.T) {
	ctx := context.Background()

	ftt.Run("Test ResolveFlow command-string specific targets", t, func(t *ftt.Test) {
		tempDir := t.TempDir()

		t.Run("Resolves to FlowUV when PEP 723 block is present inside command string", func(t *ftt.Test) {
			code := `
# /// script
# requires-python = ">=3.11"
# dependencies = ["requests>=2.0"]
# ///
print("OK")
`
			target := python.CommandTarget{Command: strings.TrimSpace(code)}
			res, err := ResolveFlow(ctx, target, "", "", ".vpython3", tempDir)
			assert.NoErr(t, err)
			assert.Loosely(t, res.Flow, should.Equal(FlowUV))
			assert.Loosely(t, res.StandardSpec.RequiresPython, should.Equal(">=3.11"))
			assert.Loosely(t, res.StandardSpec.Dependencies, should.Resemble([]string{"requests>=2.0"}))
			assert.Loosely(t, res.ProjectRoot, should.Equal(tempDir))
		})

		t.Run("Fails fast with error when PEP 723 block inside command string is corrupted/invalid", func(t *ftt.Test) {
			code := `
# /// script
# requires-python = >=3.11
# ///
print("OK")
`
			target := python.CommandTarget{Command: strings.TrimSpace(code)}
			_, err := ResolveFlow(ctx, target, "", "", ".vpython3", tempDir)
			assert.Loosely(t, err, should.NotBeNil)
			assert.Loosely(t, err.Error(), should.ContainSubstring("failed to decode PEP 723 TOML schema"))
		})

		t.Run("Falls back gracefully to parent climbing when PEP 723 block is missing inside command string", func(t *ftt.Test) {
			// Pre-create a parent default TOML spec
			tomlPath := filepath.Join(tempDir, "vpython.toml")
			tomlContent := `
requires-python = ">=3.8"
`
			err := os.WriteFile(tomlPath, []byte(strings.TrimSpace(tomlContent)), 0644)
			assert.NoErr(t, err)

			code := `print("NO INLINE SPECS")`
			target := python.CommandTarget{Command: strings.TrimSpace(code)}

			res, err := ResolveFlow(ctx, target, "", "", ".vpython3", tempDir)
			assert.NoErr(t, err)
			assert.Loosely(t, res.Flow, should.Equal(FlowUV)) // Found standard parent spec!
			assert.Loosely(t, res.StandardSpec.RequiresPython, should.Equal(">=3.8"))
			assert.Loosely(t, res.ProjectRoot, should.Equal(tempDir))
		})
	})
}

func TestResolveFlow_ExplicitSpecFallback(t *testing.T) {
	ctx := t.Context()

	ftt.Run("ExplicitSpecFallback", t, func(t *ftt.Test) {
		tempDir := t.TempDir()

		// Scenario b/557060880: caller explicitly requests .vpython3, but repo migrated to vpython.toml.
		t.Run("legacy_to_standard", func(t *ftt.Test) {
			tomlPath := filepath.Join(tempDir, "vpython.toml")
			tomlContent := `
requires-python = ">=3.11"
dependencies = ["requests>=2.0"]
`
			err := os.WriteFile(tomlPath, []byte(strings.TrimSpace(tomlContent)), 0644)
			assert.NoErr(t, err)

			// Caller explicitly requests .vpython3 which does NOT exist on disk.
			explicitSpec := filepath.Join(tempDir, ".vpython3")
			res, err := ResolveFlow(ctx, python.NoTarget{}, explicitSpec, "", ".vpython3", tempDir)
			assert.NoErr(t, err)
			assert.Loosely(t, res.Flow, should.Equal(FlowUV))
			assert.Loosely(t, res.SpecPath, should.Equal(tomlPath))
			assert.Loosely(t, res.StandardSpec.RequiresPython, should.Equal(">=3.11"))
			assert.Loosely(t, res.StandardSpec.Dependencies, should.Resemble([]string{"requests>=2.0"}))
		})

		// Scenario recipes-py revert: caller explicitly requests vpython.toml, but pinned repo only has .vpython3.
		t.Run("standard_to_legacy", func(t *ftt.Test) {
			legacyPath := filepath.Join(tempDir, ".vpython3")
			legacyContent := `
python_version: "3.11"
wheel: <
  name: "infra/python/wheels/six-py2_py3"
  version: "version:1.16.0"
>
`
			err := os.WriteFile(legacyPath, []byte(strings.TrimSpace(legacyContent)), 0644)
			assert.NoErr(t, err)

			// Caller explicitly requests vpython.toml which does NOT exist on disk.
			explicitSpec := filepath.Join(tempDir, "vpython.toml")
			res, err := ResolveFlow(ctx, python.NoTarget{}, explicitSpec, "", ".vpython3", tempDir)
			assert.NoErr(t, err)
			assert.Loosely(t, res.Flow, should.Equal(FlowLegacy))
			assert.Loosely(t, res.SpecPath, should.Equal(legacyPath))
			assert.Loosely(t, res.VpythonSpec.PythonVersion, should.Equal("3.11"))
			assert.Loosely(t, len(res.VpythonSpec.Wheel), should.Equal(1))
		})

		// Caller explicitly requests named legacy spec (e.g. gsutil.py.vpython3), repo has gsutil.vpython.toml.
		t.Run("named_legacy_to_standard", func(t *ftt.Test) {
			subDir := filepath.Join(tempDir, "named_spec")
			err := os.MkdirAll(subDir, 0755)
			assert.NoErr(t, err)

			tomlPath := filepath.Join(subDir, "gsutil.vpython.toml")
			tomlContent := `
requires-python = ">=3.11"
dependencies = ["google-cloud-storage>=2.0"]
`
			err = os.WriteFile(tomlPath, []byte(strings.TrimSpace(tomlContent)), 0644)
			assert.NoErr(t, err)

			// Caller explicitly requests gsutil.py.vpython3 or gsutil.vpython3.
			explicitSpec := filepath.Join(subDir, "gsutil.py.vpython3")
			res, err := ResolveFlow(ctx, python.NoTarget{}, explicitSpec, "", ".vpython3", subDir)
			assert.NoErr(t, err)
			assert.Loosely(t, res.Flow, should.Equal(FlowUV))
			assert.Loosely(t, res.SpecPath, should.Equal(tomlPath))
			assert.Loosely(t, res.StandardSpec.Dependencies, should.Resemble([]string{"google-cloud-storage>=2.0"}))
		})

		// Caller explicitly requests named toml (e.g. pycharm.vpython.toml), repo has .pycharm.vpython3.
		t.Run("named_standard_to_legacy", func(t *ftt.Test) {
			subDir := filepath.Join(tempDir, "debugger_spec")
			err := os.MkdirAll(subDir, 0755)
			assert.NoErr(t, err)

			legacyPath := filepath.Join(subDir, ".pycharm.vpython3")
			legacyContent := `
python_version: "3.11"
wheel: <
  name: "infra/python/wheels/pydevd-py3"
  version: "version:2.8.0"
>
`
			err = os.WriteFile(legacyPath, []byte(strings.TrimSpace(legacyContent)), 0644)
			assert.NoErr(t, err)

			// Caller explicitly requests pycharm.vpython.toml.
			explicitSpec := filepath.Join(subDir, "pycharm.vpython.toml")
			res, err := ResolveFlow(ctx, python.NoTarget{}, explicitSpec, "", ".vpython3", subDir)
			assert.NoErr(t, err)
			assert.Loosely(t, res.Flow, should.Equal(FlowLegacy))
			assert.Loosely(t, res.SpecPath, should.Equal(legacyPath))
			assert.Loosely(t, res.VpythonSpec.PythonVersion, should.Equal("3.11"))
		})

		// Explicit spec and all candidate companions are missing.
		t.Run("nonexistent", func(t *ftt.Test) {
			missingSpec := filepath.Join(tempDir, "completely_missing.vpython3")
			_, err := ResolveFlow(ctx, python.NoTarget{}, missingSpec, "", ".vpython3", tempDir)
			assert.Loosely(t, err, should.ErrLike(os.ErrNotExist))

			missingToml := filepath.Join(tempDir, "completely_missing.vpython.toml")
			_, err = ResolveFlow(ctx, python.NoTarget{}, missingToml, "", ".vpython3", tempDir)
			assert.Loosely(t, err, should.ErrLike(os.ErrNotExist))
		})

		// Explicit spec exists on disk, so it takes precedence without fallback even if companion exists.
		t.Run("explicit_precedence", func(t *ftt.Test) {
			subDir := filepath.Join(tempDir, "both_exist")
			err := os.MkdirAll(subDir, 0755)
			assert.NoErr(t, err)

			legacyPath := filepath.Join(subDir, ".vpython3")
			legacyContent := `python_version: "3.8"`
			err = os.WriteFile(legacyPath, []byte(strings.TrimSpace(legacyContent)), 0644)
			assert.NoErr(t, err)

			tomlPath := filepath.Join(subDir, "vpython.toml")
			tomlContent := `
requires-python = ">=3.11"
dependencies = ["six"]
`
			err = os.WriteFile(tomlPath, []byte(strings.TrimSpace(tomlContent)), 0644)
			assert.NoErr(t, err)

			// Explicitly asking for .vpython3 uses .vpython3
			resLegacy, err := ResolveFlow(ctx, python.NoTarget{}, legacyPath, "", ".vpython3", subDir)
			assert.NoErr(t, err)
			assert.Loosely(t, resLegacy.Flow, should.Equal(FlowLegacy))
			assert.Loosely(t, resLegacy.VpythonSpec.PythonVersion, should.Equal("3.8"))

			// Explicitly asking for vpython.toml uses vpython.toml
			resUV, err := ResolveFlow(ctx, python.NoTarget{}, tomlPath, "", ".vpython3", subDir)
			assert.NoErr(t, err)
			assert.Loosely(t, resUV.Flow, should.Equal(FlowUV))
			assert.Loosely(t, resUV.StandardSpec.RequiresPython, should.Equal(">=3.11"))
		})
	})
}
