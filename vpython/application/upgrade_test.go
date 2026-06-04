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

//go:build !windows

package application

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/logging/memlogger"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	"go.chromium.org/luci/vpython/standard"
)

func TestUpgradeSpecs_Standalone(t *testing.T) {
	ctx := context.Background()

	ftt.Run("Test standalone specs migration", t, func(t *ftt.Test) {
		tempDir := t.TempDir()

		legacyContent := `
python_version: "3.8"
wheel {
  name: "infra/python/wheels/requests-py2_py3"
  version: "version:2.22.0"
}
`

		t.Run("Converts .vpython file to vpython.toml and deletes legacy file by default", func(t *ftt.Test) {
			srcPath := filepath.Join(tempDir, ".vpython3")
			err := os.WriteFile(srcPath, []byte(strings.TrimSpace(legacyContent)), 0644)
			assert.NoErr(t, err)

			err = UpgradeSpecs(ctx, srcPath, false, true, true) // keepLegacy = false
			assert.NoErr(t, err)

			// Verify original legacy file was deleted
			_, err = os.Stat(srcPath)
			assert.Loosely(t, errors.Is(err, os.ErrNotExist), should.BeTrue)

			// Verify vpython.toml exists and has correct translated properties
			tomlPath := filepath.Join(tempDir, "vpython.toml")
			spec, err := standard.ParseVpythonTOML(tomlPath)
			assert.NoErr(t, err)
			assert.Loosely(t, spec.RequiresPython, should.Equal(">=3.8,<3.9"))
			assert.Loosely(t, spec.Dependencies, should.Match([]string{"requests==2.22.0"}))

			// Assert raw multiline string format for readability
			tomlData, err := os.ReadFile(tomlPath)
			assert.NoErr(t, err)
			expectedToml := `
requires-python = '>=3.8,<3.9'
dependencies = [
  'requests==2.22.0'
]
`
			assert.Loosely(t, strings.TrimSpace(string(tomlData)), should.Equal(strings.TrimSpace(expectedToml)))
		})

		t.Run("Converts spec with multiple dependencies to standard PEP 621 array", func(t *ftt.Test) {
			subDir := filepath.Join(tempDir, "multiple-test")
			err := os.MkdirAll(subDir, 0755)
			assert.NoErr(t, err)

			legacyMultipleContent := `
python_version: "3.8"
wheel {
  name: "infra/python/wheels/requests-py2_py3"
  version: "version:2.22.0"
}
wheel {
  name: "infra/python/wheels/numpy-py3"
  version: "version:1.21.0"
}
`
			srcPath := filepath.Join(subDir, ".vpython3")
			err = os.WriteFile(srcPath, []byte(strings.TrimSpace(legacyMultipleContent)), 0644)
			assert.NoErr(t, err)

			err = UpgradeSpecs(ctx, srcPath, false, true, true)
			assert.NoErr(t, err)

			tomlPath := filepath.Join(subDir, "vpython.toml")
			tomlData, err := os.ReadFile(tomlPath)
			assert.NoErr(t, err)

			expectedToml := `
requires-python = '>=3.8,<3.9'
dependencies = [
  'requests==2.22.0',
  'numpy==1.21.0'
]
`
			assert.Loosely(t, strings.TrimSpace(string(tomlData)), should.Equal(strings.TrimSpace(expectedToml)))
		})

		t.Run("Converts .vpython file but preserves it when keepLegacy is active", func(t *ftt.Test) {
			srcPath := filepath.Join(tempDir, ".vpython3")
			err := os.WriteFile(srcPath, []byte(strings.TrimSpace(legacyContent)), 0644)
			assert.NoErr(t, err)

			// Clear any existing vpython.toml
			tomlPath := filepath.Join(tempDir, "vpython.toml")
			_ = os.Remove(tomlPath)

			err = UpgradeSpecs(ctx, srcPath, true, true, true) // keepLegacy = true
			assert.NoErr(t, err)

			// Verify original legacy file still exists
			_, err = os.Stat(srcPath)
			assert.NoErr(t, err)

			// Verify vpython.toml was successfully written
			_, err = os.Stat(tomlPath)
			assert.NoErr(t, err)
		})
	})
}

