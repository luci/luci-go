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
	"context"
	"testing"

	"cloud.google.com/go/spanner"
	"google.golang.org/grpc/codes"
	durpb "google.golang.org/protobuf/types/known/durationpb"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/grpc/grpcutil/testing/grpccode"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"

	"go.chromium.org/luci/resultdb/internal/invocations"
	"go.chromium.org/luci/resultdb/internal/spanutil"
	"go.chromium.org/luci/resultdb/internal/testutil"
	"go.chromium.org/luci/resultdb/internal/testutil/insert"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
	"go.chromium.org/luci/resultdb/rdbperms"
)

func TestValidateGetTestResultRequest(t *testing.T) {
	t.Parallel()
	ftt.Run(`ValidateGetTestResultRequest`, t, func(t *ftt.Test) {
		t.Run(`Valid`, func(t *ftt.Test) {
			req := &pb.GetTestResultRequest{Name: "invocations/a/tests/:%2F%2Finfra%2Fjunit_tests%21junit:org.chromium.go.luci:ValidationTests%23FooBar/results/result5"}
			assert.Loosely(t, validateGetTestResultRequest(req), should.BeNil)
		})

		t.Run(`Invalid name`, func(t *ftt.Test) {
			t.Run(`, missing`, func(t *ftt.Test) {
				req := &pb.GetTestResultRequest{}
				assert.Loosely(t, validateGetTestResultRequest(req), should.ErrLike("unspecified"))
			})

			t.Run(`, invalid format`, func(t *ftt.Test) {
				req := &pb.GetTestResultRequest{Name: "bad_name"}
				assert.Loosely(t, validateGetTestResultRequest(req), should.ErrLike("does not match"))
			})
		})
	})
}

func TestGetTestResult(t *testing.T) {
	ftt.Run(`GetTestResult`, t, func(t *ftt.Test) {
		ctx := auth.WithState(testutil.SpannerTestContext(t), &authtest.FakeState{
			Identity: "user:someone@example.com",
			IdentityPermissions: []authtest.RealmPermission{
				{Realm: "testproject:testrealm", Permission: rdbperms.PermGetTestResult},
			},
		})

		srv := newTestResultDBService()
		test := func(ctx context.Context, name string, expected *pb.TestResult) {
			req := &pb.GetTestResultRequest{Name: name}
			tr, err := srv.GetTestResult(ctx, req)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, tr, should.Match(expected))
		}

		invID := invocations.ID("inv_0")
		// Insert a TestResult.
		testutil.MustApply(ctx, t,
			insert.Invocation("inv_0", pb.Invocation_ACTIVE, map[string]any{"Realm": "testproject:testrealm"}),
			spanutil.InsertMap("TestResults", map[string]any{
				"InvocationId":    invID,
				"TestId":          "://infra/junit_tests!junit:org.chromium.go.luci:ValidationTests#FooBar",
				"ResultId":        "result_id_within_inv_0",
				"Variant":         pbutil.Variant("k1", "v1", "k2", "v2"),
				"VariantHash":     "deadbeef",
				"CommitTimestamp": spanner.CommitTimestamp,
				"IsUnexpected":    true,
				"Status":          pb.TestStatus_FAIL,
				"RunDurationUsec": 1234567,
			}))

		// Fetch back the TestResult.
		expected := &pb.TestResult{
			Name:   "invocations/inv_0/tests/:%2F%2Finfra%2Fjunit_tests%21junit:org.chromium.go.luci:ValidationTests%23FooBar/results/result_id_within_inv_0",
			TestId: "://infra/junit_tests!junit:org.chromium.go.luci:ValidationTests#FooBar",
			TestVariantId: &pb.TestVariantIdentifier{
				ModuleName:        "//infra/junit_tests",
				ModuleScheme:      "junit",
				ModuleVariant:     pbutil.Variant("k1", "v1", "k2", "v2"),
				ModuleVariantHash: "68d82cb978092fc7",
				CoarseName:        "org.chromium.go.luci",
				FineName:          "ValidationTests",
				CaseName:          "FooBar",
			},
			ResultId:    "result_id_within_inv_0",
			Variant:     pbutil.Variant("k1", "v1", "k2", "v2"),
			VariantHash: "deadbeef",
			Expected:    false,
			Status:      pb.TestStatus_FAIL,
			Duration:    &durpb.Duration{Seconds: 1, Nanos: 234567000},
		}
		test(ctx, "invocations/inv_0/tests/:%2F%2Finfra%2Fjunit_tests%21junit:org.chromium.go.luci:ValidationTests%23FooBar/results/result_id_within_inv_0", expected)

		t.Run(`permission denied`, func(t *ftt.Test) {
			testutil.MustApply(ctx, t,
				insert.Invocation("inv_s", pb.Invocation_ACTIVE, map[string]any{"Realm": "secretproject:testrealm"}))
			req := &pb.GetTestResultRequest{Name: "invocations/inv_s/tests/ninja:%2F%2Fchrome%2Ftest:foo_tests%2FBarTest.DoBaz/results/result_id_within_inv_s"}
			tr, err := srv.GetTestResult(ctx, req)
			assert.Loosely(t, tr, should.BeNil)
			assert.Loosely(t, err, grpccode.ShouldBe(codes.PermissionDenied))
			assert.Loosely(t, err, should.ErrLike("caller does not have permission resultdb.testResults.get in realm of invocation inv_s"))
		})

		t.Run(`works with expected result`, func(t *ftt.Test) {
			testutil.MustApply(ctx, t, spanutil.InsertMap("TestResults", map[string]any{
				"InvocationId":    invID,
				"TestId":          "ninja://chrome/test:foo_tests/BarTest.DoBaz",
				"ResultId":        "result_id_within_inv_1",
				"Variant":         pbutil.Variant("k1", "v1", "k2", "v2"),
				"VariantHash":     "deadbeef",
				"CommitTimestamp": spanner.CommitTimestamp,
				"Status":          pb.TestStatus_PASS,
				"RunDurationUsec": 1534567,
			}))

			expected := &pb.TestResult{
				Name:   "invocations/inv_0/tests/ninja:%2F%2Fchrome%2Ftest:foo_tests%2FBarTest.DoBaz/results/result_id_within_inv_1",
				TestId: "ninja://chrome/test:foo_tests/BarTest.DoBaz",
				TestVariantId: &pb.TestVariantIdentifier{
					ModuleName:        "legacy",
					ModuleScheme:      "legacy",
					ModuleVariant:     pbutil.Variant("k1", "v1", "k2", "v2"),
					ModuleVariantHash: "68d82cb978092fc7",
					CoarseName:        "",
					FineName:          "",
					CaseName:          "ninja://chrome/test:foo_tests/BarTest.DoBaz",
				},
				ResultId:    "result_id_within_inv_1",
				Variant:     pbutil.Variant("k1", "v1", "k2", "v2"),
				VariantHash: "deadbeef",
				Expected:    true,
				Status:      pb.TestStatus_PASS,
				Duration:    &durpb.Duration{Seconds: 1, Nanos: 534567000},
			}

			// Fetch back the TestResult.
			test(ctx, "invocations/inv_0/tests/ninja:%2F%2Fchrome%2Ftest:foo_tests%2FBarTest.DoBaz/results/result_id_within_inv_1", expected)
		})
	})
}
