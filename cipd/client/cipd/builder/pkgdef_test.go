// Copyright 2015 The LUCI Authors.
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

package builder

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	"go.chromium.org/luci/cipd/client/cipd/fs"
)

func TestLoadPackageDef(t *testing.T) {
	t.Parallel()

	ftt.Run("LoadPackageDef empty works", t, func(t *ftt.Test) {
		body := strings.NewReader(`{"package": "package/name"}`)
		def, err := LoadPackageDef(body, nil)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, def, should.Match(PackageDef{
			Package: "package/name",
			Root:    ".",
		}))
	})

	ftt.Run("LoadPackageDef works", t, func(t *ftt.Test) {
		body := strings.NewReader(`{
			"package": "package/${var1}",
			"root": "../..",
			"install_mode": "copy",
			"data": [
				{
					"file": "some_file_${var1}"
				},
				{
					"file": "another_file_${var2}"
				},
				{
					"dir": "some/directory"
				},
				{
					"version_file": "some/path/version_${var1}.json"
				},
				{
					"dir": "another/${var2}",
					"exclude": [
						".*\\.pyc",
						"abc_${var2}_def"
					]
				}
			]
		}`)
		def, err := LoadPackageDef(body, map[string]string{
			"var1": "value1",
			"var2": "value2",
		})
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, def, should.Match(PackageDef{
			Package:     "package/value1",
			Root:        "../..",
			InstallMode: "copy",
			Data: []PackageChunkDef{
				{
					File: "some_file_value1",
				},
				{
					File: "another_file_value2",
				},
				{
					Dir: "some/directory",
				},
				{
					VersionFile: "some/path/version_value1.json",
				},
				{
					Dir: "another/value2",
					Exclude: []string{
						".*\\.pyc",
						"abc_value2_def",
					},
				},
			},
		}))
		assert.Loosely(t, def.VersionFile(), should.Equal("some/path/version_value1.json"))
	})

	ftt.Run("LoadPackageDef not yaml", t, func(t *ftt.Test) {
		body := strings.NewReader(`{ not yaml)`)
		_, err := LoadPackageDef(body, nil)
		assert.Loosely(t, err, should.NotBeNil)
	})

	ftt.Run("LoadPackageDef bad type", t, func(t *ftt.Test) {
		body := strings.NewReader(`{"package": []}`)
		_, err := LoadPackageDef(body, nil)
		assert.Loosely(t, err, should.NotBeNil)
	})

	ftt.Run("LoadPackageDef missing variable", t, func(t *ftt.Test) {
		body := strings.NewReader(`{
			"package": "abd",
			"data": [{"file": "${missing_var}"}]
		}`)
		_, err := LoadPackageDef(body, nil)
		assert.Loosely(t, err, should.NotBeNil)
	})

	ftt.Run("LoadPackageDef space in missing variable", t, func(t *ftt.Test) {
		body := strings.NewReader(`{
			"package": "abd",
			"data": [{"file": "${missing var}"}]
		}`)
		_, err := LoadPackageDef(body, nil)
		assert.Loosely(t, err, should.NotBeNil)
	})

	ftt.Run("LoadPackageDef bad package name", t, func(t *ftt.Test) {
		body := strings.NewReader(`{"package": "not a valid name"}`)
		_, err := LoadPackageDef(body, nil)
		assert.Loosely(t, err, should.NotBeNil)
	})

	ftt.Run("LoadPackageDef bad file section (no dir or file)", t, func(t *ftt.Test) {
		body := strings.NewReader(`{
			"package": "package/name",
			"data": [
				{"exclude": []}
			]
		}`)
		_, err := LoadPackageDef(body, nil)
		assert.Loosely(t, err, should.NotBeNil)
	})

	ftt.Run("LoadPackageDef bad file section (both dir and file)", t, func(t *ftt.Test) {
		body := strings.NewReader(`{
			"package": "package/name",
			"data": [
				{"file": "abc", "dir": "def"}
			]
		}`)
		_, err := LoadPackageDef(body, nil)
		assert.Loosely(t, err, should.NotBeNil)
	})

	ftt.Run("LoadPackageDef bad version_file", t, func(t *ftt.Test) {
		body := strings.NewReader(`{
			"package": "package/name",
			"data": [
				{"version_file": "../some/path.json"}
			]
		}`)
		_, err := LoadPackageDef(body, nil)
		assert.Loosely(t, err, should.NotBeNil)
	})

	ftt.Run("LoadPackageDef two version_file entries", t, func(t *ftt.Test) {
		body := strings.NewReader(`{
			"package": "package/name",
			"data": [
				{"version_file": "some/path.json"},
				{"version_file": "some/path.json"}
			]
		}`)
		_, err := LoadPackageDef(body, nil)
		assert.Loosely(t, err, should.NotBeNil)
	})
}

