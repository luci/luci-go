// Copyright 2023 The LUCI Authors.
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

package wheels

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"go.chromium.org/luci/cipd/client/cipd/ensure"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	"go.chromium.org/luci/vpython/api/vpython"
)

func TestGeneratingEnsureFile(t *testing.T) {
	ftt.Run("Test generate ensure file", t, func(t *ftt.Test) {
		ef, err := ensureFileFromVPythonSpec(&vpython.Spec{
			Wheel: []*vpython.Spec_Package{
				{Name: "pkg1", Version: "version1"},
				{Name: "pkg2", Version: "version2"},
			},
		}, nil)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, ef.PackagesBySubdir["wheels"], should.Match(ensure.PackageSlice{
			{PackageTemplate: "pkg1", UnresolvedVersion: "version1"},
			{PackageTemplate: "pkg2", UnresolvedVersion: "version2"},
		}))
	})
	ftt.Run("Test duplicated wheels", t, func(t *ftt.Test) {
		t.Run("Same version", func(t *ftt.Test) {
			ef, err := ensureFileFromVPythonSpec(&vpython.Spec{
				Wheel: []*vpython.Spec_Package{
					{Name: "pkg1", Version: "version1"},
					{Name: "pkg1", Version: "version1"},
				},
			}, nil)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, ef.PackagesBySubdir["wheels"], should.Match(ensure.PackageSlice{
				{PackageTemplate: "pkg1", UnresolvedVersion: "version1"},
			}))
		})
		t.Run("Different version", func(t *ftt.Test) {
			_, err := ensureFileFromVPythonSpec(&vpython.Spec{
				Wheel: []*vpython.Spec_Package{
					{Name: "pkg1", Version: "version1"},
					{Name: "pkg1", Version: "version2"},
				},
			}, nil)
			assert.Loosely(t, err, should.NotBeNil)
			assert.Loosely(t, err.Error(), should.HavePrefix("multiple versions for package"))
		})
	})
	ftt.Run("Test match tag", t, func(t *ftt.Test) {
		t.Run("match", func(t *ftt.Test) {
			ef, err := ensureFileFromVPythonSpec(&vpython.Spec{
				Wheel: []*vpython.Spec_Package{
					{Name: "pkg1", Version: "version1", MatchTag: []*vpython.PEP425Tag{{Platform: "manylinux1_x86_64"}}},
					{Name: "pkg2", Version: "version2"},
				},
			}, []*vpython.PEP425Tag{{Platform: "manylinux1_x86_64"}})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, ef.PackagesBySubdir["wheels"], should.Match(ensure.PackageSlice{
				{PackageTemplate: "pkg1", UnresolvedVersion: "version1"},
				{PackageTemplate: "pkg2", UnresolvedVersion: "version2"},
			}))
		})
		t.Run("mismatch", func(t *ftt.Test) {
			ef, err := ensureFileFromVPythonSpec(&vpython.Spec{
				Wheel: []*vpython.Spec_Package{
					{Name: "pkg1", Version: "version1", MatchTag: []*vpython.PEP425Tag{{Platform: "manylinux1_x86_64"}}},
					{Name: "pkg2", Version: "version2"},
				},
			}, []*vpython.PEP425Tag{{Platform: "manylinux1_aarch64"}})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, ef.PackagesBySubdir["wheels"], should.Match(ensure.PackageSlice{
				{PackageTemplate: "pkg2", UnresolvedVersion: "version2"},
			}))
		})
	})
}

func TestPipHelpers(t *testing.T) {
	ftt.Run("Test pip helpers", t, func(t *ftt.Test) {
		t.Run("pipNameFromPackageName", func(t *ftt.Test) {
			cases := []struct {
				in, out string
			}{
				{"infra/python/wheels/six-py2_py3", "six"},
				{"infra/python/wheels/six/${platform}", "six"},
				{"infra/python/wheels/cryptography/linux-amd64", "cryptography"},
				{"infra/python/wheels/numpy-py3", "numpy"},
				{"infra/python/wheels/requests_py2_py3", "requests"},
				{"my-package", "my-package"},
				{"infra/python/wheels/my._-package", "my-package"},
				{"infra/python/wheels/complex---name", "complex-name"},
			}
			for _, c := range cases {
				assert.Loosely(t, pipNameFromPackageName(c.in), should.Equal(c.out))
			}
		})

		t.Run("pipVersionFromPackageVersion", func(t *ftt.Test) {
			cases := []struct {
				in, out string
			}{
				{"version:1.15.0", "1.15.0"},
				{"version:2@1.15.0.chromium.1", "1.15.0+chromium.1"},
				{"my-hash", "my-hash"},
				{"version:1.2.0.chromium.1", "1.2.0+chromium.1"},
				{"version:1.2.0+chromium.1", "1.2.0+chromium.1"},
				{"version:1.2.0.post1.chromium.1", "1.2.0.post1+chromium.1"},
				{"version:1.2.0.google.2", "1.2.0+google.2"},
				{"version:1.2.0-hash", "1.2.0+hash"},
				{"version:1.2.0.c1", "1.2.0.c1"},
				{"version:1.2.0.c", "1.2.0.c"},
				{"version:1.2.0.rc1", "1.2.0.rc1"},
				{"version:2.5.6-5c85ed3d46137b17da04c59bcd805ee5", "2.5.6+5c85ed3d46137b17da04c59bcd805ee5"},
				{"version:2.5.6-0a1b2c3d", "2.5.6+0a1b2c3d"},
			}
			for _, c := range cases {
				assert.Loosely(t, pipVersionFromPackageVersion(c.in), should.Equal(c.out))
			}
		})
	})
}

