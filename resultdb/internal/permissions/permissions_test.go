// Copyright 2022 The LUCI Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
package permissions

import (
	"testing"

	"cloud.google.com/go/spanner"
	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/grpc/appstatus"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
	"go.chromium.org/luci/server/auth/realms"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/resultdb/internal/invocations"
	"go.chromium.org/luci/resultdb/internal/rootinvocations"
	"go.chromium.org/luci/resultdb/internal/testutil"
	"go.chromium.org/luci/resultdb/internal/testutil/insert"
	"go.chromium.org/luci/resultdb/internal/workunits"
	pb "go.chromium.org/luci/resultdb/proto/v1"
	"go.chromium.org/luci/resultdb/rdbperms"
)

func TestVerifyInvocations(t *testing.T) {
	ftt.Run(`VerifyInvocations`, t, func(t *ftt.Test) {
		ctx := auth.WithState(testutil.SpannerTestContext(t), &authtest.FakeState{
			Identity: "user:someone@example.com",
			IdentityPermissions: []authtest.RealmPermission{
				{Realm: "testproject:r1", Permission: rdbperms.PermListArtifacts},
				{Realm: "testproject:r2", Permission: rdbperms.PermListArtifacts},
				{Realm: "testproject:r2", Permission: rdbperms.PermListTestExonerations},
				{Realm: "testproject:r2", Permission: rdbperms.PermListTestResults},
				{Realm: "testproject:r3", Permission: rdbperms.PermListTestExonerations},
				{Realm: "testproject:r3", Permission: rdbperms.PermListTestResults},
			},
		})
		testutil.MustApply(
			ctx, t,
			insert.Invocation("i0", pb.Invocation_ACTIVE, map[string]any{"Realm": "testproject:r0"}),
			insert.Invocation("i1", pb.Invocation_ACTIVE, map[string]any{"Realm": "testproject:r1"}),
			insert.Invocation("i2", pb.Invocation_ACTIVE, map[string]any{"Realm": "testproject:r2"}),
			insert.Invocation("i3", pb.Invocation_ACTIVE, map[string]any{"Realm": "testproject:r3"}),
			insert.Invocation("i3b", pb.Invocation_ACTIVE, map[string]any{"Realm": "testproject:r3"}),
		)

		t.Run("Access allowed", func(t *ftt.Test) {
			ids := invocations.NewIDSet(invocations.ID("i1"), invocations.ID("i2"))
			err := VerifyInvocations(span.Single(ctx), ids, rdbperms.PermListArtifacts)
			assert.Loosely(t, err, should.BeNil)

			ids = invocations.NewIDSet(invocations.ID("i2"), invocations.ID("i3"))
			err = VerifyInvocations(span.Single(ctx), ids, rdbperms.PermListTestExonerations, rdbperms.PermListTestResults)
			assert.Loosely(t, err, should.BeNil)
		})
		t.Run("Access denied", func(t *ftt.Test) {
			ids := invocations.NewIDSet(invocations.ID("i0"))
			err := VerifyInvocations(span.Single(ctx), ids, rdbperms.PermListArtifacts)
			assert.Loosely(t, appstatus.Code(err), should.Equal(codes.PermissionDenied))
			assert.Loosely(t, err, should.ErrLike("resultdb.artifacts.list in realm of \"invocations/i0\""))

			ids = invocations.NewIDSet(invocations.ID("i1"), invocations.ID("i2"))
			err = VerifyInvocations(span.Single(ctx), ids, rdbperms.PermListArtifacts, rdbperms.PermListTestExonerations)
			assert.Loosely(t, appstatus.Code(err), should.Equal(codes.PermissionDenied))
			assert.Loosely(t, err, should.ErrLike("resultdb.testExonerations.list in realm of \"invocations/i1\""))

			ids = invocations.NewIDSet(invocations.ID("i2"), invocations.ID("i3"))
			err = VerifyInvocations(span.Single(ctx), ids, rdbperms.PermListTestExonerations, rdbperms.PermListTestResults, rdbperms.PermListArtifacts)
			assert.Loosely(t, appstatus.Code(err), should.Equal(codes.PermissionDenied))
			assert.Loosely(t, err, should.ErrLike("resultdb.artifacts.list in realm of \"invocations/i3\""))
		})
		t.Run("Duplicate invocations", func(t *ftt.Test) {
			ids := invocations.NewIDSet(invocations.ID("i2"), invocations.ID("i3"), invocations.ID("i3"))
			err := VerifyInvocations(span.Single(ctx), ids, rdbperms.PermListTestExonerations, rdbperms.PermListTestResults)
			assert.Loosely(t, err, should.BeNil)

			ids = invocations.NewIDSet(invocations.ID("i2"), invocations.ID("i3"), invocations.ID("i3"))
			err = VerifyInvocations(span.Single(ctx), ids, rdbperms.PermListTestExonerations, rdbperms.PermListTestResults, rdbperms.PermListArtifacts)
			assert.Loosely(t, appstatus.Code(err), should.Equal(codes.PermissionDenied))
			assert.Loosely(t, err, should.ErrLike("resultdb.artifacts.list in realm of \"invocations/i3\""))
		})
		t.Run("Duplicate realms", func(t *ftt.Test) {
			ids := invocations.NewIDSet(invocations.ID("i2"), invocations.ID("i3"), invocations.ID("i3b"))
			err := VerifyInvocations(span.Single(ctx), ids, rdbperms.PermListTestExonerations, rdbperms.PermListTestResults)
			assert.Loosely(t, err, should.BeNil)

			ids = invocations.NewIDSet(invocations.ID("i2"), invocations.ID("i3"), invocations.ID("i3b"))
			err = VerifyInvocations(span.Single(ctx), ids, rdbperms.PermListTestExonerations, rdbperms.PermListTestResults, rdbperms.PermListArtifacts)
			assert.Loosely(t, appstatus.Code(err), should.Equal(codes.PermissionDenied))
			// The error can be either for i3 or i3b, the order is undeterministic.
			assert.Loosely(t, err, should.ErrLike("resultdb.artifacts.list in realm of \"invocations/i3"))
		})
		t.Run("Invocations do not exist", func(t *ftt.Test) {
			ids := invocations.NewIDSet(invocations.ID("i2"), invocations.ID("iX"))
			err := VerifyInvocations(span.Single(ctx), ids, rdbperms.PermListTestExonerations)
			assert.Loosely(t, appstatus.Code(err), should.Equal(codes.NotFound))
			assert.Loosely(t, err, should.ErrLike("invocations/iX not found"))

			ids = invocations.NewIDSet(invocations.ID("i2"), invocations.ID(""))
			err = VerifyInvocations(span.Single(ctx), ids, rdbperms.PermListTestExonerations)
			assert.Loosely(t, appstatus.Code(err), should.Equal(codes.NotFound))
			assert.Loosely(t, err, should.ErrLike("invocations/ not found"))
		})
		t.Run("No invocations", func(t *ftt.Test) {
			ids := invocations.NewIDSet()
			err := VerifyInvocations(span.Single(ctx), ids, rdbperms.PermListTestExonerations, rdbperms.PermListTestResults)
			assert.Loosely(t, err, should.BeNil)
		})
	})
}

