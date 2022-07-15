// Copyright 2020 The LUCI Authors.
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

package realms

import (
	"fmt"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestValidatePermissionName(t *testing.T) {
	t.Parallel()

	Convey("Works", t, func() {
		So(ValidatePermissionName("service.subject.verb"), ShouldBeNil)

		So(ValidatePermissionName("service.subject.verb.stuff"), ShouldNotBeNil)
		So(ValidatePermissionName("service.subject"), ShouldNotBeNil)
		So(ValidatePermissionName("service.subject."), ShouldNotBeNil)
		So(ValidatePermissionName("service..verb"), ShouldNotBeNil)
		So(ValidatePermissionName(".subject.verb"), ShouldNotBeNil)
		So(ValidatePermissionName(""), ShouldNotBeNil)
	})
}

func TestRegister(t *testing.T) {
	// This test interacts with the global `perms` cache.
	// t.Parallel()

	Convey("TestRegister", t, func() {
		// Make sure the test succeeds when using `go test . -count=2`.
		clearPermissions()

		Convey("Works", func() {
			p1 := RegisterPermission("luci.dev.testing1")
			So(p1.Name(), ShouldEqual, "luci.dev.testing1")
			So(p1.String(), ShouldEqual, "luci.dev.testing1")
			So(fmt.Sprintf("%q", p1), ShouldEqual, `"luci.dev.testing1"`)

			So(RegisteredPermissions(), ShouldResemble, map[Permission]PermissionFlags{p1: 0})

			p2 := RegisterPermission("luci.dev.testing2")
			p2.AddFlags(UsedInQueryRealms)
			So(RegisteredPermissions(), ShouldResemble, map[Permission]PermissionFlags{
				p1: 0,
				p2: UsedInQueryRealms,
			})

			// Reregistering doesn't clear the flags.
			RegisterPermission("luci.dev.testing2")
			So(RegisteredPermissions(), ShouldResemble, map[Permission]PermissionFlags{
				p1: 0,
				p2: UsedInQueryRealms,
			})
		})

		Convey("Panics on bad name", func() {
			So(func() { RegisterPermission(".bad.name") }, ShouldPanic)
		})
	})
}

func TestGetPermissions(t *testing.T) {
	// This test interacts with the global `perms` cache.
	// t.Parallel()

	Convey("TestGetPermissions", t, func() {
		clearPermissions()
		RegisterPermission("luci.dev.testing1")
		RegisterPermission("luci.dev.testing2")

		Convey("Get single permission works", func() {
			perms, err := GetPermissions("luci.dev.testing1")
			So(err, ShouldBeNil)
			So(perms, ShouldHaveLength, 1)
			So(perms[0].Name(), ShouldEqual, "luci.dev.testing1")
		})

		Convey("Get multiple permissions works", func() {
			perms, err := GetPermissions("luci.dev.testing1", "luci.dev.testing2")
			So(err, ShouldBeNil)
			So(perms, ShouldHaveLength, 2)
			So(perms[0].Name(), ShouldEqual, "luci.dev.testing1")
			So(perms[1].Name(), ShouldEqual, "luci.dev.testing2")

			// Get in a different order.
			perms, err = GetPermissions("luci.dev.testing2", "luci.dev.testing1")
			So(err, ShouldBeNil)
			So(perms, ShouldHaveLength, 2)
			So(perms[0].Name(), ShouldEqual, "luci.dev.testing2")
			So(perms[1].Name(), ShouldEqual, "luci.dev.testing1")

			// Get duplicates.
			perms, err = GetPermissions("luci.dev.testing1", "luci.dev.testing1")
			So(err, ShouldBeNil)
			So(perms, ShouldHaveLength, 2)
			So(perms[0].Name(), ShouldEqual, "luci.dev.testing1")
			So(perms[1].Name(), ShouldEqual, "luci.dev.testing1")
		})

		Convey("Get unregistered permission returns error", func() {
			perms, err := GetPermissions("luci.dev.unregistered")
			So(err, ShouldErrLike, "permission not registered", "luci.dev.unregistered")
			So(perms, ShouldBeNil)

			// Mixed with registered permission.
			perms, err = GetPermissions("luci.dev.testing1", "luci.dev.unregistered")
			So(err, ShouldErrLike, "permission not registered", "luci.dev.unregistered")
			So(perms, ShouldBeNil)
		})
	})
}
