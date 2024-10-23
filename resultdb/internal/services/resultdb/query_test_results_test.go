// Copyright 2019 The LUCI Authors.
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
	"sort"
	"testing"

	"google.golang.org/genproto/protobuf/field_mask"

	"go.chromium.org/luci/common/tsmon"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"

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

func TestValidateQueryRequest(t *testing.T) {
	t.Parallel()
	ftt.Run(`TestValidateQueryRequest`, t, func(t *ftt.Test) {
		t.Run(`no invocations`, func(t *ftt.Test) {
			err := validateQueryRequest(&pb.QueryTestResultsRequest{})
			assert.Loosely(t, err, should.ErrLike(`invocations: unspecified`))
		})

		t.Run(`invalid invocation`, func(t *ftt.Test) {
			err := validateQueryRequest(&pb.QueryTestResultsRequest{
				Invocations: []string{"x"},
			})
			assert.Loosely(t, err, should.ErrLike(`invocations: "x": does not match`))
		})

		t.Run(`invalid page size`, func(t *ftt.Test) {
			err := validateQueryRequest(&pb.QueryTestResultsRequest{
				Invocations: []string{"invocations/x"},
				PageSize:    -1,
			})
			assert.Loosely(t, err, should.ErrLike(`page_size: negative`))
		})
	})
}

func TestValidateQueryTestResultsRequest(t *testing.T) {
	t.Parallel()
	ftt.Run(`Valid`, t, func(t *ftt.Test) {
		err := validateQueryTestResultsRequest(&pb.QueryTestResultsRequest{
			Invocations: []string{"invocations/x"},
			PageSize:    50,
		})
		assert.Loosely(t, err, should.BeNil)
	})

	ftt.Run(`invalid invocation`, t, func(t *ftt.Test) {
		err := validateQueryTestResultsRequest(&pb.QueryTestResultsRequest{
			Invocations: []string{"x"},
		})
		assert.Loosely(t, err, should.ErrLike(`invocations: "x": does not match`))
	})
}

