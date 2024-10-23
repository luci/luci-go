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

package resultdb

import (
	"fmt"
	"testing"

	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"

	"go.chromium.org/luci/resultdb/internal/invocations/graph"
	"go.chromium.org/luci/resultdb/internal/spanutil"
	"go.chromium.org/luci/resultdb/internal/testutil"
	"go.chromium.org/luci/resultdb/internal/testutil/insert"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
	"go.chromium.org/luci/resultdb/rdbperms"

	. "go.chromium.org/luci/common/testing/assertions"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/convey"
	"go.chromium.org/luci/common/testing/truth/should"
)

func variantHash(pairs ...string) string {
	return pbutil.VariantHash(pbutil.Variant(pairs...))
}

func tvStrings(tvs []*pb.TestVariant) []string {
	tvStrings := make([]string, len(tvs))
	for i, tv := range tvs {
		tvStrings[i] = fmt.Sprintf("%d/%s/%s", int32(tv.Status), tv.TestId, tv.VariantHash)
	}
	return tvStrings
}

func TestBatchGetTestVariants(t *testing.T) {
	ftt.Run(`BatchGetTestVariants`, t, func(t *ftt.Test) {
		ctx := auth.WithState(testutil.SpannerTestContext(t), &authtest.FakeState{
			Identity: "user:someone@example.com",
			IdentityPermissions: []authtest.RealmPermission{
				{Realm: "testproject:testrealm", Permission: rdbperms.PermListTestResults},
				{Realm: "testproject:testrealm", Permission: rdbperms.PermListTestExonerations},
			},
		})

		testutil.MustApply(
			ctx, t,
			insert.InvocationWithInclusions("i0", pb.Invocation_ACTIVE, map[string]any{
				"Realm":   "testproject:testrealm",
				"Sources": spanutil.Compress(pbutil.MustMarshal(testutil.TestSources())),
			}, "i0")...,
		)
		testutil.MustApply(
			ctx, t,
			insert.Invocation("i1", pb.Invocation_ACTIVE, map[string]any{
				"Realm": "testproject:testrealm",
			}),
		)
		testutil.MustApply(ctx, t, testutil.CombineMutations(
			insert.TestResults(t, "i0", "test1", pbutil.Variant("a", "b"), pb.TestStatus_PASS),
			insert.TestResults(t, "i0", "test2", pbutil.Variant("c", "d"), pb.TestStatus_PASS),
			insert.TestResults(t, "i0", "test3", pbutil.Variant("a", "b"), pb.TestStatus_FAIL),
			insert.TestResults(t, "i0", "test4", pbutil.Variant("g", "h"), pb.TestStatus_SKIP),
			insert.TestResults(t, "i1", "test1", pbutil.Variant("e", "f"), pb.TestStatus_PASS),
			insert.TestResults(t, "i1", "test3", pbutil.Variant("c", "d"), pb.TestStatus_PASS),
		)...)

		srv := newTestResultDBService()

		t.Run(`Access denied`, func(t *ftt.Test) {
			req := &pb.BatchGetTestVariantsRequest{
				Invocation: "invocations/i0",
				TestVariants: []*pb.BatchGetTestVariantsRequest_TestVariantIdentifier{
					{TestId: "test1", VariantHash: variantHash("a", "b")},
				},
			}

			// Verify missing ListTestResults permission results in an error.
			ctx = auth.WithState(ctx, &authtest.FakeState{
				Identity: "user:someone@example.com",
				IdentityPermissions: []authtest.RealmPermission{
					{Realm: "testproject:testrealm", Permission: rdbperms.PermListTestExonerations},
				},
			})
			_, err := srv.BatchGetTestVariants(ctx, req)
			assert.Loosely(t, err, convey.Adapt(ShouldBeRPCPermissionDenied)("resultdb.testResults.list"))

			// Verify missing ListTestExonerations permission results in an error.
			ctx := auth.WithState(ctx, &authtest.FakeState{
				Identity: "user:someone@example.com",
				IdentityPermissions: []authtest.RealmPermission{
					{Realm: "testproject:testrealm", Permission: rdbperms.PermListTestResults},
				},
			})
			_, err = srv.BatchGetTestVariants(ctx, req)
			assert.Loosely(t, err, convey.Adapt(ShouldBeRPCPermissionDenied)("resultdb.testExonerations.list"))
		})
		t.Run(`Valid request with included invocation`, func(t *ftt.Test) {
			res, err := srv.BatchGetTestVariants(ctx, &pb.BatchGetTestVariantsRequest{
				Invocation: "invocations/i0",
				TestVariants: []*pb.BatchGetTestVariantsRequest_TestVariantIdentifier{
					{TestId: "test1", VariantHash: variantHash("a", "b")},
					{TestId: "test3", VariantHash: variantHash("a", "b")},
					{TestId: "test4", VariantHash: variantHash("g", "h")},
				},
			})
			assert.Loosely(t, err, should.BeNil)

			// NOTE: The order isn't important here, we just don't have a
			// matcher that does an unordered comparison.
			assert.Loosely(t, tvStrings(res.TestVariants), should.Resemble([]string{
				fmt.Sprintf("10/test3/%s", variantHash("a", "b")),
				fmt.Sprintf("20/test4/%s", variantHash("g", "h")),
				fmt.Sprintf("50/test1/%s", variantHash("a", "b")),
			}))

			for _, tv := range res.TestVariants {
				assert.Loosely(t, tv.IsMasked, should.BeFalse)
				assert.Loosely(t, tv.SourcesId, should.Equal(graph.HashSources(testutil.TestSources()).String()))
			}
			assert.Loosely(t, res.Sources, should.HaveLength(1))
			assert.Loosely(t, res.Sources[graph.HashSources(testutil.TestSources()).String()], should.Resemble(testutil.TestSources()))
		})

		t.Run(`Valid request without included invocation`, func(t *ftt.Test) {
			res, err := srv.BatchGetTestVariants(ctx, &pb.BatchGetTestVariantsRequest{
				Invocation: "invocations/i1",
				TestVariants: []*pb.BatchGetTestVariantsRequest_TestVariantIdentifier{
					{TestId: "test1", VariantHash: variantHash("e", "f")},
					{TestId: "test3", VariantHash: variantHash("c", "d")},
				},
			})
			assert.Loosely(t, err, should.BeNil)

			// NOTE: The order isn't important here, we just don't have a
			// matcher that does an unordered comparison.
			assert.Loosely(t, tvStrings(res.TestVariants), should.Resemble([]string{
				fmt.Sprintf("50/test1/%s", variantHash("e", "f")),
				fmt.Sprintf("50/test3/%s", variantHash("c", "d")),
			}))

			for _, tv := range res.TestVariants {
				assert.Loosely(t, tv.IsMasked, should.BeFalse)
				assert.Loosely(t, tv.SourcesId, should.BeEmpty)
			}

			assert.Loosely(t, res.Sources, should.HaveLength(0))
		})

		t.Run(`Valid request with missing included invocation`, func(t *ftt.Test) {
			testutil.MustApply(
				ctx, t,
				// The invocation missinginv is missing in Invocations table.
				insert.Inclusion("i0", "missinginv"),
			)
			res, err := srv.BatchGetTestVariants(ctx, &pb.BatchGetTestVariantsRequest{
				Invocation: "invocations/i0",
				TestVariants: []*pb.BatchGetTestVariantsRequest_TestVariantIdentifier{
					{TestId: "test1", VariantHash: variantHash("a", "b")},
					{TestId: "test3", VariantHash: variantHash("a", "b")},
					{TestId: "test4", VariantHash: variantHash("g", "h")},
				},
			})
			assert.Loosely(t, err, should.BeNil)

			// NOTE: The order isn't important here, we just don't have a
			// matcher that does an unordered comparison.
			assert.Loosely(t, tvStrings(res.TestVariants), should.Resemble([]string{
				fmt.Sprintf("10/test3/%s", variantHash("a", "b")),
				fmt.Sprintf("20/test4/%s", variantHash("g", "h")),
				fmt.Sprintf("50/test1/%s", variantHash("a", "b")),
			}))

			for _, tv := range res.TestVariants {
				assert.Loosely(t, tv.IsMasked, should.BeFalse)
				assert.Loosely(t, tv.SourcesId, should.Equal(graph.HashSources(testutil.TestSources()).String()))
			}
			assert.Loosely(t, res.Sources, should.HaveLength(1))
			assert.Loosely(t, res.Sources[graph.HashSources(testutil.TestSources()).String()], should.Resemble(testutil.TestSources()))
		})

		t.Run(`Requesting > 500 variants fails`, func(t *ftt.Test) {
			req := pb.BatchGetTestVariantsRequest{
				Invocation:   "invocations/i0",
				TestVariants: make([]*pb.BatchGetTestVariantsRequest_TestVariantIdentifier, 501),
			}
			for i := 0; i < 500; i += 1 {
				req.TestVariants[i] = &pb.BatchGetTestVariantsRequest_TestVariantIdentifier{
					TestId:      "test1",
					VariantHash: variantHash("a", "b"),
				}
			}

			_, err := srv.BatchGetTestVariants(ctx, &req)
			assert.Loosely(t, err, convey.Adapt(ShouldBeRPCInvalidArgument)("a maximum of 500 test variants can be requested at once"))
		})

		t.Run(`Request including missing variants omits said variants`, func(t *ftt.Test) {
			res, err := srv.BatchGetTestVariants(ctx, &pb.BatchGetTestVariantsRequest{
				Invocation: "invocations/i0",
				TestVariants: []*pb.BatchGetTestVariantsRequest_TestVariantIdentifier{
					{TestId: "test1", VariantHash: variantHash("a", "b")},
					{TestId: "test1", VariantHash: variantHash("x", "y")},
				},
			})
			assert.Loosely(t, err, should.BeNil)

			assert.Loosely(t, tvStrings(res.TestVariants), should.Resemble([]string{
				fmt.Sprintf("50/test1/%s", variantHash("a", "b")),
			}))

			for _, tv := range res.TestVariants {
				assert.Loosely(t, tv.IsMasked, should.BeFalse)
				assert.Loosely(t, tv.SourcesId, should.Equal(graph.HashSources(testutil.TestSources()).String()))
			}
			assert.Loosely(t, res.Sources, should.HaveLength(1))
			assert.Loosely(t, res.Sources[graph.HashSources(testutil.TestSources()).String()], should.Resemble(testutil.TestSources()))
		})

		t.Run(`Request doesn't return variants from other invocations`, func(t *ftt.Test) {
			res, err := srv.BatchGetTestVariants(ctx, &pb.BatchGetTestVariantsRequest{
				Invocation: "invocations/i0",
				TestVariants: []*pb.BatchGetTestVariantsRequest_TestVariantIdentifier{
					{TestId: "test1", VariantHash: variantHash("e", "f")},
					{TestId: "test3", VariantHash: variantHash("c", "d")},
				},
			})
			assert.Loosely(t, err, should.BeNil)

			assert.Loosely(t, res.TestVariants, should.BeEmpty)
			assert.Loosely(t, res.Sources, should.HaveLength(0))
		})

		t.Run(`Request combines test ID and variant hash correctly`, func(t *ftt.Test) {
			res, err := srv.BatchGetTestVariants(ctx, &pb.BatchGetTestVariantsRequest{
				Invocation: "invocations/i0",
				TestVariants: []*pb.BatchGetTestVariantsRequest_TestVariantIdentifier{
					{TestId: "test1", VariantHash: variantHash("a", "b")},
					{TestId: "test3", VariantHash: variantHash("c", "d")},
				},
			})
			assert.Loosely(t, err, should.BeNil)

			// Testing that we don't match test3, a:b, even though we've
			// requested that test id and variant hash separately.
			assert.Loosely(t, tvStrings(res.TestVariants), should.Resemble([]string{
				fmt.Sprintf("50/test1/%s", variantHash("a", "b")),
			}))

			for _, tv := range res.TestVariants {
				assert.Loosely(t, tv.IsMasked, should.BeFalse)
				assert.Loosely(t, tv.SourcesId, should.Equal(graph.HashSources(testutil.TestSources()).String()))
			}
			assert.Loosely(t, res.Sources, should.HaveLength(1))
			assert.Loosely(t, res.Sources[graph.HashSources(testutil.TestSources()).String()], should.Resemble(testutil.TestSources()))
		})
	})
}

