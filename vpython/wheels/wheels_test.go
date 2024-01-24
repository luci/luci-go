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

	"go.chromium.org/luci/vpython/api/vpython"

	. "github.com/smartystreets/goconvey/convey"
)

func TestGeneratingEnsureFile(t *testing.T) {
	Convey("Test generate ensure file", t, func() {
		ef, err := ensureFileFromVPythonSpec(&vpython.Spec{
			Wheel: []*vpython.Spec_Package{
				{Name: "pkg1", Version: "version1"},
				{Name: "pkg2", Version: "version2"},
			},
		}, nil)
		So(err, ShouldBeNil)
		So(ef.PackagesBySubdir["wheels"], ShouldResemble, ensure.PackageSlice{
			{PackageTemplate: "pkg1", UnresolvedVersion: "version1"},
			{PackageTemplate: "pkg2", UnresolvedVersion: "version2"},
		})

	})
	Convey("Test duplicated wheels", t, func() {
		Convey("Same version", func() {
			ef, err := ensureFileFromVPythonSpec(&vpython.Spec{
				Wheel: []*vpython.Spec_Package{
					{Name: "pkg1", Version: "version1"},
					{Name: "pkg1", Version: "version1"},
				},
			}, nil)
			So(err, ShouldBeNil)
			So(ef.PackagesBySubdir["wheels"], ShouldResemble, ensure.PackageSlice{
				{PackageTemplate: "pkg1", UnresolvedVersion: "version1"},
			})
		})
		Convey("Different version", func() {
			_, err := ensureFileFromVPythonSpec(&vpython.Spec{
				Wheel: []*vpython.Spec_Package{
					{Name: "pkg1", Version: "version1"},
					{Name: "pkg1", Version: "version2"},
				},
			}, nil)
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldStartWith, "multiple versions for package")
		})
	})
	Convey("Test match tag", t, func() {
		Convey("match", func() {
			ef, err := ensureFileFromVPythonSpec(&vpython.Spec{
				Wheel: []*vpython.Spec_Package{
					{Name: "pkg1", Version: "version1", MatchTag: []*vpython.PEP425Tag{{Platform: "manylinux1_x86_64"}}},
					{Name: "pkg2", Version: "version2"},
				},
			}, []*vpython.PEP425Tag{{Platform: "manylinux1_x86_64"}})
			So(err, ShouldBeNil)
			So(ef.PackagesBySubdir["wheels"], ShouldResemble, ensure.PackageSlice{
				{PackageTemplate: "pkg1", UnresolvedVersion: "version1"},
				{PackageTemplate: "pkg2", UnresolvedVersion: "version2"},
			})
		})
		Convey("mismatch", func() {
			ef, err := ensureFileFromVPythonSpec(&vpython.Spec{
				Wheel: []*vpython.Spec_Package{
					{Name: "pkg1", Version: "version1", MatchTag: []*vpython.PEP425Tag{{Platform: "manylinux1_x86_64"}}},
					{Name: "pkg2", Version: "version2"},
				},
			}, []*vpython.PEP425Tag{{Platform: "manylinux1_aarch64"}})
			So(err, ShouldBeNil)
			So(ef.PackagesBySubdir["wheels"], ShouldResemble, ensure.PackageSlice{
				{PackageTemplate: "pkg2", UnresolvedVersion: "version2"},
			})
		})
	})
}