func TestQueryTestResults(t *testing.T) {
	ftt.Run(`QueryTestResults`, t, func(t *ftt.Test) {
		ctx := auth.WithState(testutil.SpannerTestContext(t), &authtest.FakeState{
			Identity: "user:someone@example.com",
			IdentityPermissions: []authtest.RealmPermission{
				{Realm: "testproject:testrealm", Permission: rdbperms.PermListTestResults},
			},
		})
		ctx, _ = tsmon.WithDummyInMemory(ctx)

		insertInv := insert.FinalizedInvocationWithInclusions
		insertTRs := insert.TestResults
		testutil.MustApply(ctx, t, testutil.CombineMutations(
			insertInv("x", map[string]any{"Realm": "secretproject:testrealm"}, "a"),
			insertInv("a", map[string]any{"Realm": "testproject:testrealm"}, "b"),
			insertInv("b", map[string]any{"Realm": "otherproject:testrealm"}, "c"),
			// The invocation c doesn't have any included invocation.
			insertInv("c", map[string]any{"Realm": "testproject:testrealm"}),
			insertTRs(t, "a", "A", nil, pb.TestStatus_FAIL, pb.TestStatus_PASS),
			insertTRs(t, "b", "B", nil, pb.TestStatus_CRASH, pb.TestStatus_PASS),
			insertTRs(t, "c", "C", nil, pb.TestStatus_PASS),
		)...)

		srv := newTestResultDBService()

		t.Run(`Without readMask`, func(t *ftt.Test) {
			res, err := srv.QueryTestResults(ctx, &pb.QueryTestResultsRequest{
				Invocations: []string{"invocations/a"},
			})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, res.TestResults, should.HaveLength(5))
		})

		t.Run(`Permission denied`, func(t *ftt.Test) {
			_, err := srv.QueryTestResults(ctx, &pb.QueryTestResultsRequest{
				Invocations: []string{"invocations/x"},
			})

			assert.Loosely(t, err, convey.Adapt(ShouldBeRPCPermissionDenied)(`caller does not have permission resultdb.testResults.list in realm of invocation x`))
		})

		t.Run(`Valid with included invocation`, func(t *ftt.Test) {
			res, err := srv.QueryTestResults(ctx, &pb.QueryTestResultsRequest{
				Invocations: []string{"invocations/a"},
			})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, res.TestResults, should.HaveLength(5))

			sort.Slice(res.TestResults, func(i, j int) bool {
				return res.TestResults[i].Name < res.TestResults[j].Name
			})

			assert.Loosely(t, res.TestResults[0].Name, should.Equal("invocations/a/tests/A/results/0"))
			assert.Loosely(t, res.TestResults[0].Status, should.Equal(pb.TestStatus_FAIL))
			assert.Loosely(t, res.TestResults[0].SummaryHtml, should.BeEmpty)

			assert.Loosely(t, res.TestResults[1].Name, should.Equal("invocations/a/tests/A/results/1"))
			assert.Loosely(t, res.TestResults[1].Status, should.Equal(pb.TestStatus_PASS))

			assert.Loosely(t, res.TestResults[2].Name, should.Equal("invocations/b/tests/B/results/0"))
			assert.Loosely(t, res.TestResults[2].Status, should.Equal(pb.TestStatus_CRASH))

			assert.Loosely(t, res.TestResults[3].Name, should.Equal("invocations/b/tests/B/results/1"))
			assert.Loosely(t, res.TestResults[3].Status, should.Equal(pb.TestStatus_PASS))

			assert.Loosely(t, res.TestResults[4].Name, should.Equal("invocations/c/tests/C/results/0"))
			assert.Loosely(t, res.TestResults[4].Status, should.Equal(pb.TestStatus_PASS))
		})

		t.Run(`Valid without included invocation`, func(t *ftt.Test) {
			res, err := srv.QueryTestResults(ctx, &pb.QueryTestResultsRequest{
				Invocations: []string{"invocations/c"},
			})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, res.TestResults, should.HaveLength(1))

			sort.Slice(res.TestResults, func(i, j int) bool {
				return res.TestResults[i].Name < res.TestResults[j].Name
			})

			assert.Loosely(t, res.TestResults[0].Name, should.Equal("invocations/c/tests/C/results/0"))
			assert.Loosely(t, res.TestResults[0].Status, should.Equal(pb.TestStatus_PASS))
		})

		t.Run(`Valid with missing included invocation`, func(t *ftt.Test) {
			testutil.MustApply(
				ctx, t,
				// The invocation missinginv is missing in Invocations table.
				insert.Inclusion("a", "missinginv"),
			)
			res, err := srv.QueryTestResults(ctx, &pb.QueryTestResultsRequest{
				Invocations: []string{"invocations/a"},
			})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, res.TestResults, should.HaveLength(5))

			sort.Slice(res.TestResults, func(i, j int) bool {
				return res.TestResults[i].Name < res.TestResults[j].Name
			})

			assert.Loosely(t, res.TestResults[0].Name, should.Equal("invocations/a/tests/A/results/0"))
			assert.Loosely(t, res.TestResults[0].Status, should.Equal(pb.TestStatus_FAIL))
			assert.Loosely(t, res.TestResults[0].SummaryHtml, should.BeEmpty)

			assert.Loosely(t, res.TestResults[1].Name, should.Equal("invocations/a/tests/A/results/1"))
			assert.Loosely(t, res.TestResults[1].Status, should.Equal(pb.TestStatus_PASS))

			assert.Loosely(t, res.TestResults[2].Name, should.Equal("invocations/b/tests/B/results/0"))
			assert.Loosely(t, res.TestResults[2].Status, should.Equal(pb.TestStatus_CRASH))

			assert.Loosely(t, res.TestResults[3].Name, should.Equal("invocations/b/tests/B/results/1"))
			assert.Loosely(t, res.TestResults[3].Status, should.Equal(pb.TestStatus_PASS))

			assert.Loosely(t, res.TestResults[4].Name, should.Equal("invocations/c/tests/C/results/0"))
			assert.Loosely(t, res.TestResults[4].Status, should.Equal(pb.TestStatus_PASS))
		})

		t.Run(`With readMask`, func(t *ftt.Test) {
			res, err := srv.QueryTestResults(ctx, &pb.QueryTestResultsRequest{
				Invocations: []string{"invocations/a"},
				PageSize:    1,
				ReadMask: &field_mask.FieldMask{
					Paths: []string{
						"status",
						"summary_html",
					}},
			})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, res.TestResults[0].Name, should.Equal("invocations/c/tests/C/results/0"))
			assert.Loosely(t, res.TestResults[0].TestId, should.BeEmpty)
			assert.Loosely(t, res.TestResults[0].SummaryHtml, should.Equal("SummaryHtml"))
		})
	})
}