func TestValidateBatchGetTestVariantsRequest(t *testing.T) {
	ftt.Run(`validateBatchGetTestVariantsRequest`, t, func(t *ftt.Test) {
		t.Run(`negative result_limit`, func(t *ftt.Test) {
			err := validateBatchGetTestVariantsRequest(&pb.BatchGetTestVariantsRequest{
				Invocation: "invocations/i0",
				TestVariants: []*pb.BatchGetTestVariantsRequest_TestVariantIdentifier{
					{TestId: "test1", VariantHash: variantHash("a", "b")},
				},
				ResultLimit: -1,
			})
			assert.Loosely(t, err, should.ErrLike(`result_limit: negative`))
		})

		t.Run(`>= 500 test variants`, func(t *ftt.Test) {
			req := &pb.BatchGetTestVariantsRequest{
				Invocation:   "invocations/i0",
				TestVariants: make([]*pb.BatchGetTestVariantsRequest_TestVariantIdentifier, 501),
			}
			for i := 0; i < 500; i += 1 {
				req.TestVariants[i] = &pb.BatchGetTestVariantsRequest_TestVariantIdentifier{
					TestId:      "test1",
					VariantHash: variantHash("a", "b"),
				}
			}

			err := validateBatchGetTestVariantsRequest(req)
			assert.Loosely(t, err, should.ErrLike(`a maximum of 500 test variants can be requested at once`))
		})
	})
}