func TestUpgradeSpecs_Inline(t *testing.T) {
	ctx := context.Background()

	ftt.Run("Test Python script inline spec migrations", t, func(t *ftt.Test) {
		tempDir := t.TempDir()

		scriptContent := `
# Some comment
# [VPYTHON:BEGIN]
# python_version: "3.11"
# # Note: numpy is extremely fast!
# wheel {
#   name: "infra/python/wheels/numpy-py3"
#   version: "version:1.21.0"
# }
# [VPYTHON:END]
print("OK")
`

		t.Run("Converts inline legacy spec block to standard PEP 723 block in-place", func(t *ftt.Test) {
			scriptPath := filepath.Join(tempDir, "script.py")
			err := os.WriteFile(scriptPath, []byte(strings.TrimSpace(scriptContent)), 0755)
			assert.NoErr(t, err)

			err = UpgradeSpecs(ctx, scriptPath, false, true, true)
			assert.NoErr(t, err)

			// Read script and verify content replacement
			data, err := os.ReadFile(scriptPath)
			assert.NoErr(t, err)

			expected := `
# Some comment
# /// script
# requires-python = '>=3.11,<3.12'
# dependencies = [
#   'numpy==1.21.0'
# ]
# ///
print("OK")
`
			assert.Loosely(t, strings.TrimSpace(string(data)), should.Equal(strings.TrimSpace(expected)))
		})

		t.Run("Skips conversion and prints intended TOML when PEP 723 block is already present", func(t *ftt.Test) {
			conflictingContent := `
# /// script
# requires-python = ">=3.8"
# ///
# [VPYTHON:BEGIN]
# python_version: "3.11"
# [VPYTHON:END]
print("OK")
`
			scriptPath := filepath.Join(tempDir, "conflict.py")
			err := os.WriteFile(scriptPath, []byte(strings.TrimSpace(conflictingContent)), 0755)
			assert.NoErr(t, err)

			mctx := memlogger.Use(ctx)

			err = UpgradeSpecs(mctx, scriptPath, false, true, true)
			assert.NoErr(t, err)

			// Verify script remains untouched
			data, err := os.ReadFile(scriptPath)
			assert.NoErr(t, err)
			assert.Loosely(t, strings.TrimSpace(string(data)), should.Equal(strings.TrimSpace(conflictingContent)))

			// Verify warning was captured in the test logger!
			ml := logging.Get(mctx).(*memlogger.MemLogger)
			assert.Loosely(t, len(ml.Messages()), should.BeGreaterThan(0))
			assert.Loosely(t, ml.Messages()[0].Msg, should.ContainSubstring("already contains a standard PEP 723 shebang block! Skipping automated conversion."))
		})
	})
}

func TestUpgradeSpecs_ExtensionlessScript(t *testing.T) {
	ctx := context.Background()

	ftt.Run("Test extensionless Python script spec migrations", t, func(t *ftt.Test) {
		tempDir := t.TempDir()

		scriptContent := `#!/usr/bin/env vpython3
# [VPYTHON:BEGIN]
# python_version: "3.11"
# wheel {
#   name: "infra/python/wheels/numpy-py3"
#   version: "version:1.21.0"
# }
# [VPYTHON:END]
print("OK")
`

		t.Run("Detects shebang and converts inline spec block to PEP 723 in extensionless script", func(t *ftt.Test) {
			scriptPath := filepath.Join(tempDir, "my_python_tool")
			err := os.WriteFile(scriptPath, []byte(strings.TrimSpace(scriptContent)), 0755)
			assert.NoErr(t, err)

			err = UpgradeSpecs(ctx, scriptPath, false, true, true)
			assert.NoErr(t, err)

			// Read script and verify content replacement
			data, err := os.ReadFile(scriptPath)
			assert.NoErr(t, err)

			expected := `#!/usr/bin/env vpython3
# /// script
# requires-python = '>=3.11,<3.12'
# dependencies = [
#   'numpy==1.21.0'
# ]
# ///
print("OK")`
			assert.Loosely(t, strings.TrimSpace(string(data)), should.Equal(strings.TrimSpace(expected)))
		})
	})
}