func TestVerifyWorkUnitAccess(t *testing.T) {
	ftt.Run(`VerifyWorkUnitAccess`, t, func(t *ftt.Test) {
		ctx := testutil.SpannerTestContext(t)

		rootInvID := rootinvocations.ID("root-inv")
		workUnitID := workunits.ID{
			RootInvocationID: rootInvID,
			WorkUnitID:       "work-unit",
		}

		// Insert root invocation for subsequent tests.
		var ms []*spanner.Mutation
		ms = append(ms, insert.RootInvocationWithRootWorkUnit(
			rootinvocations.NewBuilder(rootInvID).WithRealm("testproject:root").Build(),
		)...)
		ms = append(ms, insert.WorkUnit(
			workunits.NewBuilder(rootInvID, "work-unit").WithRealm("testproject:workunit").Build(),
		)...)
		testutil.MustApply(ctx, t, ms...)

		opts := VerifyWorkUnitAccessOptions{
			Full:                 []realms.Permission{rdbperms.PermListTestResults, rdbperms.PermListTestExonerations},
			Limited:              []realms.Permission{rdbperms.PermListLimitedTestResults, rdbperms.PermListLimitedTestExonerations},
			UpgradeLimitedToFull: []realms.Permission{rdbperms.PermGetTestResult, rdbperms.PermGetTestExoneration},
		}

		authState := &authtest.FakeState{
			Identity: "user:someone@example.com",
		}
		ctx = auth.WithState(ctx, authState)

		ctx, cancel := span.ReadOnlyTransaction(ctx)
		defer cancel()

		t.Run("root invocation not found", func(t *ftt.Test) {
			workUnitID.RootInvocationID = rootinvocations.ID("not-exists")
			_, err := VerifyWorkUnitAccess(ctx, workUnitID, opts, NoAccess)
			st, ok := appstatus.Get(err)
			assert.That(t, ok, should.BeTrue)
			assert.That(t, st.Code(), should.Equal(codes.NotFound))
			assert.That(t, st.Message(), should.ContainSubstring(`"rootInvocations/not-exists" not found`))
		})
		t.Run("no access on root invocation", func(t *ftt.Test) {
			authState.IdentityPermissions = nil
			level, err := VerifyWorkUnitAccess(ctx, workUnitID, opts, NoAccess)
			assert.Loosely(t, err, should.BeNil)
			assert.That(t, level, should.Equal(NoAccess))
		})
		t.Run("full access on root invocation", func(t *ftt.Test) {
			t.Run("baseline", func(t *ftt.Test) {
				authState.IdentityPermissions = []authtest.RealmPermission{
					{Realm: "testproject:root", Permission: rdbperms.PermListTestResults},
					{Realm: "testproject:root", Permission: rdbperms.PermListTestExonerations},
				}
				level, err := VerifyWorkUnitAccess(ctx, workUnitID, opts, NoAccess)
				assert.Loosely(t, err, should.BeNil)
				assert.That(t, level, should.Equal(FullAccess))
			})
			t.Run("missing one permission", func(t *ftt.Test) {
				authState.IdentityPermissions = []authtest.RealmPermission{
					{Realm: "testproject:root", Permission: rdbperms.PermListTestResults},
				}
				level, err := VerifyWorkUnitAccess(ctx, workUnitID, opts, NoAccess)
				assert.Loosely(t, err, should.BeNil)
				assert.That(t, level, should.Equal(NoAccess))
			})
		})
		t.Run("limited access on root invocation", func(t *ftt.Test) {
			t.Run("no upgrade permission", func(t *ftt.Test) {
				t.Run("baseline", func(t *ftt.Test) {
					authState.IdentityPermissions = []authtest.RealmPermission{
						{Realm: "testproject:root", Permission: rdbperms.PermListLimitedTestResults},
						{Realm: "testproject:root", Permission: rdbperms.PermListLimitedTestExonerations},
					}
					level, err := VerifyWorkUnitAccess(ctx, workUnitID, opts, NoAccess)
					assert.Loosely(t, err, should.BeNil)
					assert.That(t, level, should.Equal(LimitedAccess))
				})
				t.Run("missing one permission", func(t *ftt.Test) {
					authState.IdentityPermissions = []authtest.RealmPermission{
						{Realm: "testproject:root", Permission: rdbperms.PermListLimitedTestResults},
					}
					level, err := VerifyWorkUnitAccess(ctx, workUnitID, opts, NoAccess)
					assert.Loosely(t, err, should.BeNil)
					assert.That(t, level, should.Equal(NoAccess))
				})
			})
			t.Run("upgrade to full access", func(t *ftt.Test) {
				authState.IdentityPermissions = []authtest.RealmPermission{
					{Realm: "testproject:root", Permission: rdbperms.PermListLimitedTestResults},
					{Realm: "testproject:root", Permission: rdbperms.PermListLimitedTestExonerations},
					{Realm: "testproject:workunit", Permission: rdbperms.PermGetTestResult},
					{Realm: "testproject:workunit", Permission: rdbperms.PermGetTestExoneration},
				}
				t.Run("baseline", func(t *ftt.Test) {
					level, err := VerifyWorkUnitAccess(ctx, workUnitID, opts, NoAccess)
					assert.Loosely(t, err, should.BeNil)
					assert.That(t, level, should.Equal(FullAccess))
				})
				t.Run("missing one upgrade permission", func(t *ftt.Test) {
					authState.IdentityPermissions = []authtest.RealmPermission{
						{Realm: "testproject:root", Permission: rdbperms.PermListLimitedTestResults},
						{Realm: "testproject:root", Permission: rdbperms.PermListLimitedTestExonerations},
						{Realm: "testproject:workunit", Permission: rdbperms.PermGetTestResult},
					}
					level, err := VerifyWorkUnitAccess(ctx, workUnitID, opts, NoAccess)
					assert.Loosely(t, err, should.BeNil)
					assert.That(t, level, should.Equal(LimitedAccess))
				})
			})
			t.Run("work unit not found", func(t *ftt.Test) {
				authState.IdentityPermissions = []authtest.RealmPermission{
					{Realm: "testproject:root", Permission: rdbperms.PermListLimitedTestResults},
					{Realm: "testproject:root", Permission: rdbperms.PermListLimitedTestExonerations},
				}
				workUnitID.WorkUnitID = "not-exists"
				_, err := VerifyWorkUnitAccess(ctx, workUnitID, opts, NoAccess)
				st, ok := appstatus.Get(err)
				assert.That(t, ok, should.BeTrue)
				assert.That(t, st.Code(), should.Equal(codes.NotFound))
				assert.That(t, st.Message(), should.ContainSubstring(`"rootInvocations/root-inv/workUnits/not-exists" not found`))
			})
		})
		t.Run("minimum access level enforcement", func(t *ftt.Test) {
			t.Run("required FullAccess, have LimitedAccess", func(t *ftt.Test) {
				authState.IdentityPermissions = []authtest.RealmPermission{
					{Realm: "testproject:root", Permission: rdbperms.PermListLimitedTestResults},
					{Realm: "testproject:root", Permission: rdbperms.PermListLimitedTestExonerations},
				}
				_, err := VerifyWorkUnitAccess(ctx, workUnitID, opts, FullAccess)
				assert.Loosely(t, err, should.ErrLike("caller does not have permissions [resultdb.testResults.get, resultdb.testExonerations.get] in realm \"testproject:workunit\" of work unit \"rootInvocations/root-inv/workUnits/work-unit\" (trying to upgrade limited access to full access)"))
				assert.Loosely(t, appstatus.Code(err), should.Equal(codes.PermissionDenied))
			})
			t.Run("required LimitedAccess, have NoAccess", func(t *ftt.Test) {
				authState.IdentityPermissions = nil
				_, err := VerifyWorkUnitAccess(ctx, workUnitID, opts, LimitedAccess)
				assert.Loosely(t, err, should.ErrLike("caller does not have permissions [resultdb.testResults.listLimited, resultdb.testExonerations.listLimited] (or [resultdb.testResults.list, resultdb.testExonerations.list]) in realm of root invocation \"rootInvocations/root-inv\""))
				assert.Loosely(t, appstatus.Code(err), should.Equal(codes.PermissionDenied))
			})
		})
	})
}

