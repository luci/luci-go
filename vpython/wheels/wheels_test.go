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