func TestUpgradeSpecs_Directory(t *testing.T) {
	ctx := context.Background()

	ftt.Run("Test recursive directory specs migration", t, func(t *ftt.Test) {
		tempDir := t.TempDir()

		t.Run("Migrates directories recursively skipping hidden folders", func(t *ftt.Test) {
			sub1 := filepath.Join(tempDir, "sub1")
			sub2 := filepath.Join(tempDir, "sub2")
			hidden := filepath.Join(tempDir, ".git")
			assert.NoErr(t, os.MkdirAll(sub1, 0755))
			assert.NoErr(t, os.MkdirAll(sub2, 0755))
			assert.NoErr(t, os.MkdirAll(hidden, 0755))

			// Standalone spec inside sub1
			specPath := filepath.Join(sub1, ".vpython3")
			specContent := `python_version: "3.11"`
			assert.NoErr(t, os.WriteFile(specPath, []byte(specContent), 0644))

			// Inline script inside sub2
			scriptPath := filepath.Join(sub2, "nested_script.py")
			scriptContent := `
# [VPYTHON:BEGIN]
# python_version: "3.8"
# [VPYTHON:END]
print("OK")
`
			assert.NoErr(t, os.WriteFile(scriptPath, []byte(strings.TrimSpace(scriptContent)), 0755))

			// Legacy spec inside skipped .git folder
			hiddenSpecPath := filepath.Join(hidden, "hidden.vpython3")
			assert.NoErr(t, os.WriteFile(hiddenSpecPath, []byte(specContent), 0644))

			err := UpgradeSpecs(ctx, tempDir, false, true, true)
			assert.NoErr(t, err)

			// 1. Verify standalone spec inside sub1 was migrated and original was deleted
			_, err = os.Stat(specPath)
			assert.Loosely(t, errors.Is(err, os.ErrNotExist), should.BeTrue)
			_, err = os.Stat(filepath.Join(sub1, "vpython.toml"))
			assert.NoErr(t, err)

			// 2. Verify inline script inside sub2 was migrated in-place
			data, err := os.ReadFile(scriptPath)
			assert.NoErr(t, err)
			assert.Loosely(t, string(data), should.ContainSubstring("# /// script"))

			// 3. Verify the .git subdirectory was completely skipped (hiddenSpecPath still exists)
			_, err = os.Stat(hiddenSpecPath)
			assert.NoErr(t, err)
		})

		t.Run("Best-effort walking: continues migrating remaining folders if one is corrupted", func(t *ftt.Test) {
			subResilient := filepath.Join(tempDir, "sub_resilient")
			subCorrupt := filepath.Join(tempDir, "sub_corrupt")
			assert.NoErr(t, os.MkdirAll(subResilient, 0755))
			assert.NoErr(t, os.MkdirAll(subCorrupt, 0755))

			// Healthy standalone spec inside subResilient
			specPath := filepath.Join(subResilient, ".vpython3")
			assert.NoErr(t, os.WriteFile(specPath, []byte(`python_version: "3.11"`), 0644))

			// Corrupted script inside subCorrupt (mismatched guards!)
			corruptScriptPath := filepath.Join(subCorrupt, "corrupt.py")
			corruptContent := `
# [VPYTHON:BEGIN]
# (lacks end guard!)
print("broken")
`
			assert.NoErr(t, os.WriteFile(corruptScriptPath, []byte(strings.TrimSpace(corruptContent)), 0755))

			// Run recursive UpgradeSpecs on the root tempDir!
			err := UpgradeSpecs(ctx, tempDir, false, true, true)
			assert.Loosely(t, err, should.NotBeNil) // Must report the collected error at the end!
			assert.Loosely(t, err.Error(), should.ContainSubstring("contains mismatched inline spec guards"))

			// 1. Verify that the healthy folder subResilient was STILL successfully migrated!
			_, err = os.Stat(specPath)
			assert.Loosely(t, errors.Is(err, os.ErrNotExist), should.BeTrue) // Bypassed file deleted!
			_, err = os.Stat(filepath.Join(subResilient, "vpython.toml"))
			assert.NoErr(t, err) // Created vpython.toml successfully!

			// 2. Verify that the corrupted file remains exactly as it was (uncorrupted!)
			data, err := os.ReadFile(corruptScriptPath)
			assert.NoErr(t, err)
			assert.Loosely(t, string(data), should.Equal(strings.TrimSpace(corruptContent)))
		})
	})
}