func TestWriteRequirementsFromSpec(t *testing.T) {
	ftt.Run("Test write requirements from spec", t, func(t *ftt.Test) {
		dir := t.TempDir()
		reqPath := filepath.Join(dir, "requirements.txt")

		s := &vpython.Spec{
			Wheel: []*vpython.Spec_Package{
				{Name: "infra/python/wheels/six-py2_py3", Version: "version:1.15.0"},
				{Name: "infra/python/wheels/pkg1", Version: "version:1.0.0", MatchTag: []*vpython.PEP425Tag{{Platform: "manylinux1_x86_64"}}},
				{Name: "infra/python/wheels/pkg2", Version: "version:2.0.0"},
			},
		}

		t.Run("Match all", func(t *ftt.Test) {
			tags := []*vpython.PEP425Tag{{Platform: "manylinux1_x86_64"}}
			err := writeRequirementsFromSpec(reqPath, s, tags)
			assert.Loosely(t, err, should.BeNil)

			content, err := os.ReadFile(reqPath)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, string(content), should.Equal("pkg1==1.0.0\npkg2==2.0.0\nsix==1.15.0\n"))
		})

		t.Run("Match subset", func(t *ftt.Test) {
			tags := []*vpython.PEP425Tag{{Platform: "macosx_10_15_intel"}}
			err := writeRequirementsFromSpec(reqPath, s, tags)
			assert.Loosely(t, err, should.BeNil)

			content, err := os.ReadFile(reqPath)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, string(content), should.Equal("pkg2==2.0.0\nsix==1.15.0\n"))
		})

		t.Run("Duplicates", func(t *ftt.Test) {
			s := &vpython.Spec{
				Wheel: []*vpython.Spec_Package{
					{Name: "infra/python/wheels/six-py2_py3", Version: "version:1.15.0"},
					{Name: "infra/python/wheels/six", Version: "version:1.15.0"},
				},
			}
			tags := []*vpython.PEP425Tag{{Platform: "manylinux1_x86_64"}}
			err := writeRequirementsFromSpec(reqPath, s, tags)
			assert.Loosely(t, err, should.BeNil)

			content, err := os.ReadFile(reqPath)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, string(content), should.Equal("six==1.15.0\n"))
		})
	})
}

func TestWriteMappingFile(t *testing.T) {
	ftt.Run("Test write mapping file", t, func(t *ftt.Test) {
		dir := t.TempDir()
		mappingPath := filepath.Join(dir, "mapping.json")

		ef := &ensure.File{
			PackagesBySubdir: map[string]ensure.PackageSlice{
				"wheels": {
					{PackageTemplate: "infra/python/wheels/six-py2_py3", UnresolvedVersion: "version:1.15.0"},
					{PackageTemplate: "infra/python/wheels/pkg1", UnresolvedVersion: "version:1.0.0"},
					{PackageTemplate: "infra/python/wheels/pkg2", UnresolvedVersion: "version:2.0.0"},
				},
			},
		}

		err := writeMappingFile(mappingPath, ef)
		assert.Loosely(t, err, should.BeNil)

		content, err := os.ReadFile(mappingPath)
		assert.Loosely(t, err, should.BeNil)

		var mapping map[string]string
		err = json.Unmarshal(content, &mapping)
		assert.Loosely(t, err, should.BeNil)

		assert.Loosely(t, mapping["six"], should.Equal("infra/python/wheels/six-py2_py3"))
		assert.Loosely(t, mapping["pkg1"], should.Equal("infra/python/wheels/pkg1"))
		assert.Loosely(t, mapping["pkg2"], should.Equal("infra/python/wheels/pkg2"))
	})
}