func TestExclusion(t *testing.T) {
	t.Parallel()

	ftt.Run("makeExclusionFilter works", t, func(t *ftt.Test) {
		filter, err := makeExclusionFilter([]string{
			".*\\.pyc",
			".*/pip-.*-build/.*",
			"bin/activate",
			"lib/.*/site-packages/.*\\.dist-info/RECORD",
		})
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, filter, should.NotBeNil)

		// *.pyc filtering.
		assert.Loosely(t, filter(filepath.FromSlash("test.pyc")), should.BeTrue)
		assert.Loosely(t, filter(filepath.FromSlash("test.py")), should.BeFalse)
		assert.Loosely(t, filter(filepath.FromSlash("d/e/f/test.pyc")), should.BeTrue)
		assert.Loosely(t, filter(filepath.FromSlash("d/e/f/test.py")), should.BeFalse)

		// Subdir filtering.
		assert.Loosely(t, filter(filepath.FromSlash("x/pip-blah-build/d/e/f")), should.BeTrue)

		// Single file exclusion.
		assert.Loosely(t, filter(filepath.FromSlash("bin/activate")), should.BeTrue)
		assert.Loosely(t, filter(filepath.FromSlash("bin/activate2")), should.BeFalse)
		assert.Loosely(t, filter(filepath.FromSlash("d/bin/activate")), should.BeFalse)

		// More complicated regexp.
		p := "lib/python2.7/site-packages/coverage-3.7.1.dist-info/RECORD"
		assert.Loosely(t, filter(filepath.FromSlash(p)), should.BeTrue)
	})

	ftt.Run("makeExclusionFilter bad regexp", t, func(t *ftt.Test) {
		_, err := makeExclusionFilter([]string{"****"})
		assert.Loosely(t, err, should.NotBeNil)
	})
}