func TestUpgradeSpecs_CompanionSpec(t *testing.T) {
	ctx := context.Background()

	ftt.Run("Test companion spec inline merging", t, func(t *ftt.Test) {
		tempDir := t.TempDir()

		scriptContent := `#!/usr/bin/env python3
print("OK")
`
		specContent := `
python_version: "3.8"
wheel {
  name: "infra/python/wheels/requests-py2_py3"
  version: "version:2.22.0"
}
`

		t.Run("Merges companion spec file into script and deletes companion spec file by default", func(t *ftt.Test) {
			scriptPath := filepath.Join(tempDir, "my_script.py")
			err := os.WriteFile(scriptPath, []byte(strings.TrimSpace(scriptContent)), 0755)
			assert.NoErr(t, err)

			specPath := filepath.Join(tempDir, "my_script.py.vpython3")
			err = os.WriteFile(specPath, []byte(strings.TrimSpace(specContent)), 0644)
			assert.NoErr(t, err)

			err = UpgradeSpecs(ctx, specPath, false, true, true) // keepLegacy = false
			assert.NoErr(t, err)

			// Verify original companion spec file was deleted
			_, err = os.Stat(specPath)
			assert.Loosely(t, errors.Is(err, os.ErrNotExist), should.BeTrue)

			// Verify script was modified, keeping the leading shebang line and injecting PEP 723 block
			data, err := os.ReadFile(scriptPath)
			assert.NoErr(t, err)

			expected := `#!/usr/bin/env python3
# /// script
# requires-python = '>=3.8,<3.9'
# dependencies = [
#   'requests==2.22.0'
# ]
# ///

print("OK")`
			assert.Loosely(t, strings.TrimSpace(string(data)), should.Equal(strings.TrimSpace(expected)))
		})

		t.Run("Merges companion spec but preserves it when keepLegacy is active", func(t *ftt.Test) {
			scriptPath := filepath.Join(tempDir, "my_script_keep.py")
			err := os.WriteFile(scriptPath, []byte(strings.TrimSpace(scriptContent)), 0755)
			assert.NoErr(t, err)

			specPath := filepath.Join(tempDir, "my_script_keep.py.vpython3")
			err = os.WriteFile(specPath, []byte(strings.TrimSpace(specContent)), 0644)
			assert.NoErr(t, err)

			err = UpgradeSpecs(ctx, specPath, true, true, true) // keepLegacy = true
			assert.NoErr(t, err)

			// Verify original companion spec file still exists
			_, err = os.Stat(specPath)
			assert.NoErr(t, err)

			// Verify script still was successfully modified
			data, err := os.ReadFile(scriptPath)
			assert.NoErr(t, err)
			assert.Loosely(t, string(data), should.ContainSubstring("# /// script"))
		})
	})
}

