// Copyright 2025 The LUCI Authors.
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

	"go.chromium.org/luci/resultdb/internal/rootinvocations"
	"go.chromium.org/luci/resultdb/internal/testutil"
	"go.chromium.org/luci/resultdb/internal/testutil/insert"
	"go.chromium.org/luci/resultdb/rdbperms"
)

func TestWorkUnitAccessChecker(t *testing.T) {
	ftt.Run(`WorkUnitAccessChecker`, t, func(t *ftt.Test) {
		ctx := testutil.SpannerTestContext(t)

		rootInvID := rootinvocations.ID("root-inv")

		// Insert root invocation.
		var ms []*spanner.Mutation
		ms = append(ms, insert.RootInvocationWithRootWorkUnit(
			rootinvocations.NewBuilder(rootInvID).WithRealm("testproject:root").Build(),
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

		t.Run("NewWorkUnitAccessChecker", func(t *ftt.Test) {
			t.Run("root invocation not found", func(t *ftt.Test) {
				_, err := NewWorkUnitAccessChecker(ctx, rootinvocations.ID("not-exists"), opts)
				st, ok := appstatus.Get(err)
				assert.That(t, ok, should.BeTrue)
				assert.That(t, st.Code(), should.Equal(codes.NotFound))
				assert.That(t, st.Message(), should.ContainSubstring(`"rootInvocations/not-exists" not found`))
			})
			t.Run("no access on root invocation", func(t *ftt.Test) {
				authState.IdentityPermissions = nil

				checker, err := NewWorkUnitAccessChecker(ctx, rootInvID, opts)
				assert.Loosely(t, err, should.BeNil)
				assert.That(t, checker.RootInvovcationAccess, should.Equal(NoAccess))
			})
			t.Run("full access on root invocation", func(t *ftt.Test) {
				authState.IdentityPermissions = []authtest.RealmPermission{
					{Realm: "testproject:root", Permission: rdbperms.PermListTestResults},
					{Realm: "testproject:root", Permission: rdbperms.PermListTestExonerations},
					{Realm: "testproject:root", Permission: rdbperms.PermListLimitedTestResults},
					{Realm: "testproject:root", Permission: rdbperms.PermListLimitedTestExonerations},
				}

				checker, err := NewWorkUnitAccessChecker(ctx, rootInvID, opts)
				assert.Loosely(t, err, should.BeNil)
				assert.That(t, checker.RootInvovcationAccess, should.Equal(FullAccess))
			})
			t.Run("limited access on root invocation", func(t *ftt.Test) {
				authState.IdentityPermissions = []authtest.RealmPermission{
					{Realm: "testproject:root", Permission: rdbperms.PermListTestResults},
					{Realm: "testproject:root", Permission: rdbperms.PermListLimitedTestResults},
					{Realm: "testproject:root", Permission: rdbperms.PermListLimitedTestExonerations},
				}

				checker, err := NewWorkUnitAccessChecker(ctx, rootInvID, opts)
				assert.Loosely(t, err, should.BeNil)
				assert.That(t, checker.RootInvovcationAccess, should.Equal(LimitedAccess))
			})
		})

		t.Run("Check", func(t *ftt.Test) {
			t.Run("full access on root invocation", func(t *ftt.Test) {
				authState.IdentityPermissions = []authtest.RealmPermission{
					{Realm: "testproject:root", Permission: rdbperms.PermListTestResults},
					{Realm: "testproject:root", Permission: rdbperms.PermListTestExonerations},
					{Realm: "testproject:root", Permission: rdbperms.PermListLimitedTestResults},
					{Realm: "testproject:root", Permission: rdbperms.PermListLimitedTestExonerations},
				}
				checker, err := NewWorkUnitAccessChecker(ctx, rootInvID, opts)
				assert.Loosely(t, err, should.BeNil)

				level, err := checker.Check(ctx, "testproject:workunit")
				assert.Loosely(t, err, should.BeNil)
				assert.That(t, level, should.Equal(FullAccess))
			})
			t.Run("limited access on root invocation", func(t *ftt.Test) {
				authState.IdentityPermissions = []authtest.RealmPermission{
					{Realm: "testproject:root", Permission: rdbperms.PermListTestResults},
					{Realm: "testproject:root", Permission: rdbperms.PermListLimitedTestResults},
					{Realm: "testproject:root", Permission: rdbperms.PermListLimitedTestExonerations},
				}
				checker, err := NewWorkUnitAccessChecker(ctx, rootInvID, opts)
				assert.Loosely(t, err, should.BeNil)
				t.Run("no upgrade permission", func(t *ftt.Test) {
					level, err := checker.Check(ctx, "testproject:workunit")
					assert.Loosely(t, err, should.BeNil)
					assert.That(t, level, should.Equal(LimitedAccess))
				})
				t.Run("upgrade to full access", func(t *ftt.Test) {
					authState.IdentityPermissions = append(authState.IdentityPermissions,
						[]authtest.RealmPermission{
							{Realm: "testproject:workunit", Permission: rdbperms.PermGetTestResult},
							{Realm: "testproject:workunit", Permission: rdbperms.PermGetTestExoneration},
						}...)
					level, err := checker.Check(ctx, "testproject:workunit")
					assert.Loosely(t, err, should.BeNil)
					assert.That(t, level, should.Equal(FullAccess))
				})
			})
			t.Run("no access on root invocation", func(t *ftt.Test) {
				authState.IdentityPermissions = nil
				checker, err := NewWorkUnitAccessChecker(ctx, rootInvID, opts)
				assert.Loosely(t, err, should.BeNil)

				level, err := checker.Check(ctx, "testproject:workunit")
				assert.Loosely(t, err, should.BeNil)
				assert.That(t, level, should.Equal(NoAccess))
			})
		})
	})
}
