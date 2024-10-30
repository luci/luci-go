// Copyright 2022 The LUCI Authors.
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

package perms

import (
	"context"
	"testing"

	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/grpc/appstatus"
	"go.chromium.org/luci/resultdb/rdbperms"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
	"go.chromium.org/luci/server/auth/realms"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func init() {
	rdbperms.PermListTestResults.AddFlags(realms.UsedInQueryRealms)
	rdbperms.PermListTestExonerations.AddFlags(realms.UsedInQueryRealms)
	rdbperms.PermGetArtifact.AddFlags(realms.UsedInQueryRealms)
	rdbperms.PermListArtifacts.AddFlags(realms.UsedInQueryRealms)
}

func TestQueryRealms(t *testing.T) {
	ftt.Run("QueryRealms", t, func(t *ftt.Test) {
		ctx := context.Background()

		ctx = auth.WithState(ctx, &authtest.FakeState{
			Identity: "user:someone@example.com",
			IdentityPermissions: []authtest.RealmPermission{
				{
					Realm:      "project1:realm1",
					Permission: rdbperms.PermListTestResults,
				},
				{
					Realm:      "project1:realm1",
					Permission: rdbperms.PermListTestExonerations,
				},
				{
					Realm:      "project1:realm1",
					Permission: rdbperms.PermGetArtifact,
				},
				{
					Realm:      "project1:realm2",
					Permission: rdbperms.PermListTestResults,
				},
				{
					Realm:      "project1:realm2",
					Permission: rdbperms.PermListTestExonerations,
				},
				{
					Realm:      "project2:realm1",
					Permission: rdbperms.PermListTestResults,
				},
				{
					Realm:      "project2:realm1",
					Permission: rdbperms.PermGetArtifact,
				},
				{
					Realm:      "project2:realm1",
					Permission: rdbperms.PermListArtifacts,
				},
			},
		})

		t.Run("QueryRealms", func(t *ftt.Test) {
			t.Run("no permission specified", func(t *ftt.Test) {
				realms, err := QueryRealms(ctx, "project1", nil)
				assert.Loosely(t, err, should.ErrLike("at least one permission must be specified"))
				assert.Loosely(t, realms, should.BeEmpty)
			})

			t.Run("no project specified", func(t *ftt.Test) {
				realms, err := QueryRealms(ctx, "", nil, rdbperms.PermListTestResults)
				assert.Loosely(t, err, should.ErrLike("project must be specified"))
				assert.Loosely(t, realms, should.BeEmpty)
			})

			t.Run("check single permission", func(t *ftt.Test) {
				realms, err := QueryRealms(ctx, "project1", nil, rdbperms.PermListTestResults)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, realms, should.Resemble([]string{"project1:realm1", "project1:realm2"}))
			})

			t.Run("check multiple permissions", func(t *ftt.Test) {
				realms, err := QueryRealms(ctx, "project1", nil, rdbperms.PermListTestResults, rdbperms.PermGetArtifact)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, realms, should.Resemble([]string{"project1:realm1"}))
			})

			t.Run("no matched realms", func(t *ftt.Test) {
				realms, err := QueryRealms(ctx, "project1", nil, rdbperms.PermListTestExonerations, rdbperms.PermListArtifacts)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, realms, should.BeEmpty)
			})

			t.Run("no matched realms with non-empty method variant", func(t *ftt.Test) {
				realms, err := QueryRealmsNonEmpty(ctx, "project1", nil, rdbperms.PermListTestExonerations, rdbperms.PermListArtifacts)
				assert.That(t, err, should.ErrLike("caller does not have permissions"))
				assert.That(t, err, should.ErrLike("in any realm in project \"project1\""))
				assert.Loosely(t, appstatus.Code(err), should.Equal(codes.PermissionDenied))
				assert.Loosely(t, realms, should.BeEmpty)
			})
		})
		t.Run("QuerySubRealms", func(t *ftt.Test) {
			t.Run("no permission specified", func(t *ftt.Test) {
				realms, err := QuerySubRealmsNonEmpty(ctx, "project1", "realm1", nil)
				assert.Loosely(t, err, should.ErrLike("at least one permission must be specified"))
				assert.Loosely(t, realms, should.BeEmpty)
			})

			t.Run("no project specified", func(t *ftt.Test) {
				realms, err := QuerySubRealmsNonEmpty(ctx, "", "", nil, rdbperms.PermListTestResults)
				assert.Loosely(t, err, should.ErrLike("project must be specified"))
				assert.Loosely(t, realms, should.BeEmpty)
			})

			t.Run("project scope", func(t *ftt.Test) {
				t.Run("check single permission", func(t *ftt.Test) {
					realms, err := QuerySubRealmsNonEmpty(ctx, "project1", "", nil, rdbperms.PermListTestResults)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, realms, should.Resemble([]string{"realm1", "realm2"}))
				})

				t.Run("check multiple permissions", func(t *ftt.Test) {
					realms, err := QuerySubRealmsNonEmpty(ctx, "project1", "", nil, rdbperms.PermListTestResults, rdbperms.PermGetArtifact)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, realms, should.Resemble([]string{"realm1"}))
				})

				t.Run("no matched realms", func(t *ftt.Test) {
					realms, err := QuerySubRealmsNonEmpty(ctx, "project1", "", nil, rdbperms.PermListTestExonerations, rdbperms.PermListArtifacts)
					assert.That(t, err, should.ErrLike("caller does not have permissions"))
					assert.That(t, err, should.ErrLike("in any realm in project \"project1\""))
					assert.Loosely(t, appstatus.Code(err), should.Equal(codes.PermissionDenied))
					assert.Loosely(t, realms, should.BeEmpty)
				})
			})

			t.Run("realm scope", func(t *ftt.Test) {
				t.Run("check single permission", func(t *ftt.Test) {
					realms, err := QuerySubRealmsNonEmpty(ctx, "project1", "realm1", nil, rdbperms.PermListTestResults)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, realms, should.Resemble([]string{"realm1"}))
				})

				t.Run("check multiple permissions", func(t *ftt.Test) {
					realms, err := QuerySubRealmsNonEmpty(ctx, "project1", "realm1", nil, rdbperms.PermListTestResults, rdbperms.PermGetArtifact)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, realms, should.Resemble([]string{"realm1"}))
				})

				t.Run("no matched realms", func(t *ftt.Test) {
					realms, err := QuerySubRealmsNonEmpty(ctx, "project1", "realm1", nil, rdbperms.PermListTestExonerations, rdbperms.PermListArtifacts)
					assert.Loosely(t, err, should.ErrLike("caller does not have permission"))
					assert.Loosely(t, err, should.ErrLike("in realm \"project1:realm1\""))
					assert.Loosely(t, appstatus.Code(err), should.Equal(codes.PermissionDenied))
					assert.Loosely(t, realms, should.BeEmpty)
				})
			})
		})
	})
}