func TestUpgradeSpecs_Standalone_ExistingTOML(t *testing.T) {
	ctx := context.Background()

	ftt.Run("Test standalone migrations against existing vpython.toml", t, func(t *ftt.Test) {
		tempDir := t.TempDir()

		legacyContent := `
python_version: "3.8"
wheel {
  name: "infra/python/wheels/requests-py2_py3"
  version: "version:2.22.0"
}
`

		t.Run("Preserves existing vpython.toml file permissions", func(t *ftt.Test) {
			tomlPath := filepath.Join(tempDir, "vpython.toml")
			_ = os.Remove(tomlPath)
			existingToml := `
[tool.pyright]
venv = "normal"
`
			err := os.WriteFile(tomlPath, []byte(strings.TrimSpace(existingToml)), 0400)
			assert.NoErr(t, err)

			srcPath := filepath.Join(tempDir, ".vpython3")
			err = os.WriteFile(srcPath, []byte(strings.TrimSpace(legacyContent)), 0644)
			assert.NoErr(t, err)

			err = UpgradeSpecs(ctx, srcPath, false, true, true)
			assert.NoErr(t, err)

			data, err := os.ReadFile(tomlPath)
			assert.NoErr(t, err)
			assert.Loosely(t, string(data), should.ContainSubstring("requires-python"))
			assert.Loosely(t, string(data), should.NotContainSubstring("pyright")) // Overwritten!

			st, err := os.Stat(tomlPath)
			assert.NoErr(t, err)
			assert.Loosely(t, st.Mode().Perm(), should.Equal(os.FileMode(0400)))

			_ = os.Chmod(tomlPath, 0644)
		})
	})
}

func TestUpgradeSpecs_Python2(t *testing.T) {
	ctx := context.Background()

	ftt.Run("Test legacy Python 2.7 specs are ignored by converter", t, func(t *ftt.Test) {
		tempDir := t.TempDir()

		t.Run("Converts standalone .vpython file to vpython.toml and deletes original", func(t *ftt.Test) {
			srcPath := filepath.Join(tempDir, ".vpython")
			content := `python_version: "2.7"`
			err := os.WriteFile(srcPath, []byte(content), 0644)
			assert.NoErr(t, err)

			err = UpgradeSpecs(ctx, srcPath, false, true, true)
			assert.NoErr(t, err)

			// Verify legacy .vpython was deleted
			_, err = os.Stat(srcPath)
			assert.Loosely(t, errors.Is(err, os.ErrNotExist), should.BeTrue)

			// Verify vpython.toml was created with standard Python 2 constraints
			tomlPath := filepath.Join(tempDir, "vpython.toml")
			spec, err := standard.ParseVpythonTOML(tomlPath)
			assert.NoErr(t, err)
			assert.Loosely(t, spec.RequiresPython, should.Equal(">=2.7,<2.8"))
		})

		t.Run("Skips Python 2 inline script blocks cleanly leaving script untouched", func(t *ftt.Test) {
			scriptContent := `
# [VPYTHON:BEGIN]
# python_version: "2.7"
# [VPYTHON:END]
print "OK"
`
			scriptPath := filepath.Join(tempDir, "script2.py")
			err := os.WriteFile(scriptPath, []byte(strings.TrimSpace(scriptContent)), 0755)
			assert.NoErr(t, err)

			err = UpgradeSpecs(ctx, scriptPath, false, true, true)
			assert.NoErr(t, err)

			// Verify script remains completely untouched
			data, err := os.ReadFile(scriptPath)
			assert.NoErr(t, err)
			assert.Loosely(t, strings.TrimSpace(string(data)), should.Equal(strings.TrimSpace(scriptContent)))
		})
	})
}

