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

package repo

import (
	"context"
	"fmt"
	"testing"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"

	api "go.chromium.org/luci/cipd/api/cipd/v1"

	. "github.com/smartystreets/goconvey/convey"
)

func TestRoles(t *testing.T) {
	t.Parallel()

	Convey("Works", t, func() {
		fakeDB := authtest.NewFakeDB(
			authtest.MockMembership("user:admin@example.com", "admins"),

			authtest.MockMembership("user:top-owner@example.com", "top-owners"),
			authtest.MockMembership("user:top-writer@example.com", "top-writers"),
			authtest.MockMembership("user:top-reader@example.com", "top-readers"),

			authtest.MockMembership("user:inner-owner@example.com", "inner-owners"),
			authtest.MockMembership("user:inner-writer@example.com", "inner-writers"),
			authtest.MockMembership("user:inner-reader@example.com", "inner-readers"),
		)

		metas := []*api.PrefixMetadata{}
		metas = addPrefixACLs(metas, "", map[api.Role][]string{
			api.Role_OWNER: {"group:admins"},
		})
		metas = addPrefixACLs(metas, "top", map[api.Role][]string{
			api.Role_OWNER:  {"user:direct-owner@example.com", "group:top-owners"},
			api.Role_WRITER: {"group:top-writers"},
			api.Role_READER: {"group:top-readers"},
		})
		metas = addPrefixACLs(metas, "top/something/else", map[api.Role][]string{
			api.Role_OWNER:  {"group:inner-owners"},
			api.Role_WRITER: {"group:inner-writers"},
			api.Role_READER: {"group:inner-readers"},
		})

		allRoles := []api.Role{api.Role_READER, api.Role_WRITER, api.Role_OWNER}
		writerRoles := []api.Role{api.Role_READER, api.Role_WRITER}
		readerRoles := []api.Role{api.Role_READER}
		noRoles := []api.Role{}

		expectedRoles := []struct {
			user          identity.Identity
			expectedRoles []api.Role
		}{
			{"user:admin@example.com", allRoles},
			{"user:direct-owner@example.com", allRoles},
			{"user:top-owner@example.com", allRoles},
			{"user:inner-owner@example.com", allRoles},

			{"user:top-writer@example.com", writerRoles},
			{"user:inner-writer@example.com", writerRoles},

			{"user:top-reader@example.com", readerRoles},
			{"user:inner-reader@example.com", readerRoles},

			{"user:someone-else@example.com", noRoles},
			{"anonymous:anonymous", noRoles},
		}

		for _, tc := range expectedRoles {
			Convey(fmt.Sprintf("User %s roles", tc.user), func() {
				ctx := auth.WithState(context.Background(), &authtest.FakeState{
					Identity: tc.user,
					FakeDB:   fakeDB,
				})

				// Get the roles by checking explicitly each one via hasRole.
				Convey("hasRole works", func() {
					haveRoles := []api.Role{}
					for _, r := range allRoles {
						yes, err := hasRole(ctx, metas, r)
						So(err, ShouldBeNil)
						if yes {
							haveRoles = append(haveRoles, r)
						}
					}
					So(haveRoles, ShouldResemble, tc.expectedRoles)
				})

				// Get the same set of roles through rolesInPrefix.
				Convey("rolesInPrefix", func() {
					haveRoles, err := rolesInPrefix(ctx, tc.user, metas)
					So(err, ShouldBeNil)
					So(haveRoles, ShouldResemble, tc.expectedRoles)
				})
			})
		}
	})
}

func addPrefixACLs(metas []*api.PrefixMetadata, prefix string, acls map[api.Role][]string) []*api.PrefixMetadata {
	a := make([]*api.PrefixMetadata_ACL, 0, len(acls))
	for role, principals := range acls {
		a = append(a, &api.PrefixMetadata_ACL{
			Role:       role,
			Principals: principals,
		})
	}
	return append(metas, &api.PrefixMetadata{
		Prefix: prefix,
		Acls:   a,
	})
}
