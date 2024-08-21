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

	"google.golang.org/protobuf/types/known/timestamppb"

	api "go.chromium.org/luci/cipd/api/cipd/v1"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestPrefixMetadataToACLs(t *testing.T) {
	t.Parallel()

	epoch := time.Date(2018, time.February, 1, 2, 3, 0, 0, time.UTC)

	ftt.Run("Works", t, func(t *ftt.Test) {
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
					UpdateTime: timestamppb.New(epoch),
				},
				{
					Prefix: "a/b/c",
					Acls: []*api.PrefixMetadata_ACL{
						{Role: api.Role_OWNER, Principals: []string{"group:c"}},
					},
					UpdateUser: "user:c-updater@example.com",
					UpdateTime: timestamppb.New(epoch),
				},
			},
		})

		assert.Loosely(t, out, should.Resemble([]PackageACL{
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
		}))
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

	ftt.Run("Works", t, func(t *ftt.Test) {
		meta := original()
		dirty, err := mutateACLs(&meta, []PackageACLChange{
			{Action: GrantRole, Role: "OWNER", Principal: "group:b"},
			{Action: RevokeRole, Role: "READER", Principal: "group:a"},
		})
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, dirty, should.BeTrue)
		assert.Loosely(t, meta, should.Resemble(api.PrefixMetadata{
			Acls: []*api.PrefixMetadata_ACL{
				{Role: api.Role_OWNER, Principals: []string{"group:b"}},
			},
		}))
	})

	ftt.Run("Noop change", t, func(t *ftt.Test) {
		meta := original()
		dirty, err := mutateACLs(&meta, []PackageACLChange{
			{Action: GrantRole, Role: "READER", Principal: "group:a"},
		})
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, dirty, should.BeFalse)
		assert.Loosely(t, meta, should.Resemble(original()))
	})

	ftt.Run("Bad role", t, func(t *ftt.Test) {
		meta := original()
		dirty, err := mutateACLs(&meta, []PackageACLChange{
			{Action: GrantRole, Role: "OWNER", Principal: "group:a"},
			{Action: GrantRole, Role: "ZZZ", Principal: "group:a"},
		})
		assert.Loosely(t, err, should.ErrLike(`unrecognized role "ZZZ"`))
		assert.Loosely(t, dirty, should.BeTrue) // granted OWNER before failing
	})

	ftt.Run("Bad action", t, func(t *ftt.Test) {
		meta := original()
		dirty, err := mutateACLs(&meta, []PackageACLChange{
			{Action: GrantRole, Role: "OWNER", Principal: "group:a"},
			{Action: "ZZZ", Role: "WRITER", Principal: "group:a"},
		})
		assert.Loosely(t, err, should.ErrLike(`unrecognized PackageACLChangeAction "ZZZ"`))
		assert.Loosely(t, dirty, should.BeTrue) // granted OWNER before failing
	})
}

func TestGrantRevokeRole(t *testing.T) {
	t.Parallel()

	ftt.Run("Grant role", t, func(t *ftt.Test) {
		m := &api.PrefixMetadata{}

		assert.Loosely(t, grantRole(m, api.Role_READER, "group:a"), should.BeTrue)
		assert.Loosely(t, grantRole(m, api.Role_READER, "group:b"), should.BeTrue)
		assert.Loosely(t, grantRole(m, api.Role_READER, "group:a"), should.BeFalse)
		assert.Loosely(t, grantRole(m, api.Role_WRITER, "group:a"), should.BeTrue)

		assert.Loosely(t, m, should.Resemble(&api.PrefixMetadata{
			Acls: []*api.PrefixMetadata_ACL{
				{Role: api.Role_READER, Principals: []string{"group:a", "group:b"}},
				{Role: api.Role_WRITER, Principals: []string{"group:a"}},
			},
		}))
	})

	ftt.Run("Revoke role", t, func(t *ftt.Test) {
		m := &api.PrefixMetadata{
			Acls: []*api.PrefixMetadata_ACL{
				{Role: api.Role_READER, Principals: []string{"group:a", "group:b"}},
				{Role: api.Role_WRITER, Principals: []string{"group:a"}},
			},
		}

		assert.Loosely(t, revokeRole(m, api.Role_READER, "group:a"), should.BeTrue)
		assert.Loosely(t, revokeRole(m, api.Role_READER, "group:b"), should.BeTrue)
		assert.Loosely(t, revokeRole(m, api.Role_READER, "group:a"), should.BeFalse)
		assert.Loosely(t, revokeRole(m, api.Role_WRITER, "group:a"), should.BeTrue)

		assert.Loosely(t, m, should.Resemble(&api.PrefixMetadata{}))
	})
}