func TestUpgradeSpecs_Standalone_Precedence(t *testing.T) {
	ctx := context.Background()

	ftt.Run("Test standalone specs migration precedence", t, func(t *ftt.Test) {
		tempDir := t.TempDir()

		py2SpecPath := filepath.Join(tempDir, ".vpython")
		py3SpecPath := filepath.Join(tempDir, ".vpython3")

		t.Run("Migrates .vpython3 and cleans up obsolete .vpython by default", func(t *ftt.Test) {
			assert.NoErr(t, os.WriteFile(py2SpecPath, []byte(`python_version: "2.7"`), 0644))
			assert.NoErr(t, os.WriteFile(py3SpecPath, []byte(`python_version: "3.11"`), 0644))

			tomlPath := filepath.Join(tempDir, "vpython.toml")
			_ = os.Remove(tomlPath)

			err := UpgradeSpecs(ctx, tempDir, false, true, true)
			assert.NoErr(t, err)

			// 1. Verify that .vpython3 was successfully migrated and deleted
			_, err = os.Stat(py3SpecPath)
			assert.Loosely(t, errors.Is(err, os.ErrNotExist), should.BeTrue)

			// 2. Verify that the obsolete legacy .vpython was cleanly deleted!
			_, err = os.Stat(py2SpecPath)
			assert.Loosely(t, errors.Is(err, os.ErrNotExist), should.BeTrue)

			// 3. Verify that vpython.toml has exactly the correct high-priority .vpython3 spec
			spec, err := standard.ParseVpythonTOML(tomlPath)
			assert.NoErr(t, err)
			assert.Loosely(t, spec.RequiresPython, should.Equal(">=3.11,<3.12"))
		})

		t.Run("Migrates .vpython3 but preserves both files when keepLegacy is active", func(t *ftt.Test) {
			assert.NoErr(t, os.WriteFile(py2SpecPath, []byte(`python_version: "2.7"`), 0644))
			assert.NoErr(t, os.WriteFile(py3SpecPath, []byte(`python_version: "3.11"`), 0644))

			tomlPath := filepath.Join(tempDir, "vpython.toml")
			_ = os.Remove(tomlPath)

			err := UpgradeSpecs(ctx, tempDir, true, true, true)
			assert.NoErr(t, err)

			// Verify both files still exist!
			_, err = os.Stat(py3SpecPath)
			assert.NoErr(t, err)
			_, err = os.Stat(py2SpecPath)
			assert.NoErr(t, err)
		})
	})
}

func TestUpgradeSpecs_UnsupportedTarget(t *testing.T) {
	ctx := context.Background()

	ftt.Run("Test direct specs upgrade with unsupported target file extensions", t, func(t *ftt.Test) {
		tempDir := t.TempDir()

		// Create a mock custom file target with no extensions!
		scriptPath := filepath.Join(tempDir, "my-script")
		err := os.WriteFile(scriptPath, []byte(`print("OK")`), 0755)
		assert.NoErr(t, err)

		mctx := memlogger.Use(ctx)

		// Execute UpgradeSpecs directly on the custom script!
		err = UpgradeSpecs(mctx, scriptPath, false, true, true)
		assert.NoErr(t, err)

		// Verify custom script was NOT deleted!
		_, err = os.Stat(scriptPath)
		assert.NoErr(t, err)

		// Verify clear warning was captured in the test logger!
		ml := logging.Get(mctx).(*memlogger.MemLogger)
		assert.Loosely(t, len(ml.Messages()), should.BeGreaterThan(0))
		assert.Loosely(t, ml.Messages()[0].Msg, should.ContainSubstring("Skipped unsupported file target"))
		assert.Loosely(t, ml.Messages()[0].Msg, should.ContainSubstring("only supports migrating standalone specifications (.vpython, .vpython3) and Python scripts"))
	})
}

func TestUpgradeSpecs_MismatchedGuards(t *testing.T) {
	ctx := context.Background()

	ftt.Run("Test script spec upgrades with mismatched inline guards", t, func(t *ftt.Test) {
		tempDir := t.TempDir()

		// Create a script with a begin guard but missing the end guard!
		scriptPath := filepath.Join(tempDir, "script.py")
		scriptContent := `
# [VPYTHON:BEGIN]
# python_version: "3.11"
# (missing end guard!)
print("OK")
`
		err := os.WriteFile(scriptPath, []byte(strings.TrimSpace(scriptContent)), 0755)
		assert.NoErr(t, err)

		// Execute UpgradeSpecs directly and verify it returns a mismatched guards error!
		err = UpgradeSpecs(ctx, scriptPath, false, true, true)
		assert.Loosely(t, err, should.NotBeNil)
		assert.Loosely(t, err.Error(), should.ContainSubstring("contains mismatched inline spec guards"))
		assert.Loosely(t, err.Error(), should.ContainSubstring("begin index: 0, end index: -1"))

		// Verify original script remains completely uncorrupted and untouched!
		data, err := os.ReadFile(scriptPath)
		assert.NoErr(t, err)
		assert.Loosely(t, string(data), should.Equal(strings.TrimSpace(scriptContent)))
	})
}

