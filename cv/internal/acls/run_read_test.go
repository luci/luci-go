// Copyright 2021 The LUCI Authors.
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

package acls

import (
	"testing"

	"github.com/google/go-cmp/cmp/cmpopts"
	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/registry"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/grpc/appstatus"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"

	cfgpb "go.chromium.org/luci/cv/api/config/v2"
	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/configs/prjcfg/prjcfgtest"
	"go.chromium.org/luci/cv/internal/configs/validation"
	"go.chromium.org/luci/cv/internal/cvtesting"
	"go.chromium.org/luci/cv/internal/run"
)

func init() {
	registry.RegisterCmpOption(cmpopts.IgnoreUnexported(run.Run{}))
}

func TestRunReadChecker(t *testing.T) {
	t.Parallel()

	ftt.Run("NewRunReadChecker works", t, func(t *ftt.Test) {
		ct := cvtesting.Test{}
		ctx := ct.SetUp(t)

		const projectPublic = "infra"
		const projectInternal = "infra-internal"

		prjcfgtest.Create(ctx, projectPublic, &cfgpb.Config{
			// TODO(crbug/1233963): update this test to stop relying on legacy-based
			// ACL.
			CqStatusHost: validation.CQStatusHostPublic,
			ConfigGroups: []*cfgpb.ConfigGroup{{
				Name: "first",
			}},
		})
		prjcfgtest.Create(ctx, projectInternal, &cfgpb.Config{
			// TODO(crbug/1233963): update this test to stop relying on legacy-based
			// ACL.
			CqStatusHost: validation.CQStatusHostInternal,
			ConfigGroups: []*cfgpb.ConfigGroup{{
				Name: "first",
			}},
		})

		publicRun := &run.Run{
			ID:            common.RunID(projectPublic + "/123-visible"),
			EVersion:      5,
			ConfigGroupID: prjcfgtest.MustExist(ctx, projectPublic).ConfigGroupIDs[0],
		}
		internalRun := &run.Run{
			ID:            common.RunID(projectInternal + "/456-invisible"),
			EVersion:      5,
			ConfigGroupID: prjcfgtest.MustExist(ctx, projectInternal).ConfigGroupIDs[0],
		}
		assert.NoErr(t, datastore.Put(ctx, publicRun, internalRun))

		ctx = auth.WithState(ctx, &authtest.FakeState{
			Identity: identity.AnonymousIdentity,
		})

		t.Run("Loading individual Runs", func(t *ftt.Test) {
			t.Run("Run doesn't exist", func(t *ftt.Test) {
				_, err1 := run.LoadRun(ctx, common.RunID("foo/bar"), NewRunReadChecker())
				assert.Loosely(t, appstatus.Code(err1), should.Equal(codes.NotFound))

				t.Run("No access must be indistinguishable from not existing Run", func(t *ftt.Test) {
					_, err2 := run.LoadRun(ctx, internalRun.ID, NewRunReadChecker())
					assert.Loosely(t, appstatus.Code(err2), should.Equal(codes.NotFound))

					st1, _ := appstatus.Get(err1)
					st2, _ := appstatus.Get(err2)
					assert.That(t, st1.Message(), should.Match(st2.Message()))
					assert.Loosely(t, st1.Details(), should.BeEmpty)
					assert.Loosely(t, st2.Details(), should.BeEmpty)
				})
			})

			t.Run("OK public", func(t *ftt.Test) {
				r, err := run.LoadRun(ctx, publicRun.ID, NewRunReadChecker())
				assert.NoErr(t, err)
				assert.That(t, r, should.Match(publicRun))
			})

			t.Run("OK internal", func(t *ftt.Test) {
				// TODO(crbug/1233963): add a test once non-legacy ACLs are working.
				ctx = auth.WithState(ctx, &authtest.FakeState{
					Identity:       "user:googler@example.com",
					IdentityGroups: []string{"internal-cq-status-access"},
				})
				r, err := run.LoadRun(ctx, internalRun.ID, NewRunReadChecker())
				assert.NoErr(t, err)
				assert.That(t, r, should.Match(internalRun))
			})

			t.Run("OK v0 API users", func(t *ftt.Test) {
				ctx = auth.WithState(ctx, &authtest.FakeState{
					Identity:       "user:v0-api-users@example.com",
					IdentityGroups: []string{V0APIAllowGroup},
				})
				r, err := run.LoadRun(ctx, publicRun.ID, NewRunReadChecker())
				assert.NoErr(t, err)
				assert.That(t, r, should.Match(publicRun))
				r, err = run.LoadRun(ctx, internalRun.ID, NewRunReadChecker())
				assert.NoErr(t, err)
				assert.That(t, r, should.Match(internalRun))
			})

			t.Run("PermissionDenied", func(t *ftt.Test) {
				ctx = auth.WithState(ctx, &authtest.FakeState{
					Identity:       "user:public-user@example.com",
					IdentityGroups: []string{"insufficient"},
				})
				_, err := run.LoadRun(ctx, internalRun.ID, NewRunReadChecker())
				assert.Loosely(t, appstatus.Code(err), should.Equal(codes.NotFound))
			})
		})
	})
}
