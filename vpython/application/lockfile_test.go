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
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	"go.chromium.org/luci/vpython/standard"
)

func TestResolveTargetPythonVersion(t *testing.T) {
	t.Parallel()

	ftt.Run("Test resolveTargetPythonVersion", t, func(t *ftt.Test) {
		ver, err := resolveTargetPythonVersion(">=3.8,<3.9")
		assert.NoErr(t, err)
		assert.Loosely(t, ver, should.Equal("3.8"))

		ver, err = resolveTargetPythonVersion(">=3.11,<3.12")
		assert.NoErr(t, err)
		assert.Loosely(t, ver, should.Equal("3.11"))

		ver, err = resolveTargetPythonVersion(">=3.8")
		assert.NoErr(t, err)
		assert.Loosely(t, ver, should.Equal("3.11"))

		ver, err = resolveTargetPythonVersion("")
		assert.NoErr(t, err)
		assert.Loosely(t, ver, should.Equal("3.11"))

		_, err = resolveTargetPythonVersion(">=2.7,<2.8")
		assert.Loosely(t, err, should.NotBeNil)

		_, err = resolveTargetPythonVersion(">=3.12")
		assert.Loosely(t, err, should.NotBeNil)
	})
}

func TestIsStandardSpecTOML(t *testing.T) {
	t.Parallel()

	ftt.Run("Test isStandardSpecTOML", t, func(t *ftt.Test) {
		assert.Loosely(t, isStandardSpecTOML("vpython.toml"), should.BeTrue)
		assert.Loosely(t, isStandardSpecTOML("standalone.vpython.toml"), should.BeTrue)
		assert.Loosely(t, isStandardSpecTOML("pyproject.toml"), should.BeFalse)
	})
}

func TestCompileLockfileBoundsPythonVersion(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX shell mock test")
	}

	ctx := context.Background()

	ftt.Run("Test compileLockfile passes --python-version and bounded [tool.uv] environments", t, func(t *ftt.Test) {
		tempDir := t.TempDir()

		argsLog := filepath.Join(tempDir, "uv_args.txt")
		envLog := filepath.Join(tempDir, "uv_env.txt")
		pyprojectLog := filepath.Join(tempDir, "captured_pyproject.toml")
		mockUV := filepath.Join(tempDir, "mock_uv.sh")
		mockScript := "#!/bin/sh\n" +
			"printf '%s\\n' \"$@\" > \"" + argsLog + "\"\n" +
			"printf 'UV_PYTHON=%s\\n' \"$UV_PYTHON\" > \"" + envLog + "\"\n" +
			"while [ $# -gt 0 ]; do\n" +
			"  if [ \"$1\" = \"--project\" ]; then\n" +
			"    cp \"$2/pyproject.toml\" \"" + pyprojectLog + "\"\n" +
			"    break\n" +
			"  fi\n" +
			"  shift\n" +
			"done\n" +
			"echo 'ruff==0.15.4 --hash=sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef'\n"
		assert.NoErr(t, os.WriteFile(mockUV, []byte(mockScript), 0755))

		t.Run("Bounds Python 3.11 spec to python_version == '3.11'", func(t *ftt.Test) {
			specPath := filepath.Join(tempDir, "vpython.toml")
			assert.NoErr(t, os.WriteFile(specPath, []byte("requires-python = '>=3.11,<3.12'\ndependencies = ['ruff==0.15.4']\n"), 0644))

			t.Setenv("UV_PYTHON", "/usr/bin/python3.13")

			spec := &standard.ProjectSpec{
				RequiresPython: ">=3.11,<3.12",
				Dependencies:   []string{"ruff==0.15.4"},
			}

			err := SyncLockfile(ctx, specPath, mockUV, "", false, spec)
			assert.NoErr(t, err)

			argsData, err := os.ReadFile(argsLog)
			assert.NoErr(t, err)
			assert.Loosely(t, string(argsData), should.ContainSubstring("--python-version\n3.11"))
			assert.Loosely(t, string(argsData), should.ContainSubstring("--project\n"))

			envData, err := os.ReadFile(envLog)
			assert.NoErr(t, err)
			assert.Loosely(t, string(envData), should.ContainSubstring("UV_PYTHON=\n"))

			pyprojectData, err := os.ReadFile(pyprojectLog)
			assert.NoErr(t, err)
			assert.Loosely(t, string(pyprojectData), should.ContainSubstring(`requires-python = "==3.11.*"`))
			assert.Loosely(t, string(pyprojectData), should.ContainSubstring(`environments = ["python_version == '3.11'"]`))
		})

		t.Run("Bounds Python 3.8 partner spec during UpgradeSpecs", func(t *ftt.Test) {
			specPath := filepath.Join(tempDir, "legacy38.vpython.toml")
			assert.NoErr(t, os.WriteFile(specPath, []byte("requires-python = '>=3.8,<3.9'\ndependencies = ['ruff==0.15.4']\n"), 0644))

			t.Setenv("VPYTHON_UV_BIN", mockUV)
			err := UpgradeSpecs(ctx, specPath, false, false, true)
			assert.NoErr(t, err)

			argsData, err := os.ReadFile(argsLog)
			assert.NoErr(t, err)
			assert.Loosely(t, string(argsData), should.ContainSubstring("--python-version\n3.8"))

			pyprojectData, err := os.ReadFile(pyprojectLog)
			assert.NoErr(t, err)
			assert.Loosely(t, string(pyprojectData), should.ContainSubstring(`requires-python = "==3.8.*"`))
			assert.Loosely(t, string(pyprojectData), should.ContainSubstring(`environments = ["python_version == '3.8'"]`))
		})
	})
}

func BenchmarkSyncLockfileCheck(b *testing.B) {
	ctx := context.Background()
	tempDir := b.TempDir()

	specPath := filepath.Join(tempDir, "vpython.toml")
	lockPath := specPath + ".uv.lock"

	// Create dummy vpython.toml
	os.WriteFile(specPath, []byte(""), 0644)

	// Create a simulated lockfile
	lockContent := `
numpy==1.2.3 \
    --hash=sha256:0c542922586a265e699188e52d5f5ac5ec0dd517e5a1041d90d2bbf23f906058 \
    --hash=sha256:57439f482b36d91b4d0e719f2982abe9da94540a3b3d05c5c1a5e43c7b315c8e
    # via mozlog
requests==2.22.0 \
    --hash=sha256:0c542922586a265e699188e52d5f5ac5ec0dd517e5a1041d90d2bbf23f906058
wheel==0.36.2 \
    --hash=sha256:57439f482b36d91b4d0e719f2982abe9da94540a3b3d05c5c1a5e43c7b315c8e
urllib3==1.25.8
chardet==3.0.4
idna==2.8
certifi==2019.11.28
`
	os.WriteFile(lockPath, []byte(lockContent), 0644)

	spec := &standard.ProjectSpec{
		Dependencies: []string{
			"requests[security]>=2.0.0",
			"numpy",
			"wheel (==0.36.2) ; python_version >= '3.8'",
		},
	}

	for b.Loop() {
		err := SyncLockfile(ctx, specPath, "uv", "python", true, spec)
		if err != nil {
			b.Fatalf("SyncLockfile failed: %v", err)
		}
	}
}