func TestUpgradeSpecs_CompanionSpec_Precedence(t *testing.T) {
	ctx := context.Background()

	ftt.Run("Test script upgrade precedence: companion specs override inline specs", t, func(t *ftt.Test) {
		tempDir := t.TempDir()

		// Create a script containing a lower-precedence inline spec!
		scriptPath := filepath.Join(tempDir, "script.py")
		scriptContent := `
# [VPYTHON:BEGIN]
# python_version: "3.8"
# [VPYTHON:END]
print("OK")
`
		err := os.WriteFile(scriptPath, []byte(strings.TrimSpace(scriptContent)), 0755)
		assert.NoErr(t, err)

		// Create a higher-precedence companion spec file!
		specPath := scriptPath + ".vpython3"
		specContent := `python_version: "3.11"`
		err = os.WriteFile(specPath, []byte(strings.TrimSpace(specContent)), 0644)
		assert.NoErr(t, err)

		// Run UpgradeSpecs on the folder containing both!
		err = UpgradeSpecs(ctx, tempDir, false, true, true)
		assert.NoErr(t, err)

		// 1. Verify that the companion spec file was successfully merged and deleted!
		_, err = os.Stat(specPath)
		assert.Loosely(t, errors.Is(err, os.ErrNotExist), should.BeTrue)

		// 2. Verify that the script contains the high-priority "3.11" companion spec constraints!
		// (The lower precedence inline "3.8" spec must be cleanly overwritten and deleted!).
		data, err := os.ReadFile(scriptPath)
		assert.NoErr(t, err)
		assert.Loosely(t, string(data), should.ContainSubstring("# /// script"))
		assert.Loosely(t, string(data), should.ContainSubstring("requires-python = '>=3.11,<3.12'"))
		assert.Loosely(t, string(data), should.NotContainSubstring("requires-python = \">=3.8,<3.9\""))
	})
}

func TestUpgradeSpecs_CustomStandalone(t *testing.T) {
	ctx := context.Background()

	ftt.Run("Test custom standalone specs migration", t, func(t *ftt.Test) {
		tempDir := t.TempDir()

		t.Run("Migrates custom standalone spec with no dependencies and does NOT create lockfile", func(t *ftt.Test) {
			legacyContent := `
python_version: "3.8"
`
			srcPath := filepath.Join(tempDir, "standalone.vpython3")
			err := os.WriteFile(srcPath, []byte(strings.TrimSpace(legacyContent)), 0644)
			assert.NoErr(t, err)

			err = UpgradeSpecs(ctx, srcPath, false, true, true)
			assert.NoErr(t, err)

			// Verify original custom legacy spec was deleted
			_, err = os.Stat(srcPath)
			assert.Loosely(t, errors.Is(err, os.ErrNotExist), should.BeTrue)

			// Verify standalone.vpython.toml was created
			tomlPath := filepath.Join(tempDir, "standalone.vpython.toml")
			spec, err := standard.ParseVpythonTOML(tomlPath)
			assert.NoErr(t, err)
			assert.Loosely(t, spec.RequiresPython, should.Equal(">=3.8,<3.9"))
			assert.Loosely(t, spec.Dependencies, should.BeEmpty)

			// Verify proactive lockfile was NOT created
			lockPath := tomlPath + ".uv.lock"
			_, err = os.Stat(lockPath)
			assert.Loosely(t, os.IsNotExist(err), should.BeTrue)
		})
	})
}