func TestVerifyWorkUnitsAccess(t *testing.T) {
	ftt.Run(`VerifyWorkUnitsAccess`, t, func(t *ftt.Test) {
		ctx := testutil.SpannerTestContext(t)

		rootInvID := rootinvocations.ID("root-inv")
		workUnitIDs := []workunits.ID{
			{RootInvocationID: rootInvID, WorkUnitID: "work-unit-1"},
			{RootInvocationID: rootInvID, WorkUnitID: "work-unit-2"},
		}

		// Insert root invocation and work units for subsequent tests.
		var ms []*spanner.Mutation
		ms = append(ms, insert.RootInvocationWithRootWorkUnit(
			rootinvocations.NewBuilder(rootInvID).WithRealm("testproject:root").Build(),
		)...)
		ms = append(ms, insert.WorkUnit(
			workunits.NewBuilder(rootInvID, "work-unit-1").WithRealm("testproject:workunit1").Build(),
		)...)
		ms = append(ms, insert.WorkUnit(
			workunits.NewBuilder(rootInvID, "work-unit-2").WithRealm("testproject:workunit2").Build(),
		)...)
		testutil.MustApply(ctx, t, ms...)

		opts := VerifyWorkUnitAccessOptions{
			Full:                 []realms.Permission{rdbperms.PermListTestResults, rdbperms.PermListTestExonerations},
			Limited:              []realms.Permission{rdbperms.PermListLimitedTestResults, rdbperms.PermListLimitedTestExonerations},
			UpgradeLimitedToFull: []realms.Permission{rdbperms.PermGetTestResult, rdbperms.PermGetTestExoneration},
		}

		authState := &authtest.FakeState{
			Identity: "user:someone@example.com",
		}
		ctx = auth.WithState(ctx, authState)

		ctx, cancel := span.ReadOnlyTransaction(ctx)
		defer cancel()

		t.Run("work units in different root invocation", func(t *ftt.Test) {
			workUnitIDs[1] = workunits.ID{RootInvocationID: rootinvocations.ID("other-root"), WorkUnitID: "work-unit-1"}
			_, err := VerifyWorkUnitsAccess(ctx, workUnitIDs, opts, NoAccess)
			assert.That(t, err, should.ErrLike("all work units must belong to the same root invocation"))
		})
		t.Run("root invocation not found", func(t *ftt.Test) {
			workUnitIDs = []workunits.ID{
				{RootInvocationID: rootinvocations.ID("not-exists"), WorkUnitID: "work-unit-1"},
			}
			_, err := VerifyWorkUnitsAccess(ctx, workUnitIDs, opts, NoAccess)
			st, ok := appstatus.Get(err)
			assert.That(t, ok, should.BeTrue)
			assert.That(t, st.Code(), should.Equal(codes.NotFound))
			assert.That(t, st.Message(), should.ContainSubstring(`"rootInvocations/not-exists" not found`))
		})
		t.Run("no access on root invocation", func(t *ftt.Test) {
			authState.IdentityPermissions = nil
			levels, err := VerifyWorkUnitsAccess(ctx, workUnitIDs, opts, NoAccess)
			assert.Loosely(t, err, should.BeNil)
			assert.That(t, levels, should.Match([]AccessLevel{NoAccess, NoAccess}))
		})
		t.Run("full access on root invocation", func(t *ftt.Test) {
			t.Run("baseline", func(t *ftt.Test) {
				authState.IdentityPermissions = []authtest.RealmPermission{
					{Realm: "testproject:root", Permission: rdbperms.PermListTestResults},
					{Realm: "testproject:root", Permission: rdbperms.PermListTestExonerations},
				}
				levels, err := VerifyWorkUnitsAccess(ctx, workUnitIDs, opts, NoAccess)
				assert.Loosely(t, err, should.BeNil)
				assert.That(t, levels, should.Match([]AccessLevel{FullAccess, FullAccess}))
			})
			t.Run("missing one permission", func(t *ftt.Test) {
				authState.IdentityPermissions = []authtest.RealmPermission{
					{Realm: "testproject:root", Permission: rdbperms.PermListTestResults},
				}
				levels, err := VerifyWorkUnitsAccess(ctx, workUnitIDs, opts, NoAccess)
				assert.Loosely(t, err, should.BeNil)
				assert.That(t, levels, should.Match([]AccessLevel{NoAccess, NoAccess}))
			})
		})
		t.Run("limited access on root invocation", func(t *ftt.Test) {
			authState.IdentityPermissions = []authtest.RealmPermission{
				{Realm: "testproject:root", Permission: rdbperms.PermListLimitedTestResults},
				{Realm: "testproject:root", Permission: rdbperms.PermListLimitedTestExonerations},
			}
			t.Run("no upgrade permission", func(t *ftt.Test) {
				t.Run("baseline", func(t *ftt.Test) {
					levels, err := VerifyWorkUnitsAccess(ctx, workUnitIDs, opts, NoAccess)
					assert.Loosely(t, err, should.BeNil)
					assert.That(t, levels, should.Match([]AccessLevel{LimitedAccess, LimitedAccess}))
				})
				t.Run("missing one permission", func(t *ftt.Test) {
					authState.IdentityPermissions = []authtest.RealmPermission{
						{Realm: "testproject:root", Permission: rdbperms.PermListLimitedTestResults},
					}
					levels, err := VerifyWorkUnitsAccess(ctx, workUnitIDs, opts, NoAccess)
					assert.Loosely(t, err, should.BeNil)
					assert.That(t, levels, should.Match([]AccessLevel{NoAccess, NoAccess}))
				})
			})
			t.Run("upgrade to full access for some", func(t *ftt.Test) {
				authState.IdentityPermissions = append(authState.IdentityPermissions, authtest.RealmPermission{
					Realm: "testproject:workunit1", Permission: rdbperms.PermGetTestResult,
				})
				authState.IdentityPermissions = append(authState.IdentityPermissions, authtest.RealmPermission{
					Realm: "testproject:workunit1", Permission: rdbperms.PermGetTestExoneration,
				})
				t.Run("baseline", func(t *ftt.Test) {
					levels, err := VerifyWorkUnitsAccess(ctx, workUnitIDs, opts, NoAccess)
					assert.Loosely(t, err, should.BeNil)
					assert.That(t, levels, should.Match([]AccessLevel{FullAccess, LimitedAccess}))
				})
				t.Run("missing one upgrade permission", func(t *ftt.Test) {
					authState.IdentityPermissions = []authtest.RealmPermission{
						{Realm: "testproject:root", Permission: rdbperms.PermListLimitedTestResults},
						{Realm: "testproject:root", Permission: rdbperms.PermListLimitedTestExonerations},
						{Realm: "testproject:workunit1", Permission: rdbperms.PermGetTestResult},
					}
					levels, err := VerifyWorkUnitsAccess(ctx, workUnitIDs, opts, NoAccess)
					assert.Loosely(t, err, should.BeNil)
					assert.That(t, levels, should.Match([]AccessLevel{LimitedAccess, LimitedAccess}))
				})
			})
			t.Run("work unit not found", func(t *ftt.Test) {
				workUnitIDs[1] = workunits.ID{RootInvocationID: rootInvID, WorkUnitID: "not-exists"}
				_, err := VerifyWorkUnitsAccess(ctx, workUnitIDs, opts, NoAccess)
				st, ok := appstatus.Get(err)
				assert.That(t, ok, should.BeTrue)
				assert.That(t, st.Code(), should.Equal(codes.NotFound))
				assert.That(t, st.Message(), should.ContainSubstring(`"rootInvocations/root-inv/workUnits/not-exists" not found`))
			})
		})
		t.Run("minimum access level enforcement", func(t *ftt.Test) {
			t.Run("required FullAccess, have LimitedAccess", func(t *ftt.Test) {
				authState.IdentityPermissions = []authtest.RealmPermission{
					{Realm: "testproject:root", Permission: rdbperms.PermListLimitedTestResults},
					{Realm: "testproject:root", Permission: rdbperms.PermListLimitedTestExonerations},
				}
				_, err := VerifyWorkUnitsAccess(ctx, workUnitIDs, opts, FullAccess)
				assert.Loosely(t, err, should.ErrLike("caller does not have permissions [resultdb.testResults.get, resultdb.testExonerations.get] in realm \"testproject:workunit1\" of work unit \"rootInvocations/root-inv/workUnits/work-unit-1\" (trying to upgrade limited access to full access)"))
				assert.Loosely(t, appstatus.Code(err), should.Equal(codes.PermissionDenied))
			})
			t.Run("required LimitedAccess, have NoAccess", func(t *ftt.Test) {
				authState.IdentityPermissions = nil
				_, err := VerifyWorkUnitsAccess(ctx, workUnitIDs, opts, LimitedAccess)
				assert.Loosely(t, err, should.ErrLike("caller does not have permissions [resultdb.testResults.listLimited, resultdb.testExonerations.listLimited] (or [resultdb.testResults.list, resultdb.testExonerations.list]) in realm of root invocation \"rootInvocations/root-inv\""))
				assert.Loosely(t, appstatus.Code(err), should.Equal(codes.PermissionDenied))
			})
		})
	})
}

