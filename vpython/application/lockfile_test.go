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

	"go.chromium.org/luci/cipd/client/cipd"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	"go.chromium.org/luci/vpython/standard"
)

func TestIsStandardSpecTOML(t *testing.T) {
	t.Parallel()

	ftt.Run("Test isStandardSpecTOML", t, func(t *ftt.Test) {
		assert.Loosely(t, isStandardSpecTOML("vpython.toml"), should.BeTrue)
		assert.Loosely(t, isStandardSpecTOML("standalone.vpython.toml"), should.BeTrue)
		assert.Loosely(t, isStandardSpecTOML("pyproject.toml"), should.BeFalse)
	})
}

func TestParseArgsSetsCIPDCacheDirEnv(t *testing.T) {
	ftt.Run("ParseArgs exports CIPD_CACHE_DIR before lockfile sync or upgrade", t, func(t *ftt.Test) {
		t.Cleanup(func() { defaultVpythonRoot.Store("") })
		tempRoot := t.TempDir()
		t.Setenv(cipd.EnvCacheDir, "")

		app := &Application{
			Arguments: []string{"-vpython-root", tempRoot, "-c", "print(1)"},
		}
		ctx := app.Initialize(context.Background())
		assert.NoErr(t, app.ParseEnvs(ctx))
		assert.NoErr(t, app.ParseArgs(ctx))

		expectedCache := filepath.Join(tempRoot, "cipd")
		assert.That(t, app.CIPDCacheDir, should.Equal(expectedCache))
		assert.That(t, os.Getenv(cipd.EnvCacheDir), should.Equal(expectedCache))
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
			"printf 'UV_PYTHON=%s\\nUV_NO_CONFIG=%s\\n' \"$UV_PYTHON\" \"$UV_NO_CONFIG\" > \"" + envLog + "\"\n" +
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
			assert.That(t, string(envData), should.ContainSubstring("UV_NO_CONFIG=1\n"))

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

		t.Run("Bounds explicit Chromium patch pin to major.minor track", func(t *ftt.Test) {
			specPath := filepath.Join(tempDir, "patch314.vpython.toml")
			assert.NoErr(t, os.WriteFile(specPath, []byte("requires-python = '==3.14.8.chromium.1'\ndependencies = ['ruff==0.15.4']\n"), 0644))

			spec := &standard.ProjectSpec{
				RequiresPython: "==3.14.8.chromium.1",
				Dependencies:   []string{"ruff==0.15.4"},
			}

			assert.NoErr(t, SyncLockfile(ctx, specPath, mockUV, "", false, spec))

			argsData, err := os.ReadFile(argsLog)
			assert.NoErr(t, err)
			assert.That(t, string(argsData), should.ContainSubstring("--python-version\n3.14"))

			pyprojectData, err := os.ReadFile(pyprojectLog)
			assert.NoErr(t, err)
			assert.That(t, string(pyprojectData), should.ContainSubstring(`requires-python = "==3.14.*"`))
			assert.That(t, string(pyprojectData), should.ContainSubstring(`environments = ["python_version == '3.14'"]`))
		})

		t.Run("Passes --python and UV_PYTHON when pythonBin is resolved", func(t *ftt.Test) {
			specPath := filepath.Join(tempDir, "with_python.vpython.toml")
			assert.NoErr(t, os.WriteFile(specPath, []byte("requires-python = '>=3.11'\ndependencies = ['ruff==0.15.4']\n"), 0644))

			spec := &standard.ProjectSpec{
				RequiresPython: ">=3.11",
				Dependencies:   []string{"ruff==0.15.4"},
			}

			assert.NoErr(t, SyncLockfile(ctx, specPath, mockUV, "/opt/cipd/bin/python3", false, spec))

			argsData, err := os.ReadFile(argsLog)
			assert.NoErr(t, err)
			assert.That(t, string(argsData), should.ContainSubstring("--python\n/opt/cipd/bin/python3"))

			envData, err := os.ReadFile(envLog)
			assert.NoErr(t, err)
			assert.That(t, string(envData), should.ContainSubstring("UV_PYTHON=/opt/cipd/bin/python3\n"))
		})
	})
}

func TestSyncLockfile_PEP508AndPEP503Normalization(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	ftt.Run("Test SyncLockfile handles PEP 508 != / parenthesized constraints and PEP 503 multi-separator package names", t, func(t *ftt.Test) {
		tempDir := t.TempDir()
		specPath := filepath.Join(tempDir, "vpython.toml")
		lockPath := specPath + ".uv.lock"

		assert.NoErr(t, os.WriteFile(specPath, []byte("requires-python = '>=3.11'\n"), 0644))

		lockContent := "" +
			"urllib3==2.0.0 --hash=sha256:0123456789abcdef\n" +
			"foo-bar==1.0.0 --hash=sha256:0123456789abcdef\n" +
			"wheel==0.36.2 --hash=sha256:0123456789abcdef\n"
		assert.NoErr(t, os.WriteFile(lockPath, []byte(lockContent), 0644))

		spec := &standard.ProjectSpec{
			RequiresPython: ">=3.11",
			Dependencies: []string{
				"urllib3!=1.25.0",
				"foo__bar>=1.0",
				"wheel(==0.36.2)",
			},
		}

		err := SyncLockfile(ctx, specPath, "uv", "", true, spec)
		assert.NoErr(t, err)
		assert.Loosely(t, len(spec.Dependencies), should.Equal(3))
	})
}