func TestFindFiles(t *testing.T) {
	t.Parallel()

	ftt.Run("Given a temp directory", t, func(t *ftt.Test) {
		tempDir := t.TempDir()

		mkF := func(path string) { writeFile(tempDir, path, "", 0666) }
		mkD := func(path string) { mkDir(tempDir, path) }
		mkL := func(path, target string) { writeSymlink(tempDir, path, target) }

		t.Run("FindFiles works", func(t *ftt.Test) {
			mkF("ENV/abc.py")
			mkF("ENV/abc.pyc") // excluded via "exclude: '.*\.pyc'"
			mkF("ENV/abc.pyo")
			mkF("ENV/dir/def.py")
			mkD("ENV/empty")      // will be skipped
			mkF("ENV/exclude_me") // excluded via "exclude: 'exclude_me'"

			// Symlinks do not work on Windows.
			if runtime.GOOS != "windows" {
				mkL("ENV/abs_link", filepath.Dir(tempDir))
				mkL("ENV/rel_link", "abc.py")
				mkL("ENV/abs_in_root", filepath.Join(tempDir, "ENV", "dir", "def.py"))
			}

			mkF("infra/xyz.py")
			mkF("infra/zzz.pyo")
			mkF("infra/excluded.py")
			mkF("infra/excluded_dir/a")
			mkF("infra/excluded_dir/b")

			mkF("file1.py")
			mkF("dir/file2.py")

			mkF("garbage/a")
			mkF("garbage/b")

			assertFiles := func(pkgDef PackageDef, cwd string) {
				files, err := pkgDef.FindFiles(cwd)
				assert.Loosely(t, err, should.BeNil)
				names := make([]string, len(files))
				byName := make(map[string]fs.File, len(files))
				for i, f := range files {
					names[i] = f.Name()
					byName[f.Name()] = f
				}

				if runtime.GOOS == "windows" {
					assert.Loosely(t, names, should.Match([]string{
						"ENV/abc.py",
						"ENV/abc.pyo",
						"ENV/dir/def.py",
						"dir/file2.py",
						"file1.py",
						"infra/xyz.py",
					}))
				} else {
					assert.Loosely(t, names, should.Match([]string{
						"ENV/abc.py",
						"ENV/abc.pyo",
						"ENV/abs_in_root",
						"ENV/abs_link",
						"ENV/dir/def.py",
						"ENV/rel_link",
						"dir/file2.py",
						"file1.py",
						"infra/xyz.py",
					}))
					// Separately check symlinks.
					ensureSymlinkTarget(t, byName["ENV/abs_in_root"], "dir/def.py")
					ensureSymlinkTarget(t, byName["ENV/abs_link"], filepath.ToSlash(filepath.Dir(tempDir)))
					ensureSymlinkTarget(t, byName["ENV/rel_link"], "abc.py")
				}
			}

			pkgDef := PackageDef{
				Package: "test",
				Data: []PackageChunkDef{
					{
						Dir:     "ENV",
						Exclude: []string{".*\\.pyc", "exclude_me"},
					},
					{
						Dir: "infra",
						Exclude: []string{
							".*\\.pyo",
							"excluded.py",
							"excluded_dir",
						},
					},
					{File: "file1.py"},
					{File: "dir/file2.py"},
					// Will be "deduplicated", because already matched by first entry.
					{File: "ENV/abc.py"},
				},
			}

			t.Run("with relative root", func(t *ftt.Test) {
				pkgDef.Root = "../../"

				assertFiles(pkgDef, filepath.Join(tempDir, "a", "b"))
			})

			t.Run("with absolute root", func(t *ftt.Test) {
				pkgDef.Root = tempDir

				someOtherTmpDir := t.TempDir()
				assertFiles(pkgDef, someOtherTmpDir)
			})

		})

	})
}

////////////////////////////////////////////////////////////////////////////////

func mkDir(root string, path string) {
	abs := filepath.Join(root, filepath.FromSlash(path))
	err := os.MkdirAll(abs, 0777)
	if err != nil {
		panic("Failed to create a directory under temp directory")
	}
}

func writeFile(root string, path string, data string, mode os.FileMode) {
	abs := filepath.Join(root, filepath.FromSlash(path))
	os.MkdirAll(filepath.Dir(abs), 0777)
	err := os.WriteFile(abs, []byte(data), mode)
	if err != nil {
		panic("Failed to write a temp file")
	}
}

func writeSymlink(root string, path string, target string) {
	abs := filepath.Join(root, filepath.FromSlash(path))
	os.MkdirAll(filepath.Dir(abs), 0777)
	err := os.Symlink(target, abs)
	if err != nil {
		panic("Failed to create symlink")
	}
}

func ensureSymlinkTarget(t testing.TB, file fs.File, target string) {
	assert.Loosely(t, file.Symlink(), should.BeTrue)
	discoveredTarget, err := file.SymlinkTarget()
	assert.Loosely(t, err, should.BeNil)
	assert.Loosely(t, discoveredTarget, should.Equal(target))
}