func TestVerifyAllWorkUnitsAccess(t *testing.T) {
	ftt.Run(`VerifyAllWorkUnitsAccess`, t, func(t *ftt.Test) {
		ctx := testutil.SpannerTestContext(t)

		rootInvID := rootinvocations.ID("root-inv")

		// Insert root invocation and work units for subsequent tests.
		var ms []*spanner.Mutation
		ms = append(ms, insert.RootInvocationWithRootWorkUnit(
			rootinvocations.NewBuilder(rootInvID).WithRealm("testproject:root").Build(),
		)...)
		ms = append(ms, insert.WorkUnit(
			workunits.NewBuilder(rootInvID, "work-unit-1").WithRealm("testproject:workunit1").Build(),
		)...)
		ms = append(ms, insert.WorkUnit(
			workunits.NewBuilder(rootInvID, "work-unit-2").WithRealm("testproject:workunit2").Build(),
		)...)
		testutil.MustApply(ctx, t, ms...)

		opts := VerifyWorkUnitAccessOptions{
			Full:                 []realms.Permission{rdbperms.PermListTestResults, rdbperms.PermListTestExonerations},
			Limited:              []realms.Permission{rdbperms.PermListLimitedTestResults, rdbperms.PermListLimitedTestExonerations},
			UpgradeLimitedToFull: []realms.Permission{rdbperms.PermGetTestResult, rdbperms.PermGetTestExoneration},
		}

		authState := &authtest.FakeState{
			Identity: "user:someone@example.com",
		}
		ctx = auth.WithState(ctx, authState)

		ctx, cancel := span.ReadOnlyTransaction(ctx)
		defer cancel()

		t.Run("root invocation not found", func(t *ftt.Test) {
			_, err := VerifyAllWorkUnitsAccess(ctx, rootinvocations.ID("not-exists"), opts, NoAccess)
			st, ok := appstatus.Get(err)
			assert.That(t, ok, should.BeTrue)
			assert.That(t, st.Code(), should.Equal(codes.NotFound))
			assert.That(t, st.Message(), should.ContainSubstring(`"rootInvocations/not-exists" not found`))
		})
		t.Run("no access on root invocation", func(t *ftt.Test) {
			authState.IdentityPermissions = nil
			result, err := VerifyAllWorkUnitsAccess(ctx, rootInvID, opts, NoAccess)
			assert.Loosely(t, err, should.BeNil)
			assert.That(t, result, should.Match(RootInvocationAccess{Level: NoAccess}))
		})
		t.Run("full access on root invocation", func(t *ftt.Test) {
			authState.IdentityPermissions = []authtest.RealmPermission{
				{Realm: "testproject:root", Permission: rdbperms.PermListTestResults},
				{Realm: "testproject:root", Permission: rdbperms.PermListTestExonerations},
			}
			t.Run("baseline", func(t *ftt.Test) {
				result, err := VerifyAllWorkUnitsAccess(ctx, rootInvID, opts, NoAccess)
				assert.Loosely(t, err, should.BeNil)
				assert.That(t, result, should.Match(RootInvocationAccess{Level: FullAccess}))
			})
			t.Run("missing one permission", func(t *ftt.Test) {
				authState.IdentityPermissions = []authtest.RealmPermission{
					{Realm: "testproject:root", Permission: rdbperms.PermListTestResults},
				}
				result, err := VerifyAllWorkUnitsAccess(ctx, rootInvID, opts, NoAccess)
				assert.Loosely(t, err, should.BeNil)
				assert.That(t, result, should.Match(RootInvocationAccess{Level: NoAccess}))
			})
		})
		t.Run("limited access on root invocation", func(t *ftt.Test) {
			authState.IdentityPermissions = []authtest.RealmPermission{
				{Realm: "testproject:root", Permission: rdbperms.PermListLimitedTestResults},
				{Realm: "testproject:root", Permission: rdbperms.PermListLimitedTestExonerations},
			}
			t.Run("no upgrade permission", func(t *ftt.Test) {
				t.Run("baseline", func(t *ftt.Test) {
					result, err := VerifyAllWorkUnitsAccess(ctx, rootInvID, opts, NoAccess)
					assert.Loosely(t, err, should.BeNil)
					assert.That(t, result, should.Match(RootInvocationAccess{Level: LimitedAccess, Realms: []string{}}))
				})
				t.Run("missing one permission", func(t *ftt.Test) {
					authState.IdentityPermissions = []authtest.RealmPermission{
						{Realm: "testproject:root", Permission: rdbperms.PermListLimitedTestResults},
					}
					result, err := VerifyAllWorkUnitsAccess(ctx, rootInvID, opts, NoAccess)
					assert.Loosely(t, err, should.BeNil)
					assert.That(t, result, should.Match(RootInvocationAccess{Level: NoAccess}))
				})
			})
			t.Run("upgrade to full access for some", func(t *ftt.Test) {
				authState.IdentityPermissions = append(authState.IdentityPermissions, authtest.RealmPermission{
					Realm: "testproject:workunit1", Permission: rdbperms.PermGetTestResult,
				})
				authState.IdentityPermissions = append(authState.IdentityPermissions, authtest.RealmPermission{
					Realm: "testproject:workunit1", Permission: rdbperms.PermGetTestExoneration,
				})
				t.Run("baseline", func(t *ftt.Test) {
					result, err := VerifyAllWorkUnitsAccess(ctx, rootInvID, opts, NoAccess)
					assert.Loosely(t, err, should.BeNil)
					assert.That(t, result, should.Match(RootInvocationAccess{Level: LimitedAccess, Realms: []string{"testproject:workunit1"}}))
				})
				t.Run("missing one upgrade permission", func(t *ftt.Test) {
					authState.IdentityPermissions = []authtest.RealmPermission{
						{Realm: "testproject:root", Permission: rdbperms.PermListLimitedTestResults},
						{Realm: "testproject:root", Permission: rdbperms.PermListLimitedTestExonerations},
						{Realm: "testproject:workunit1", Permission: rdbperms.PermGetTestResult},
					}
					result, err := VerifyAllWorkUnitsAccess(ctx, rootInvID, opts, NoAccess)
					assert.Loosely(t, err, should.BeNil)
					assert.That(t, result, should.Match(RootInvocationAccess{Level: LimitedAccess, Realms: []string{}}))
				})
			})
		})
		t.Run("minimum access level enforcement", func(t *ftt.Test) {
			t.Run("required FullAccess, have LimitedAccess", func(t *ftt.Test) {
				authState.IdentityPermissions = []authtest.RealmPermission{
					{Realm: "testproject:root", Permission: rdbperms.PermListLimitedTestResults},
					{Realm: "testproject:root", Permission: rdbperms.PermListLimitedTestExonerations},
				}
				_, err := VerifyAllWorkUnitsAccess(ctx, rootInvID, opts, FullAccess)
				assert.Loosely(t, err, should.ErrLike("caller does not have permissions [resultdb.testResults.list, resultdb.testExonerations.list] in realm of root invocation \"rootInvocations/root-inv\""))
				assert.Loosely(t, appstatus.Code(err), should.Equal(codes.PermissionDenied))
			})
			t.Run("required LimitedAccess, have NoAccess", func(t *ftt.Test) {
				authState.IdentityPermissions = nil
				_, err := VerifyAllWorkUnitsAccess(ctx, rootInvID, opts, LimitedAccess)
				assert.Loosely(t, err, should.ErrLike("caller does not have permissions [resultdb.testResults.listLimited, resultdb.testExonerations.listLimited] (or [resultdb.testResults.list, resultdb.testExonerations.list]) in realm of root invocation \"rootInvocations/root-inv\""))
				assert.Loosely(t, appstatus.Code(err), should.Equal(codes.PermissionDenied))
			})
		})
	})
}
