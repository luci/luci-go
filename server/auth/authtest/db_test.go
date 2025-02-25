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
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	"go.chromium.org/luci/server/auth/realms"
	"go.chromium.org/luci/server/auth/service/protocol"
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

	ftt.Run("With FakeDB", t, func(t *ftt.Test) {
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

		t.Run("Membership checks work", func(t *ftt.Test) {
			out, err := db.CheckMembership(ctx, "user:abc@def.com", []string{"group-a", "group-b", "group-c", "group-d"})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, out, should.Match([]string{"group-a", "group-b", "group-c"}))

			resp, err := db.IsMember(ctx, "user:abc@def.com", nil)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, resp, should.BeFalse)

			resp, err = db.IsMember(ctx, "user:abc@def.com", []string{"group-b"})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, resp, should.BeTrue)

			resp, err = db.IsMember(ctx, "user:abc@def.com", []string{"group-c"})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, resp, should.BeTrue)

			resp, err = db.IsMember(ctx, "user:abc@def.com", []string{"group-d"})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, resp, should.BeFalse)

			resp, err = db.IsMember(ctx, "user:abc@def.com", []string{"another", "group-b"})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, resp, should.BeTrue)

			resp, err = db.IsMember(ctx, "user:another@def.com", []string{"group-b"})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, resp, should.BeFalse)

			resp, err = db.IsMember(ctx, "user:another@def.com", []string{"another", "group-b"})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, resp, should.BeFalse)

			resp, err = db.IsMember(ctx, "user:abc@def.com", []string{"another"})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, resp, should.BeFalse)
		})

		t.Run("Permission checks work", func(t *ftt.Test) {
			resp, err := db.HasPermission(ctx, "user:abc@def.com", testPerm1, "proj:realm", nil)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, resp, should.BeTrue)

			resp, err = db.HasPermission(ctx, "user:abc@def.com", testPerm2, "proj:realm", nil)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, resp, should.BeTrue)

			resp, err = db.HasPermission(ctx, "user:abc@def.com", testPerm3, "proj:realm", nil)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, resp, should.BeFalse)

			resp, err = db.HasPermission(ctx, "user:abc@def.com", testPerm1, "proj:unknown", nil)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, resp, should.BeFalse)
		})

		t.Run("Conditional permission checks work", func(t *ftt.Test) {
			resp, err := db.HasPermission(ctx, "user:abc@def.com", testPerm1, "proj:cond", nil)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, resp, should.BeFalse)

			resp, err = db.HasPermission(ctx, "user:abc@def.com", testPerm1, "proj:cond", realms.Attrs{
				"attr1": "val1",
			})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, resp, should.BeTrue)

			resp, err = db.HasPermission(ctx, "user:abc@def.com", testPerm1, "proj:cond", realms.Attrs{
				"attr1": "val2",
			})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, resp, should.BeTrue)

			resp, err = db.HasPermission(ctx, "user:abc@def.com", testPerm1, "proj:cond", realms.Attrs{
				"attr1": "val3",
			})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, resp, should.BeFalse)

			resp, err = db.HasPermission(ctx, "user:abc@def.com", testPerm1, "proj:cond", realms.Attrs{
				"unknown": "val1",
			})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, resp, should.BeFalse)

			resp, err = db.HasPermission(ctx, "user:abc@def.com", testPerm2, "proj:cond", realms.Attrs{
				"attr1": "val1",
			})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, resp, should.BeFalse)

			resp, err = db.HasPermission(ctx, "user:abc@def.com", testPerm2, "proj:cond", realms.Attrs{
				"attr1": "val1",
				"attr2": "val3",
			})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, resp, should.BeTrue)
		})

		t.Run("QueryRealms works", func(t *ftt.Test) {
			res, err := db.QueryRealms(ctx, "user:abc@def.com", testPerm1, "", nil)
			assert.Loosely(t, err, should.BeNil)
			sort.Strings(res)
			assert.Loosely(t, res, should.Match([]string{"another:realm", "proj:realm"}))

			res, err = db.QueryRealms(ctx, "user:abc@def.com", testPerm1, "proj", nil)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, res, should.Match([]string{"proj:realm"}))

			res, err = db.QueryRealms(ctx, "user:zzz@def.com", testPerm1, "", nil)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, res, should.BeEmpty)

			// Conditional bindings.
			res, err = db.QueryRealms(ctx, "user:abc@def.com", testPerm1, "", realms.Attrs{
				"attr1": "val1",
			})
			assert.Loosely(t, err, should.BeNil)
			sort.Strings(res)
			assert.Loosely(t, res, should.Match([]string{"another:realm", "proj:cond", "proj:realm"}))

			// Unflagged permission.
			_, err = db.QueryRealms(ctx, "user:abc@def.com", testPerm2, "", nil)
			assert.Loosely(t, err, should.ErrLike("permission testing.tests.perm2 cannot be used in QueryRealms"))
		})

		t.Run("FilterKnownGroups works", func(t *ftt.Test) {
			known, err := db.FilterKnownGroups(ctx, []string{"missing", "group-b", "group-a", "group-c", "group-d", "group-a", "missing"})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, known, should.Match([]string{
				"group-b", "group-a", "group-c", "group-d", "group-a",
			}))
		})

		t.Run("GetRealmData works", func(t *ftt.Test) {
			data, err := db.GetRealmData(ctx, "proj:some")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, data, should.Equal(dataSome))

			// No automatic fallback to root happens, mock it yourself.
			data, err = db.GetRealmData(ctx, "proj:zzz")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, data, should.BeNil)
		})

		t.Run("IP allowlist checks work", func(t *ftt.Test) {
			resp, err := db.IsAllowedIP(ctx, net.ParseIP("127.0.0.42"), "allowlist")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, resp, should.BeTrue)

			resp, err = db.IsAllowedIP(ctx, net.ParseIP("127.0.0.42"), "another")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, resp, should.BeFalse)

			resp, err = db.IsAllowedIP(ctx, net.ParseIP("192.0.0.1"), "allowlist")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, resp, should.BeFalse)
		})

		t.Run("Error works", func(t *ftt.Test) {
			mockedErr := errors.New("boom")
			db.AddMocks(MockError(mockedErr))

			_, err := db.IsMember(ctx, "user:abc@def.com", []string{"group-a"})
			assert.Loosely(t, err, should.Equal(mockedErr))

			_, err = db.HasPermission(ctx, "user:abc@def.com", testPerm1, "proj:realm", nil)
			assert.Loosely(t, err, should.Equal(mockedErr))

			_, err = db.IsAllowedIP(ctx, net.ParseIP("127.0.0.42"), "allowlist")
			assert.Loosely(t, err, should.Equal(mockedErr))

			_, err = db.FilterKnownGroups(ctx, []string{"a", "b"})
			assert.Loosely(t, err, should.Equal(mockedErr))
		})
	})
}