func TestSyncLockfile_VersionChange(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		lock     string
		deps     []string
		wantDeps []string
		wantErr  string
	}{
		{
			name:     "matching_pin",
			lock:     "six==1.16.0 ; python_full_version < '3.12' \\\n    --hash=sha256:0123\n",
			deps:     []string{"six==1.16.0"},
			wantDeps: []string{"six==1.16.0 ; python_full_version < '3.12'"},
		},
		{
			name:     "direct_url_and_mixed_case",
			lock:     "foo-bar==1.0\nbaz @ https://example.com/baz-1.0.whl\n",
			deps:     []string{"Foo_Bar==1.0", "baz @ https://example.com/baz-1.0.whl"},
			wantDeps: []string{"foo-bar==1.0", "baz @ https://example.com/baz-1.0.whl"},
		},
		{
			name:    "bumped_pin_out_of_sync",
			lock:    "six==1.16.0 ; python_full_version < '3.12' \\\n    --hash=sha256:0123\n",
			deps:    []string{"six==1.17.0"},
			wantErr: "missing or out-of-sync dependencies: six==1.17.0",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			specPath := filepath.Join(t.TempDir(), "vpython.toml")
			assert.NoErr(t, os.WriteFile(specPath, []byte("requires-python = '>=3.11'\n"), 0644))
			assert.NoErr(t, os.WriteFile(specPath+".uv.lock", []byte(tc.lock), 0644))

			spec := &standard.ProjectSpec{Dependencies: tc.deps}
			err := SyncLockfile(t.Context(), specPath, "uv", "", true, spec)
			if tc.wantErr != "" {
				assert.Loosely(t, err, should.ErrLike(tc.wantErr))
				return
			}
			assert.NoErr(t, err)
			assert.Loosely(t, spec.Dependencies, should.Match(tc.wantDeps))
		})
	}
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

func TestRequirementSpecifiers(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		dep  string
		want string
	}{
		{name: "bare_name", dep: "numpy", want: ""},
		{name: "extras_only", dep: "requests[security]", want: ""},
		{name: "direct_url", dep: "foo @ https://example.com/foo.whl", want: ""},
		{name: "exact_pin", dep: "six==1.17.0", want: "==1.17.0"},
		{name: "extras_with_ge", dep: "requests[security]>=2.0.0", want: ">=2.0.0"},
		{name: "parens_and_marker", dep: "wheel (==0.36.2) ; python_version >= '3.8'", want: "==0.36.2"},
		{name: "pin_with_marker", dep: "pywin32==306; sys_platform == 'win32'", want: "==306"},
		{name: "compound_range", dep: "foo >= 1.0, < 2.0", want: ">= 1.0, < 2.0"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assert.Loosely(t, requirementSpecifiers(tc.dep), should.Equal(tc.want))
		})
	}
}

func TestLockedVersions(t *testing.T) {
	t.Parallel()

	lock := `
six==1.16.0 ; python_full_version < '3.12' \
    --hash=sha256:0123 \
    # via foo
Foo_Bar==2.0 --hash=sha256:0123
numpy==1.23.5+chromium.4 ; sys_platform == 'linux'
numpy==1.21.0 ; sys_platform == 'win32'
bar[extra]==3.0
baz @ https://example.com/baz-1.0.whl?x==y
`
	assert.Loosely(t, lockedVersions([]byte(lock)), should.Match(map[string][]string{
		"six":     {"1.16.0"},
		"foo-bar": {"2.0"},
		"numpy":   {"1.23.5+chromium.4", "1.21.0"},
		"bar":     {"3.0"},
		"baz":     nil,
	}))
}

func TestLockSatisfies(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		specifiers string
		versions   []string
		want       bool
	}{
		{name: "unconstrained", specifiers: "", versions: []string{"1.0"}, want: true},
		{name: "mismatched_pin", specifiers: "==1.17.0", versions: []string{"1.16.0"}, want: false},
		{name: "multi_marker_match", specifiers: "==1.21.0", versions: []string{"1.23.5+chromium.4", "1.21.0"}, want: true},
		{name: "unparseable_specifier_fails_open", specifiers: "==not a version", versions: []string{"1.0"}, want: true},
		{name: "unversioned_lock_entry", specifiers: "==1.0", versions: nil, want: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assert.Loosely(t, lockSatisfies(tc.specifiers, tc.versions), should.Equal(tc.want))
		})
	}
}
