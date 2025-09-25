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
	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/common/tsmon"
	"go.chromium.org/luci/grpc/grpcutil/testing/grpccode"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"

	"go.chromium.org/luci/resultdb/internal/testutil"
	"go.chromium.org/luci/resultdb/internal/testutil/insert"
	pb "go.chromium.org/luci/resultdb/proto/v1"
	"go.chromium.org/luci/resultdb/rdbperms"
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

		testutil.MustApply(ctx, t, testutil.CombineMutations(
			insert.FinalizedInvocationWithInclusions("x", map[string]any{"Realm": "secretproject:testrealm"}, "a"),
			insert.FinalizedInvocationWithInclusions("a", map[string]any{"Realm": "testproject:testrealm"}, "b"),
			insert.FinalizedInvocationWithInclusions("b", map[string]any{"Realm": "otherproject:testrealm"}, "c"),
			// The invocation c doesn't have any included invocation.
			insert.FinalizedInvocationWithInclusions("c", map[string]any{"Realm": "testproject:testrealm"}),
			insert.TestResults(t, "a", ":module!gtest::Class#A", nil, pb.TestResult_FAILED, pb.TestResult_PASSED),
			insert.TestResultsLegacy(t, "b", ":module!gtest::Class#B", nil, pb.TestStatus_CRASH, pb.TestStatus_PASS),
			insert.TestResults(t, "c", ":module!gtest::Class#C", nil, pb.TestResult_PASSED),
		)...)

		srv := newTestResultDBService()

		t.Run(`Without readMask`, func(t *ftt.Test) {
			res, err := srv.QueryTestResults(ctx, &pb.QueryTestResultsRequest{
				Invocations: []string{"invocations/a"},
			})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, res.TestResults, should.HaveLength(5))
			for _, r := range res.TestResults {
				// By default, some large fields are omitted as per the API documentation.
				assert.Loosely(t, r.SummaryHtml, should.BeEmpty)
				assert.Loosely(t, r.Properties, should.BeNil)
				assert.Loosely(t, r.Tags, should.BeNil)
			}
		})

		t.Run(`Permission denied`, func(t *ftt.Test) {
			_, err := srv.QueryTestResults(ctx, &pb.QueryTestResultsRequest{
				Invocations: []string{"invocations/x"},
			})

			assert.Loosely(t, err, grpccode.ShouldBe(codes.PermissionDenied))
			assert.Loosely(t, err, should.ErrLike(`caller does not have permission resultdb.testResults.list in realm of "invocations/x"`))
		})

		t.Run(`Valid with included invocation`, func(t *ftt.Test) {
			res, err := srv.QueryTestResults(ctx, &pb.QueryTestResultsRequest{
				Invocations: []string{"invocations/a"},
				ReadMask: &field_mask.FieldMask{
					// Query all fields to make comparison with expected results simpler.
					Paths: []string{"*"},
				},
			})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, res.TestResults, should.HaveLength(5))

			sort.Slice(res.TestResults, func(i, j int) bool {
				return res.TestResults[i].Name < res.TestResults[j].Name
			})

			var expectedResults []*pb.TestResult
			expectedResults = append(expectedResults, insert.MakeTestResults("a", ":module!gtest::Class#A", &pb.Variant{}, pb.TestResult_FAILED, pb.TestResult_PASSED)...)
			expectedResults = append(expectedResults, insert.MakeTestResultsLegacy("b", ":module!gtest::Class#B", &pb.Variant{}, pb.TestStatus_CRASH, pb.TestStatus_PASS)...)
			expectedResults = append(expectedResults, insert.MakeTestResults("c", ":module!gtest::Class#C", &pb.Variant{}, pb.TestResult_PASSED)...)
			assert.Loosely(t, res.TestResults, should.Match(expectedResults))
		})

		t.Run(`Valid without included invocation`, func(t *ftt.Test) {
			res, err := srv.QueryTestResults(ctx, &pb.QueryTestResultsRequest{
				Invocations: []string{"invocations/c"},
				ReadMask: &field_mask.FieldMask{
					// Query all fields to make comparison with expected results simpler.
					Paths: []string{"*"},
				},
			})

			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, res.TestResults, should.HaveLength(1))

			assert.Loosely(t, res.TestResults[0].Name, should.Equal("invocations/c/tests/:module%21gtest::Class%23C/results/0"))
			expectedResults := insert.MakeTestResults("c", ":module!gtest::Class#C", &pb.Variant{}, pb.TestResult_PASSED)
			assert.Loosely(t, res.TestResults, should.Match(expectedResults))
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

			assert.Loosely(t, res.TestResults[0].Name, should.Equal("invocations/a/tests/:module%21gtest::Class%23A/results/0"))
			assert.Loosely(t, res.TestResults[0].Status, should.Equal(pb.TestStatus_FAIL))
			assert.Loosely(t, res.TestResults[0].SummaryHtml, should.BeEmpty)

			assert.Loosely(t, res.TestResults[1].Name, should.Equal("invocations/a/tests/:module%21gtest::Class%23A/results/1"))
			assert.Loosely(t, res.TestResults[1].Status, should.Equal(pb.TestStatus_PASS))

			assert.Loosely(t, res.TestResults[2].Name, should.Equal("invocations/b/tests/:module%21gtest::Class%23B/results/0"))
			assert.Loosely(t, res.TestResults[2].Status, should.Equal(pb.TestStatus_CRASH))

			assert.Loosely(t, res.TestResults[3].Name, should.Equal("invocations/b/tests/:module%21gtest::Class%23B/results/1"))
			assert.Loosely(t, res.TestResults[3].Status, should.Equal(pb.TestStatus_PASS))

			assert.Loosely(t, res.TestResults[4].Name, should.Equal("invocations/c/tests/:module%21gtest::Class%23C/results/0"))
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
			assert.Loosely(t, res.TestResults[0].Name, should.Equal("invocations/c/tests/:module%21gtest::Class%23C/results/0"))
			assert.Loosely(t, res.TestResults[0].TestId, should.BeEmpty)
			assert.Loosely(t, res.TestResults[0].SummaryHtml, should.Equal("SummaryHtml"))
		})
	})
}
