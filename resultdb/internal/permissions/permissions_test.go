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
	"context"
	"testing"

	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/grpc/appstatus"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/resultdb/internal/invocations"
	"go.chromium.org/luci/resultdb/internal/testutil"
	"go.chromium.org/luci/resultdb/internal/testutil/insert"
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
			assert.Loosely(t, err, should.ErrLike("resultdb.artifacts.list in realm of invocation i0"))

			ids = invocations.NewIDSet(invocations.ID("i1"), invocations.ID("i2"))
			err = VerifyInvocations(span.Single(ctx), ids, rdbperms.PermListArtifacts, rdbperms.PermListTestExonerations)
			assert.Loosely(t, appstatus.Code(err), should.Equal(codes.PermissionDenied))
			assert.Loosely(t, err, should.ErrLike("resultdb.testExonerations.list in realm of invocation i1"))

			ids = invocations.NewIDSet(invocations.ID("i2"), invocations.ID("i3"))
			err = VerifyInvocations(span.Single(ctx), ids, rdbperms.PermListTestExonerations, rdbperms.PermListTestResults, rdbperms.PermListArtifacts)
			assert.Loosely(t, appstatus.Code(err), should.Equal(codes.PermissionDenied))
			assert.Loosely(t, err, should.ErrLike("resultdb.artifacts.list in realm of invocation i3"))
		})
		t.Run("Duplicate invocations", func(t *ftt.Test) {
			ids := invocations.NewIDSet(invocations.ID("i2"), invocations.ID("i3"), invocations.ID("i3"))
			err := VerifyInvocations(span.Single(ctx), ids, rdbperms.PermListTestExonerations, rdbperms.PermListTestResults)
			assert.Loosely(t, err, should.BeNil)

			ids = invocations.NewIDSet(invocations.ID("i2"), invocations.ID("i3"), invocations.ID("i3"))
			err = VerifyInvocations(span.Single(ctx), ids, rdbperms.PermListTestExonerations, rdbperms.PermListTestResults, rdbperms.PermListArtifacts)
			assert.Loosely(t, appstatus.Code(err), should.Equal(codes.PermissionDenied))
			assert.Loosely(t, err, should.ErrLike("resultdb.artifacts.list in realm of invocation i3"))
		})
		t.Run("Duplicate realms", func(t *ftt.Test) {
			ids := invocations.NewIDSet(invocations.ID("i2"), invocations.ID("i3"), invocations.ID("i3b"))
			err := VerifyInvocations(span.Single(ctx), ids, rdbperms.PermListTestExonerations, rdbperms.PermListTestResults)
			assert.Loosely(t, err, should.BeNil)

			ids = invocations.NewIDSet(invocations.ID("i2"), invocations.ID("i3"), invocations.ID("i3b"))
			err = VerifyInvocations(span.Single(ctx), ids, rdbperms.PermListTestExonerations, rdbperms.PermListTestResults, rdbperms.PermListArtifacts)
			assert.Loosely(t, appstatus.Code(err), should.Equal(codes.PermissionDenied))
			assert.Loosely(t, err, should.ErrLike("resultdb.artifacts.list in realm of invocation i3"))
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

func TestHasPermissionsInRealms(t *testing.T) {
	ftt.Run("HasPermissionsInRealms", t, func(t *ftt.Test) {
		ctx := auth.WithState(context.Background(), &authtest.FakeState{
			Identity: "user:someone@example.com",
			IdentityPermissions: []authtest.RealmPermission{
				{Realm: "testproject:r1", Permission: rdbperms.PermListArtifacts},
				{Realm: "testproject:r1", Permission: rdbperms.PermListTestExonerations},
				{Realm: "testproject:r1", Permission: rdbperms.PermListTestResults},
				{Realm: "testproject:r2", Permission: rdbperms.PermListLimitedTestExonerations},
				{Realm: "testproject:r2", Permission: rdbperms.PermListLimitedTestResults},
				{Realm: "testproject:r2", Permission: rdbperms.PermListTestExonerations},
				{Realm: "testproject:r2", Permission: rdbperms.PermListTestResults},
			},
		})

		t.Run("Missing permissions", func(t *ftt.Test) {
			// Case: user has no permissions in one of the realms
			realms := map[invocations.ID]string{
				"i0": "testproject:r0",
			}
			verified, desc, err := HasPermissionsInRealms(ctx, realms,
				rdbperms.PermListTestResults)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, verified, should.Equal(false))
			assert.Loosely(t, desc, should.ContainSubstring("resultdb.testResults.list in realm of invocation i0"))

			// Case: user has some permissions in all realms, but not the specified
			//       permissions for all realms
			realms = map[invocations.ID]string{
				"i1":  "testproject:r1",
				"i2":  "testproject:r2",
				"i2b": "testproject:r2",
			}
			verified, desc, err = HasPermissionsInRealms(ctx, realms,
				rdbperms.PermListTestResults, rdbperms.PermListLimitedTestResults)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, verified, should.Equal(false))
			assert.Loosely(t, desc, should.ContainSubstring("resultdb.testResults.listLimited in realm of invocation i1"))
		})
		t.Run("All permissions present", func(t *ftt.Test) {
			realms := map[invocations.ID]string{
				"i1":  "testproject:r1",
				"i2":  "testproject:r2",
				"i2b": "testproject:r2",
			}
			verified, desc, err := HasPermissionsInRealms(ctx, realms,
				rdbperms.PermListTestResults, rdbperms.PermListTestExonerations)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, verified, should.Equal(true))
			assert.Loosely(t, desc, should.BeEmpty)
		})
		t.Run("Empty realms", func(t *ftt.Test) {
			realms := map[invocations.ID]string{}
			verified, desc, err := HasPermissionsInRealms(ctx, realms,
				rdbperms.PermListTestResults)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, verified, should.Equal(true))
			assert.Loosely(t, desc, should.BeEmpty)
		})
		t.Run("No permissions specified", func(t *ftt.Test) {
			realms := map[invocations.ID]string{
				"i1":  "testproject:r1",
				"i2":  "testproject:r2",
				"i2b": "testproject:r2",
			}
			verified, desc, err := HasPermissionsInRealms(ctx, realms)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, verified, should.Equal(true))
			assert.Loosely(t, desc, should.BeEmpty)
		})
	})
}
