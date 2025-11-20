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
	"regexp"
	"testing"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"

	repopb "go.chromium.org/luci/cipd/api/cipd/v1/repopb"
	configpb "go.chromium.org/luci/cipd/api/config/v1"
	"go.chromium.org/luci/cipd/appengine/impl/prefixcfg"
)

func TestRoles(t *testing.T) {
	t.Parallel()

	ftt.Run("Works", t, func(t *ftt.Test) {
		fakeDB := authtest.NewFakeDB(
			authtest.MockMembership("user:admin@google.com", "admins"),

			authtest.MockMembership("user:top-owner@google.com", "top-owners"),
			authtest.MockMembership("user:top-writer@google.com", "top-writers"),
			authtest.MockMembership("user:top-reader@example.com", "top-readers"),

			authtest.MockMembership("user:inner-owner@google.com", "inner-owners"),
			authtest.MockMembership("user:inner-writer@google.com", "inner-writers"),
			authtest.MockMembership("user:inner-reader@example.com", "inner-readers"),

			authtest.MockMembership("user:top-owner@example.com", "top-owners"),
			authtest.MockMembership("user:top-writer@example.com", "top-writers"),
		)

		metas := []*repopb.PrefixMetadata{}
		metas = addPrefixACLs(metas, "", map[repopb.Role][]string{
			repopb.Role_OWNER: {"group:admins"},
		})
		metas = addPrefixACLs(metas, "top", map[repopb.Role][]string{
			repopb.Role_OWNER:  {"user:direct-owner@google.com", "group:top-owners"},
			repopb.Role_WRITER: {"group:top-writers"},
			repopb.Role_READER: {"group:top-readers"},
		})
		metas = addPrefixACLs(metas, "top/something/else", map[repopb.Role][]string{
			repopb.Role_OWNER:  {"group:inner-owners"},
			repopb.Role_WRITER: {"group:inner-writers"},
			repopb.Role_READER: {"group:inner-readers"},
		})

		allRoles := []repopb.Role{repopb.Role_READER, repopb.Role_WRITER, repopb.Role_OWNER}
		writerRoles := []repopb.Role{repopb.Role_READER, repopb.Role_WRITER}
		readerRoles := []repopb.Role{repopb.Role_READER}
		noRoles := []repopb.Role{}

		expectedRoles := []struct {
			user          identity.Identity
			expectedRoles []repopb.Role
		}{
			{"user:admin@google.com", allRoles},
			{"user:direct-owner@google.com", allRoles},
			{"user:top-owner@google.com", allRoles},
			{"user:inner-owner@google.com", allRoles},

			{"user:top-writer@google.com", writerRoles},
			{"user:inner-writer@google.com", writerRoles},

			{"user:top-reader@example.com", readerRoles},
			{"user:inner-reader@example.com", readerRoles},

			{"user:someone-else@google.com", noRoles},
			{"anonymous:anonymous", noRoles},

			{"user:top-owner@example.com", readerRoles},
			{"user:top-writer@example.com", readerRoles},
		}

		for _, tc := range expectedRoles {
			t.Run(fmt.Sprintf("User %s roles", tc.user), func(t *ftt.Test) {
				ctx := auth.WithState(context.Background(), &authtest.FakeState{
					Identity: tc.user,
					FakeDB:   fakeDB,
				})

				cfg := &prefixcfg.Entry{
					PrefixConfig: &configpb.Prefix{
						Path: "/",
						AllowWritersFromRegexp: []string{
							".*@google\\.com",
							".*@.*\\.gserviceaccount\\.com",
						},
					},
				}
				for _, raw := range cfg.PrefixConfig.AllowWritersFromRegexp {
					cfg.AllowWritersFromRegexp = append(cfg.AllowWritersFromRegexp, regexp.MustCompile(raw))
				}

				// Get the roles by checking explicitly each one via hasRole.
				t.Run("hasRole works", func(t *ftt.Test) {
					haveRoles := []repopb.Role{}
					for _, r := range allRoles {
						yes, _, err := hasRole(ctx, cfg, metas, r)
						assert.Loosely(t, err, should.BeNil)
						if yes {
							haveRoles = append(haveRoles, r)
						}
					}
					assert.Loosely(t, haveRoles, should.Match(tc.expectedRoles))
				})

				// Get the same set of roles through rolesInPrefix.
				t.Run("rolesInPrefix", func(t *ftt.Test) {
					haveRoles, err := rolesInPrefix(ctx, tc.user, cfg, metas)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, haveRoles, should.Match(tc.expectedRoles))
				})
			})
		}
	})
}

func addPrefixACLs(metas []*repopb.PrefixMetadata, prefix string, acls map[repopb.Role][]string) []*repopb.PrefixMetadata {
	a := make([]*repopb.PrefixMetadata_ACL, 0, len(acls))
	for role, principals := range acls {
		a = append(a, &repopb.PrefixMetadata_ACL{
			Role:       role,
			Principals: principals,
		})
	}
	return append(metas, &repopb.PrefixMetadata{
		Prefix: prefix,
		Acls:   a,
	})
}
