// Copyright 2018 The LUCI Authors.
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

package cipd

import (
	"testing"
	"time"

	"go.chromium.org/luci/common/proto/google"

	api "go.chromium.org/luci/cipd/api/cipd/v1"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestPrefixMetadataToACLs(t *testing.T) {
	t.Parallel()

	epoch := time.Date(2018, time.February, 1, 2, 3, 0, 0, time.UTC)

	Convey("Works", t, func() {
		out := prefixMetadataToACLs(&api.InheritedPrefixMetadata{
			PerPrefixMetadata: []*api.PrefixMetadata{
				{
					Prefix: "a",
					Acls: []*api.PrefixMetadata_ACL{
						{Role: api.Role_READER, Principals: []string{"group:a"}},
						{Role: api.Role_READER, Principals: []string{"group:b"}},
						{Role: api.Role_WRITER, Principals: []string{"group:b"}},
						{Role: api.Role_OWNER, Principals: []string{"group:c"}},
					},
					UpdateUser: "user:a-updater@example.com",
					UpdateTime: google.NewTimestamp(epoch),
				},
				{
					Prefix: "a/b/c",
					Acls: []*api.PrefixMetadata_ACL{
						{Role: api.Role_OWNER, Principals: []string{"group:c"}},
					},
					UpdateUser: "user:c-updater@example.com",
					UpdateTime: google.NewTimestamp(epoch),
				},
			},
		})

		So(out, ShouldResemble, []PackageACL{
			{
				PackagePath: "a",
				Role:        "READER",
				Principals:  []string{"group:a", "group:b"}, // merged into one PackageACL
				ModifiedBy:  "user:a-updater@example.com",
				ModifiedTs:  UnixTime(epoch),
			},
			{
				PackagePath: "a",
				Role:        "WRITER",
				Principals:  []string{"group:b"},
				ModifiedBy:  "user:a-updater@example.com",
				ModifiedTs:  UnixTime(epoch),
			},
			{
				PackagePath: "a",
				Role:        "OWNER",
				Principals:  []string{"group:c"},
				ModifiedBy:  "user:a-updater@example.com",
				ModifiedTs:  UnixTime(epoch),
			},
			{
				PackagePath: "a/b/c",
				Role:        "OWNER",
				Principals:  []string{"group:c"},
				ModifiedBy:  "user:c-updater@example.com",
				ModifiedTs:  UnixTime(epoch),
			},
		})
	})
}

func TestMutateACLs(t *testing.T) {
	t.Parallel()

	original := func() api.PrefixMetadata {
		return api.PrefixMetadata{
			Acls: []*api.PrefixMetadata_ACL{
				{Role: api.Role_READER, Principals: []string{"group:a"}},
			},
		}
	}

	Convey("Works", t, func() {
		meta := original()
		dirty, err := mutateACLs(&meta, []PackageACLChange{
			{Action: GrantRole, Role: "OWNER", Principal: "group:b"},
			{Action: RevokeRole, Role: "READER", Principal: "group:a"},
		})
		So(err, ShouldBeNil)
		So(dirty, ShouldBeTrue)
		So(meta, ShouldResemble, api.PrefixMetadata{
			Acls: []*api.PrefixMetadata_ACL{
				{Role: api.Role_OWNER, Principals: []string{"group:b"}},
			},
		})
	})

	Convey("Noop change", t, func() {
		meta := original()
		dirty, err := mutateACLs(&meta, []PackageACLChange{
			{Action: GrantRole, Role: "READER", Principal: "group:a"},
		})
		So(err, ShouldBeNil)
		So(dirty, ShouldBeFalse)
		So(meta, ShouldResemble, original())
	})

	Convey("Bad role", t, func() {
		meta := original()
		dirty, err := mutateACLs(&meta, []PackageACLChange{
			{Action: GrantRole, Role: "OWNER", Principal: "group:a"},
			{Action: GrantRole, Role: "ZZZ", Principal: "group:a"},
		})
		So(err, ShouldErrLike, `unrecognized role "ZZZ"`)
		So(dirty, ShouldBeTrue) // granted OWNER before failing
	})

	Convey("Bad action", t, func() {
		meta := original()
		dirty, err := mutateACLs(&meta, []PackageACLChange{
			{Action: GrantRole, Role: "OWNER", Principal: "group:a"},
			{Action: "ZZZ", Role: "WRITER", Principal: "group:a"},
		})
		So(err, ShouldErrLike, `unrecognized PackageACLChangeAction "ZZZ"`)
		So(dirty, ShouldBeTrue) // granted OWNER before failing
	})
}

func TestGrantRevokeRole(t *testing.T) {
	t.Parallel()

	Convey("Grant role", t, func() {
		m := &api.PrefixMetadata{}

		So(grantRole(m, api.Role_READER, "group:a"), ShouldBeTrue)
		So(grantRole(m, api.Role_READER, "group:b"), ShouldBeTrue)
		So(grantRole(m, api.Role_READER, "group:a"), ShouldBeFalse)
		So(grantRole(m, api.Role_WRITER, "group:a"), ShouldBeTrue)

		So(m, ShouldResemble, &api.PrefixMetadata{
			Acls: []*api.PrefixMetadata_ACL{
				{Role: api.Role_READER, Principals: []string{"group:a", "group:b"}},
				{Role: api.Role_WRITER, Principals: []string{"group:a"}},
			},
		})
	})

	Convey("Revoke role", t, func() {
		m := &api.PrefixMetadata{
			Acls: []*api.PrefixMetadata_ACL{
				{Role: api.Role_READER, Principals: []string{"group:a", "group:b"}},
				{Role: api.Role_WRITER, Principals: []string{"group:a"}},
			},
		}

		So(revokeRole(m, api.Role_READER, "group:a"), ShouldBeTrue)
		So(revokeRole(m, api.Role_READER, "group:b"), ShouldBeTrue)
		So(revokeRole(m, api.Role_READER, "group:a"), ShouldBeFalse)
		So(revokeRole(m, api.Role_WRITER, "group:a"), ShouldBeTrue)

		So(m, ShouldResemble, &api.PrefixMetadata{})
	})
}
