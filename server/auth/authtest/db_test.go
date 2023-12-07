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

package authtest

import (
	"context"
	"errors"
	"net"
	"sort"
	"testing"

	"go.chromium.org/luci/auth/identity"

	"go.chromium.org/luci/server/auth/realms"
	"go.chromium.org/luci/server/auth/service/protocol"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

var (
	testPerm1 = realms.RegisterPermission("testing.tests.perm1")
	testPerm2 = realms.RegisterPermission("testing.tests.perm2")
	testPerm3 = realms.RegisterPermission("testing.tests.perm3")
)

func init() {
	testPerm1.AddFlags(realms.UsedInQueryRealms)
}

func TestFakeDB(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	dataRoot := &protocol.RealmData{EnforceInService: []string{"A"}}
	dataSome := &protocol.RealmData{EnforceInService: []string{"B"}}

	Convey("With FakeDB", t, func() {
		db := NewFakeDB(
			MockMembership("user:abc@def.com", "group-a"),
			MockMembership("user:abc@def.com", "group-b"),

			MockGroup("group-c", []identity.Identity{"user:abc@def.com"}),
			MockGroup("group-d", nil),

			MockPermission("user:abc@def.com", "proj:realm", testPerm1),
			MockPermission("user:abc@def.com", "another:realm", testPerm1),
			MockPermission("user:abc@def.com", "proj:realm", testPerm2),

			MockPermission("user:abc@def.com", "proj:cond", testPerm1,
				RestrictAttribute("attr1", "val1", "val2")),
			MockPermission("user:abc@def.com", "proj:cond", testPerm2,
				RestrictAttribute("attr1", "val1", "val2"),
				RestrictAttribute("attr2", "val3")),

			MockRealmData("proj:@root", dataRoot),
			MockRealmData("proj:some", dataSome),

			MockIPAllowlist("127.0.0.42", "allowlist"),
		)

		Convey("Membership checks work", func() {
			out, err := db.CheckMembership(ctx, "user:abc@def.com", []string{"group-a", "group-b", "group-c", "group-d"})
			So(err, ShouldBeNil)
			So(out, ShouldResemble, []string{"group-a", "group-b", "group-c"})

			resp, err := db.IsMember(ctx, "user:abc@def.com", nil)
			So(err, ShouldBeNil)
			So(resp, ShouldBeFalse)

			resp, err = db.IsMember(ctx, "user:abc@def.com", []string{"group-b"})
			So(err, ShouldBeNil)
			So(resp, ShouldBeTrue)

			resp, err = db.IsMember(ctx, "user:abc@def.com", []string{"group-c"})
			So(err, ShouldBeNil)
			So(resp, ShouldBeTrue)

			resp, err = db.IsMember(ctx, "user:abc@def.com", []string{"group-d"})
			So(err, ShouldBeNil)
			So(resp, ShouldBeFalse)

			resp, err = db.IsMember(ctx, "user:abc@def.com", []string{"another", "group-b"})
			So(err, ShouldBeNil)
			So(resp, ShouldBeTrue)

			resp, err = db.IsMember(ctx, "user:another@def.com", []string{"group-b"})
			So(err, ShouldBeNil)
			So(resp, ShouldBeFalse)

			resp, err = db.IsMember(ctx, "user:another@def.com", []string{"another", "group-b"})
			So(err, ShouldBeNil)
			So(resp, ShouldBeFalse)

			resp, err = db.IsMember(ctx, "user:abc@def.com", []string{"another"})
			So(err, ShouldBeNil)
			So(resp, ShouldBeFalse)
		})

		Convey("Permission checks work", func() {
			resp, err := db.HasPermission(ctx, "user:abc@def.com", testPerm1, "proj:realm", nil)
			So(err, ShouldBeNil)
			So(resp, ShouldBeTrue)

			resp, err = db.HasPermission(ctx, "user:abc@def.com", testPerm2, "proj:realm", nil)
			So(err, ShouldBeNil)
			So(resp, ShouldBeTrue)

			resp, err = db.HasPermission(ctx, "user:abc@def.com", testPerm3, "proj:realm", nil)
			So(err, ShouldBeNil)
			So(resp, ShouldBeFalse)

			resp, err = db.HasPermission(ctx, "user:abc@def.com", testPerm1, "proj:unknown", nil)
			So(err, ShouldBeNil)
			So(resp, ShouldBeFalse)
		})

		Convey("Conditional permission checks work", func() {
			resp, err := db.HasPermission(ctx, "user:abc@def.com", testPerm1, "proj:cond", nil)
			So(err, ShouldBeNil)
			So(resp, ShouldBeFalse)

			resp, err = db.HasPermission(ctx, "user:abc@def.com", testPerm1, "proj:cond", realms.Attrs{
				"attr1": "val1",
			})
			So(err, ShouldBeNil)
			So(resp, ShouldBeTrue)

			resp, err = db.HasPermission(ctx, "user:abc@def.com", testPerm1, "proj:cond", realms.Attrs{
				"attr1": "val2",
			})
			So(err, ShouldBeNil)
			So(resp, ShouldBeTrue)

			resp, err = db.HasPermission(ctx, "user:abc@def.com", testPerm1, "proj:cond", realms.Attrs{
				"attr1": "val3",
			})
			So(err, ShouldBeNil)
			So(resp, ShouldBeFalse)

			resp, err = db.HasPermission(ctx, "user:abc@def.com", testPerm1, "proj:cond", realms.Attrs{
				"unknown": "val1",
			})
			So(err, ShouldBeNil)
			So(resp, ShouldBeFalse)

			resp, err = db.HasPermission(ctx, "user:abc@def.com", testPerm2, "proj:cond", realms.Attrs{
				"attr1": "val1",
			})
			So(err, ShouldBeNil)
			So(resp, ShouldBeFalse)

			resp, err = db.HasPermission(ctx, "user:abc@def.com", testPerm2, "proj:cond", realms.Attrs{
				"attr1": "val1",
				"attr2": "val3",
			})
			So(err, ShouldBeNil)
			So(resp, ShouldBeTrue)
		})

		Convey("QueryRealms works", func() {
			res, err := db.QueryRealms(ctx, "user:abc@def.com", testPerm1, "", nil)
			So(err, ShouldBeNil)
			sort.Strings(res)
			So(res, ShouldResemble, []string{"another:realm", "proj:realm"})

			res, err = db.QueryRealms(ctx, "user:abc@def.com", testPerm1, "proj", nil)
			So(err, ShouldBeNil)
			So(res, ShouldResemble, []string{"proj:realm"})

			res, err = db.QueryRealms(ctx, "user:zzz@def.com", testPerm1, "", nil)
			So(err, ShouldBeNil)
			So(res, ShouldBeEmpty)

			// Conditional bindings.
			res, err = db.QueryRealms(ctx, "user:abc@def.com", testPerm1, "", realms.Attrs{
				"attr1": "val1",
			})
			So(err, ShouldBeNil)
			sort.Strings(res)
			So(res, ShouldResemble, []string{"another:realm", "proj:cond", "proj:realm"})

			// Unflagged permission.
			_, err = db.QueryRealms(ctx, "user:abc@def.com", testPerm2, "", nil)
			So(err, ShouldErrLike, "permission testing.tests.perm2 cannot be used in QueryRealms")
		})

		Convey("FilterKnownGroups works", func() {
			known, err := db.FilterKnownGroups(ctx, []string{"missing", "group-b", "group-a", "group-c", "group-d", "group-a", "missing"})
			So(err, ShouldBeNil)
			So(known, ShouldResemble, []string{
				"group-b", "group-a", "group-c", "group-d", "group-a",
			})
		})

		Convey("GetRealmData works", func() {
			data, err := db.GetRealmData(ctx, "proj:some")
			So(err, ShouldBeNil)
			So(data, ShouldEqual, dataSome)

			// No automatic fallback to root happens, mock it yourself.
			data, err = db.GetRealmData(ctx, "proj:zzz")
			So(err, ShouldBeNil)
			So(data, ShouldBeNil)
		})

		Convey("IP allowlist checks work", func() {
			resp, err := db.IsAllowedIP(ctx, net.ParseIP("127.0.0.42"), "allowlist")
			So(err, ShouldBeNil)
			So(resp, ShouldBeTrue)

			resp, err = db.IsAllowedIP(ctx, net.ParseIP("127.0.0.42"), "another")
			So(err, ShouldBeNil)
			So(resp, ShouldBeFalse)

			resp, err = db.IsAllowedIP(ctx, net.ParseIP("192.0.0.1"), "allowlist")
			So(err, ShouldBeNil)
			So(resp, ShouldBeFalse)
		})

		Convey("Error works", func() {
			mockedErr := errors.New("boom")
			db.AddMocks(MockError(mockedErr))

			_, err := db.IsMember(ctx, "user:abc@def.com", []string{"group-a"})
			So(err, ShouldEqual, mockedErr)

			_, err = db.HasPermission(ctx, "user:abc@def.com", testPerm1, "proj:realm", nil)
			So(err, ShouldEqual, mockedErr)

			_, err = db.IsAllowedIP(ctx, net.ParseIP("127.0.0.42"), "allowlist")
			So(err, ShouldEqual, mockedErr)

			_, err = db.FilterKnownGroups(ctx, []string{"a", "b"})
			So(err, ShouldEqual, mockedErr)
		})
	})
}
