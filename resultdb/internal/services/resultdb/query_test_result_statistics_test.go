// Copyright 2020 The LUCI Authors.
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

package resultdb

import (
	"context"
	"testing"

	"go.chromium.org/luci/common/tsmon"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/resultdb/internal/invocations"
	"go.chromium.org/luci/resultdb/internal/resultcount"
	"go.chromium.org/luci/resultdb/internal/testutil"
	"go.chromium.org/luci/resultdb/internal/testutil/insert"
	pb "go.chromium.org/luci/resultdb/proto/v1"
	"go.chromium.org/luci/resultdb/rdbperms"

	. "go.chromium.org/luci/common/testing/assertions"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/convey"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestQueryTestResultStatistics(t *testing.T) {
	ftt.Run(`QueryTestResultStatistics`, t, func(t *ftt.Test) {
		ctx := auth.WithState(testutil.SpannerTestContext(t), &authtest.FakeState{
			Identity: "user:someone@example.com",
			IdentityPermissions: []authtest.RealmPermission{
				{Realm: "testproject:testrealm", Permission: rdbperms.PermListTestResults},
			},
		})
		ctx, _ = tsmon.WithDummyInMemory(ctx)

		incCount := func(invID invocations.ID, count int64) {
			_, err := span.ReadWriteTransaction(ctx, func(ctx context.Context) error {
				return resultcount.IncrementTestResultCount(ctx, invID, count)
			})
			assert.Loosely(t, err, should.BeNil)
		}

		testutil.MustApply(ctx, t,
			insert.Invocation(
				"x", pb.Invocation_FINALIZED, map[string]any{
					"Realm": "secretproject:testrealm",
				}),
			insert.Invocation(
				"a", pb.Invocation_FINALIZED, map[string]any{
					"Realm": "testproject:testrealm",
				}),
			insert.Invocation(
				"b", pb.Invocation_FINALIZED, map[string]any{
					"Realm": "otherproject:testrealm",
				}),
			insert.Invocation(
				"c", pb.Invocation_FINALIZED, map[string]any{
					"Realm": "otherproject:testrealm",
				}),
			// The invocation d doesn't have any included invocation.
			insert.Invocation(
				"d", pb.Invocation_FINALIZED, map[string]any{
					"Realm": "testproject:testrealm",
				}),
			insert.Inclusion("a", "b"),
			insert.Inclusion("a", "c"),
			// To test no double count if an invocation is included by multiple
			// invocations.
			insert.Inclusion("b", "d"),
			insert.Inclusion("c", "d"),
		)

		incCount(invocations.ID("b"), 5)
		incCount(invocations.ID("c"), 10)
		incCount(invocations.ID("d"), 20)

		srv := newTestResultDBService()

		t.Run(`Permission denied`, func(t *ftt.Test) {
			_, err := srv.QueryTestResultStatistics(ctx, &pb.QueryTestResultStatisticsRequest{
				Invocations: []string{"invocations/x"},
			})
			assert.Loosely(t, err, convey.Adapt(ShouldBeRPCPermissionDenied)(`caller does not have permission resultdb.testResults.list in realm of invocation x`))
		})

		t.Run(`Valid with included invocation`, func(t *ftt.Test) {
			res, err := srv.QueryTestResultStatistics(ctx, &pb.QueryTestResultStatisticsRequest{
				Invocations: []string{"invocations/a"},
			})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, res.TotalTestResults, should.Equal(35))
		})

		t.Run(`Valid without included invocation`, func(t *ftt.Test) {
			res, err := srv.QueryTestResultStatistics(ctx, &pb.QueryTestResultStatisticsRequest{
				Invocations: []string{"invocations/d"},
			})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, res.TotalTestResults, should.Equal(20))
		})

		t.Run(`Valid with missing included invocation`, func(t *ftt.Test) {
			testutil.MustApply(
				ctx, t,
				// The invocation missinginv is missing in Invocations table.
				insert.Inclusion("a", "missinginv"),
			)
			res, err := srv.QueryTestResultStatistics(ctx, &pb.QueryTestResultStatisticsRequest{
				Invocations: []string{"invocations/a"},
			})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, res.TotalTestResults, should.Equal(35))
		})
	})
}
