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

package rpc

import (
	"context"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/resultdb/rdbperms"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/analysis/internal/testresults"
	"go.chromium.org/luci/analysis/internal/testutil"
	"go.chromium.org/luci/analysis/pbutil"
	pb "go.chromium.org/luci/analysis/proto/v1"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestTestHistoryServer(t *testing.T) {
	Convey("TestHistoryServer", t, func() {
		ctx := testutil.IntegrationTestContext(t)

		ctx = auth.WithState(ctx, &authtest.FakeState{
			Identity: "user:someone@example.com",
			IdentityPermissions: []authtest.RealmPermission{
				{
					Realm:      "project:realm",
					Permission: rdbperms.PermListTestResults,
				},
				{
					Realm:      "project:realm",
					Permission: rdbperms.PermListTestExonerations,
				},
				{
					Realm:      "project:other-realm",
					Permission: rdbperms.PermListTestResults,
				},
				{
					Realm:      "project:other-realm",
					Permission: rdbperms.PermListTestExonerations,
				},
			},
		})

		referenceTime := time.Date(2025, time.February, 12, 0, 0, 0, 0, time.UTC)
		day := 24 * time.Hour

		var1 := pbutil.Variant("key1", "val1", "key2", "val1")
		var2 := pbutil.Variant("key1", "val2", "key2", "val1")
		var3 := pbutil.Variant("key1", "val2", "key2", "val2")
		var4 := pbutil.Variant("key1", "val1", "key2", "val2")
		var5 := pbutil.Variant("key1", "val3", "key2", "val2")

		_, err := span.ReadWriteTransaction(ctx, func(ctx context.Context) error {
			insertTR := func(subRealm string, testID string) {
				span.BufferWrite(ctx, (&testresults.TestRealm{
					Project:  "project",
					TestID:   testID,
					SubRealm: subRealm,
				}).SaveUnverified())
			}
			insertTR("realm", "test_id")
			insertTR("realm", "test_id1")
			insertTR("realm", "test_id2")
			insertTR("other-realm", "test_id3")
			insertTR("forbidden-realm", "test_id4")

			insertTVR := func(subRealm string, variant *pb.Variant) {
				span.BufferWrite(ctx, (&testresults.TestVariantRealm{
					Project:     "project",
					TestID:      "test_id",
					SubRealm:    subRealm,
					Variant:     variant,
					VariantHash: pbutil.VariantHash(variant),
				}).SaveUnverified())
			}

			insertTVR("realm", var1)
			insertTVR("realm", var2)
			insertTVR("realm", var3)
			insertTVR("other-realm", var4)
			insertTVR("forbidden-realm", var5)

			insertTV := func(partitionTime time.Time, variant *pb.Variant, invId string, hasUnsubmittedChanges bool, isFromBisection bool, subRealm string) {
				baseTestResult := testresults.NewTestResult().
					WithProject("project").
					WithTestID("test_id").
					WithVariantHash(pbutil.VariantHash(variant)).
					WithPartitionTime(partitionTime).
					WithIngestedInvocationID(invId).
					WithSubRealm(subRealm).
					WithStatus(pb.TestResultStatus_PASS).
					WithIsFromBisection(isFromBisection).
					WithoutRunDuration()
				if hasUnsubmittedChanges {
					baseTestResult = baseTestResult.WithChangelists([]testresults.Changelist{
						{
							Host:      "anothergerrit.gerrit.instance",
							Change:    5471,
							Patchset:  6,
							OwnerKind: pb.ChangelistOwnerKind_HUMAN,
						},
						{
							Host:      "mygerrit-review.googlesource.com",
							Change:    4321,
							Patchset:  5,
							OwnerKind: pb.ChangelistOwnerKind_AUTOMATION,
						},
					})
				} else {
					baseTestResult = baseTestResult.WithChangelists(nil)
				}

				trs := testresults.NewTestVerdict().
					WithBaseTestResult(baseTestResult.Build()).
					WithStatus(pb.TestVerdictStatus_EXPECTED).
					WithPassedAvgDuration(nil).
					Build()
				for _, tr := range trs {
					span.BufferWrite(ctx, tr.SaveUnverified())
				}
			}

			insertTV(referenceTime.Add(-1*day), var1, "inv1", false, false, "realm")
			insertTV(referenceTime.Add(-1*day), var1, "inv2", false, false, "realm")
			insertTV(referenceTime.Add(-1*day), var2, "inv1", false, false, "realm")
			insertTV(referenceTime.Add(-1*day), var2, "inv2", false, true, "realm")

			insertTV(referenceTime.Add(-2*day), var1, "inv1", false, false, "realm")
			insertTV(referenceTime.Add(-2*day), var1, "inv2", true, false, "realm")
			insertTV(referenceTime.Add(-2*day), var2, "inv1", true, false, "realm")

			insertTV(referenceTime.Add(-3*day), var3, "inv1", true, false, "realm")

			insertTV(referenceTime.Add(-4*day), var4, "inv2", false, false, "other-realm")
			insertTV(referenceTime.Add(-5*day), var5, "inv3", false, false, "forbidden-realm")

			return nil
		})
		So(err, ShouldBeNil)

		server := NewTestHistoryServer()

		Convey("Query", func() {
			req := &pb.QueryTestHistoryRequest{
				Project: "project",
				TestId:  "test_id",
				Predicate: &pb.TestVerdictPredicate{
					SubRealm: "realm",
				},
				PageSize: 5,
			}

			expectedChangelists := []*pb.Changelist{
				{
					Host:      "anothergerrit.gerrit.instance",
					Change:    5471,
					Patchset:  6,
					OwnerKind: pb.ChangelistOwnerKind_HUMAN,
				},
				{
					Host:      "mygerrit-review.googlesource.com",
					Change:    4321,
					Patchset:  5,
					OwnerKind: pb.ChangelistOwnerKind_AUTOMATION,
				},
			}

			Convey("unauthorised requests are rejected", func() {
				testPerm := func(ctx context.Context) {
					res, err := server.Query(ctx, req)
					So(err, ShouldErrLike, `caller does not have permission`, `in realm "project:realm"`)
					So(err, ShouldHaveGRPCStatus, codes.PermissionDenied)
					So(res, ShouldBeNil)
				}

				// No permission.
				ctx = auth.WithState(ctx, &authtest.FakeState{
					Identity: "user:someone@example.com",
				})
				testPerm(ctx)

				// testResults.list only.
				ctx = auth.WithState(ctx, &authtest.FakeState{
					Identity: "user:someone@example.com",
					IdentityPermissions: []authtest.RealmPermission{
						{
							Realm:      "project:realm",
							Permission: rdbperms.PermListTestResults,
						},
						{
							Realm:      "project:other_realm",
							Permission: rdbperms.PermListTestExonerations,
						},
					},
				})
				testPerm(ctx)

				// testExonerations.list only.
				ctx = auth.WithState(ctx, &authtest.FakeState{
					Identity: "user:someone@example.com",
					IdentityPermissions: []authtest.RealmPermission{
						{
							Realm:      "project:other_realm",
							Permission: rdbperms.PermListTestResults,
						},
						{
							Realm:      "project:realm",
							Permission: rdbperms.PermListTestExonerations,
						},
					},
				})
				testPerm(ctx)
			})

			Convey("invalid requests are rejected", func() {
				req.PageSize = -1
				res, err := server.Query(ctx, req)
				So(err, ShouldNotBeNil)
				So(err, ShouldHaveGRPCStatus, codes.InvalidArgument)
				So(res, ShouldBeNil)
			})

			Convey("multi-realms", func() {
				req.Predicate.SubRealm = ""
				req.Predicate.VariantPredicate = &pb.VariantPredicate{
					Predicate: &pb.VariantPredicate_Contains{
						Contains: pbutil.Variant("key2", "val2"),
					},
				}
				res, err := server.Query(ctx, req)
				So(err, ShouldBeNil)
				So(res, ShouldResembleProto, &pb.QueryTestHistoryResponse{
					Verdicts: []*pb.TestVerdict{
						{
							TestId:        "test_id",
							VariantHash:   pbutil.VariantHash(var3),
							InvocationId:  "inv1",
							Status:        pb.TestVerdictStatus_EXPECTED,
							PartitionTime: timestamppb.New(referenceTime.Add(-3 * day)),
							Changelists:   expectedChangelists,
						},
						{
							TestId:        "test_id",
							VariantHash:   pbutil.VariantHash(var4),
							InvocationId:  "inv2",
							Status:        pb.TestVerdictStatus_EXPECTED,
							PartitionTime: timestamppb.New(referenceTime.Add(-4 * day)),
						},
					},
				})
			})

			Convey("e2e", func() {
				res, err := server.Query(ctx, req)
				So(err, ShouldBeNil)
				So(res, ShouldResembleProto, &pb.QueryTestHistoryResponse{
					Verdicts: []*pb.TestVerdict{
						{
							TestId:        "test_id",
							VariantHash:   pbutil.VariantHash(var1),
							InvocationId:  "inv1",
							Status:        pb.TestVerdictStatus_EXPECTED,
							PartitionTime: timestamppb.New(referenceTime.Add(-1 * day)),
						},
						{
							TestId:        "test_id",
							VariantHash:   pbutil.VariantHash(var1),
							InvocationId:  "inv2",
							Status:        pb.TestVerdictStatus_EXPECTED,
							PartitionTime: timestamppb.New(referenceTime.Add(-1 * day)),
						},
						{
							TestId:        "test_id",
							VariantHash:   pbutil.VariantHash(var2),
							InvocationId:  "inv1",
							Status:        pb.TestVerdictStatus_EXPECTED,
							PartitionTime: timestamppb.New(referenceTime.Add(-1 * day)),
						},
						{
							TestId:        "test_id",
							VariantHash:   pbutil.VariantHash(var1),
							InvocationId:  "inv1",
							Status:        pb.TestVerdictStatus_EXPECTED,
							PartitionTime: timestamppb.New(referenceTime.Add(-2 * day)),
						},
						{
							TestId:        "test_id",
							VariantHash:   pbutil.VariantHash(var1),
							InvocationId:  "inv2",
							Status:        pb.TestVerdictStatus_EXPECTED,
							PartitionTime: timestamppb.New(referenceTime.Add(-2 * day)),
							Changelists:   expectedChangelists,
						},
					},
					NextPageToken: res.NextPageToken,
				})
				So(res.NextPageToken, ShouldNotBeEmpty)

				req.PageToken = res.NextPageToken
				res, err = server.Query(ctx, req)
				So(err, ShouldBeNil)
				So(res, ShouldResembleProto, &pb.QueryTestHistoryResponse{
					Verdicts: []*pb.TestVerdict{
						{
							TestId:        "test_id",
							VariantHash:   pbutil.VariantHash(var2),
							InvocationId:  "inv1",
							Status:        pb.TestVerdictStatus_EXPECTED,
							PartitionTime: timestamppb.New(referenceTime.Add(-2 * day)),
							Changelists:   expectedChangelists,
						},
						{
							TestId:        "test_id",
							VariantHash:   pbutil.VariantHash(var3),
							InvocationId:  "inv1",
							Status:        pb.TestVerdictStatus_EXPECTED,
							PartitionTime: timestamppb.New(referenceTime.Add(-3 * day)),
							Changelists:   expectedChangelists,
						},
					},
				})
			})

			Convey("include bisection", func() {
				req.PageSize = 10
				req.Predicate.IncludeBisectionResults = true
				res, err := server.Query(ctx, req)
				So(err, ShouldBeNil)
				So(res, ShouldResembleProto, &pb.QueryTestHistoryResponse{
					Verdicts: []*pb.TestVerdict{
						{
							TestId:        "test_id",
							VariantHash:   pbutil.VariantHash(var1),
							InvocationId:  "inv1",
							Status:        pb.TestVerdictStatus_EXPECTED,
							PartitionTime: timestamppb.New(referenceTime.Add(-1 * day)),
						},
						{
							TestId:        "test_id",
							VariantHash:   pbutil.VariantHash(var1),
							InvocationId:  "inv2",
							Status:        pb.TestVerdictStatus_EXPECTED,
							PartitionTime: timestamppb.New(referenceTime.Add(-1 * day)),
						},
						{
							TestId:        "test_id",
							VariantHash:   pbutil.VariantHash(var2),
							InvocationId:  "inv1",
							Status:        pb.TestVerdictStatus_EXPECTED,
							PartitionTime: timestamppb.New(referenceTime.Add(-1 * day)),
						},
						{
							TestId:        "test_id",
							VariantHash:   pbutil.VariantHash(var2),
							InvocationId:  "inv2",
							Status:        pb.TestVerdictStatus_EXPECTED,
							PartitionTime: timestamppb.New(referenceTime.Add(-1 * day)),
						},
						{
							TestId:        "test_id",
							VariantHash:   pbutil.VariantHash(var1),
							InvocationId:  "inv1",
							Status:        pb.TestVerdictStatus_EXPECTED,
							PartitionTime: timestamppb.New(referenceTime.Add(-2 * day)),
						},
						{
							TestId:        "test_id",
							VariantHash:   pbutil.VariantHash(var1),
							InvocationId:  "inv2",
							Status:        pb.TestVerdictStatus_EXPECTED,
							PartitionTime: timestamppb.New(referenceTime.Add(-2 * day)),
							Changelists:   expectedChangelists,
						},
						{
							TestId:        "test_id",
							VariantHash:   pbutil.VariantHash(var2),
							InvocationId:  "inv1",
							Status:        pb.TestVerdictStatus_EXPECTED,
							PartitionTime: timestamppb.New(referenceTime.Add(-2 * day)),
							Changelists:   expectedChangelists,
						},
						{
							TestId:        "test_id",
							VariantHash:   pbutil.VariantHash(var3),
							InvocationId:  "inv1",
							Status:        pb.TestVerdictStatus_EXPECTED,
							PartitionTime: timestamppb.New(referenceTime.Add(-3 * day)),
							Changelists:   expectedChangelists,
						},
					},
				})
			})
		})

		Convey("QueryStats", func() {
			req := &pb.QueryTestHistoryStatsRequest{
				Project: "project",
				TestId:  "test_id",
				Predicate: &pb.TestVerdictPredicate{
					SubRealm: "realm",
				},
				PageSize: 3,
			}

			Convey("unauthorised requests are rejected", func() {
				testPerm := func(ctx context.Context) {
					res, err := server.QueryStats(ctx, req)
					So(err, ShouldErrLike, `caller does not have permission`, `in realm "project:realm"`)
					So(err, ShouldHaveGRPCStatus, codes.PermissionDenied)
					So(res, ShouldBeNil)
				}

				// No permission.
				ctx = auth.WithState(ctx, &authtest.FakeState{
					Identity: "user:someone@example.com",
				})
				testPerm(ctx)

				// testResults.list only.
				ctx = auth.WithState(ctx, &authtest.FakeState{
					Identity: "user:someone@example.com",
					IdentityPermissions: []authtest.RealmPermission{
						{
							Realm:      "project:realm",
							Permission: rdbperms.PermListTestResults,
						},
						{
							Realm:      "project:other_realm",
							Permission: rdbperms.PermListTestExonerations,
						},
					},
				})
				testPerm(ctx)

				// testExonerations.list only.
				ctx = auth.WithState(ctx, &authtest.FakeState{
					Identity: "user:someone@example.com",
					IdentityPermissions: []authtest.RealmPermission{
						{
							Realm:      "project:other_realm",
							Permission: rdbperms.PermListTestResults,
						},
						{
							Realm:      "project:realm",
							Permission: rdbperms.PermListTestExonerations,
						},
					},
				})
				testPerm(ctx)
			})

			Convey("invalid requests are rejected", func() {
				req.PageSize = -1
				res, err := server.QueryStats(ctx, req)
				So(err, ShouldNotBeNil)
				So(err, ShouldHaveGRPCStatus, codes.InvalidArgument)
				So(res, ShouldBeNil)
			})

			Convey("multi-realms", func() {
				req.Predicate.SubRealm = ""
				req.Predicate.VariantPredicate = &pb.VariantPredicate{
					Predicate: &pb.VariantPredicate_Contains{
						Contains: pbutil.Variant("key2", "val2"),
					},
				}
				res, err := server.QueryStats(ctx, req)
				So(err, ShouldBeNil)
				So(res, ShouldResembleProto, &pb.QueryTestHistoryStatsResponse{
					Groups: []*pb.QueryTestHistoryStatsResponse_Group{
						{
							PartitionTime: timestamppb.New(referenceTime.Add(-3 * day)),
							VariantHash:   pbutil.VariantHash(var3),
							ExpectedCount: 1,
						},
						{
							PartitionTime: timestamppb.New(referenceTime.Add(-4 * day)),
							VariantHash:   pbutil.VariantHash(var4),
							ExpectedCount: 1,
						},
					},
				})
			})

			Convey("e2e", func() {
				res, err := server.QueryStats(ctx, req)
				So(err, ShouldBeNil)
				So(res, ShouldResembleProto, &pb.QueryTestHistoryStatsResponse{
					Groups: []*pb.QueryTestHistoryStatsResponse_Group{
						{
							PartitionTime: timestamppb.New(referenceTime.Add(-1 * day)),
							VariantHash:   pbutil.VariantHash(var1),
							ExpectedCount: 2,
						},
						{
							PartitionTime: timestamppb.New(referenceTime.Add(-1 * day)),
							VariantHash:   pbutil.VariantHash(var2),
							ExpectedCount: 1,
						},
						{
							PartitionTime: timestamppb.New(referenceTime.Add(-2 * day)),
							VariantHash:   pbutil.VariantHash(var1),
							ExpectedCount: 2,
						},
					},
					NextPageToken: res.NextPageToken,
				})
				So(res.NextPageToken, ShouldNotBeEmpty)

				req.PageToken = res.NextPageToken
				res, err = server.QueryStats(ctx, req)
				So(err, ShouldBeNil)
				So(res, ShouldResembleProto, &pb.QueryTestHistoryStatsResponse{
					Groups: []*pb.QueryTestHistoryStatsResponse_Group{
						{
							PartitionTime: timestamppb.New(referenceTime.Add(-2 * day)),
							VariantHash:   pbutil.VariantHash(var2),
							ExpectedCount: 1,
						},
						{
							PartitionTime: timestamppb.New(referenceTime.Add(-3 * day)),
							VariantHash:   pbutil.VariantHash(var3),
							ExpectedCount: 1,
						},
					},
				})
			})

			Convey("include bisection", func() {
				req.Predicate.IncludeBisectionResults = true
				req.PageSize = 10
				res, err := server.QueryStats(ctx, req)
				So(err, ShouldBeNil)
				So(res, ShouldResembleProto, &pb.QueryTestHistoryStatsResponse{
					Groups: []*pb.QueryTestHistoryStatsResponse_Group{
						{
							PartitionTime: timestamppb.New(referenceTime.Add(-1 * day)),
							VariantHash:   pbutil.VariantHash(var1),
							ExpectedCount: 2,
						},
						{
							PartitionTime: timestamppb.New(referenceTime.Add(-1 * day)),
							VariantHash:   pbutil.VariantHash(var2),
							ExpectedCount: 2,
						},
						{
							PartitionTime: timestamppb.New(referenceTime.Add(-2 * day)),
							VariantHash:   pbutil.VariantHash(var1),
							ExpectedCount: 2,
						},
						{
							PartitionTime: timestamppb.New(referenceTime.Add(-2 * day)),
							VariantHash:   pbutil.VariantHash(var2),
							ExpectedCount: 1,
						},
						{
							PartitionTime: timestamppb.New(referenceTime.Add(-3 * day)),
							VariantHash:   pbutil.VariantHash(var3),
							ExpectedCount: 1,
						},
					},
				})
			})
		})

		Convey("QueryVariants", func() {
			req := &pb.QueryVariantsRequest{
				Project:  "project",
				TestId:   "test_id",
				SubRealm: "realm",
				PageSize: 2,
			}

			Convey("unauthorised requests are rejected", func() {
				ctx = auth.WithState(ctx, &authtest.FakeState{
					Identity: "user:someone@example.com",
				})
				res, err := server.QueryVariants(ctx, req)
				So(err, ShouldErrLike, `caller does not have permission`, `in realm "project:realm"`)
				So(err, ShouldHaveGRPCStatus, codes.PermissionDenied)
				So(res, ShouldBeNil)
			})

			Convey("invalid requests are rejected", func() {
				req.PageSize = -1
				res, err := server.QueryVariants(ctx, req)
				So(err, ShouldNotBeNil)
				So(err, ShouldHaveGRPCStatus, codes.InvalidArgument)
				So(res, ShouldBeNil)
			})

			Convey("multi-realms", func() {
				req.PageSize = 0
				req.SubRealm = ""
				req.VariantPredicate = &pb.VariantPredicate{
					Predicate: &pb.VariantPredicate_Contains{
						Contains: pbutil.Variant("key2", "val2"),
					},
				}
				res, err := server.QueryVariants(ctx, req)
				So(err, ShouldBeNil)
				So(res, ShouldResembleProto, &pb.QueryVariantsResponse{
					Variants: []*pb.QueryVariantsResponse_VariantInfo{
						{
							VariantHash: pbutil.VariantHash(var3),
							Variant:     var3,
						},
						{
							VariantHash: pbutil.VariantHash(var4),
							Variant:     var4,
						},
					},
				})
			})

			Convey("e2e", func() {
				res, err := server.QueryVariants(ctx, req)
				So(err, ShouldBeNil)
				So(res, ShouldResembleProto, &pb.QueryVariantsResponse{
					Variants: []*pb.QueryVariantsResponse_VariantInfo{
						{
							VariantHash: pbutil.VariantHash(var1),
							Variant:     var1,
						},
						{
							VariantHash: pbutil.VariantHash(var3),
							Variant:     var3,
						},
					},
					NextPageToken: res.NextPageToken,
				})
				So(res.NextPageToken, ShouldNotBeEmpty)

				req.PageToken = res.NextPageToken
				res, err = server.QueryVariants(ctx, req)
				So(err, ShouldBeNil)
				So(res, ShouldResembleProto, &pb.QueryVariantsResponse{
					Variants: []*pb.QueryVariantsResponse_VariantInfo{
						{
							VariantHash: pbutil.VariantHash(var2),
							Variant:     var2,
						},
					},
				})
			})
		})

		Convey("QueryTests", func() {
			req := &pb.QueryTestsRequest{
				Project:         "project",
				TestIdSubstring: "test_id",
				SubRealm:        "realm",
				PageSize:        2,
			}

			Convey("unauthorised requests are rejected", func() {
				ctx = auth.WithState(ctx, &authtest.FakeState{
					Identity: "user:someone@example.com",
				})
				res, err := server.QueryTests(ctx, req)
				So(err, ShouldErrLike, `caller does not have permission`, `in realm "project:realm"`)
				So(err, ShouldHaveGRPCStatus, codes.PermissionDenied)
				So(res, ShouldBeNil)
			})

			Convey("invalid requests are rejected", func() {
				req.PageSize = -1
				res, err := server.QueryTests(ctx, req)
				So(err, ShouldNotBeNil)
				So(err, ShouldHaveGRPCStatus, codes.InvalidArgument)
				So(res, ShouldBeNil)
			})

			Convey("multi-realms", func() {
				req.PageSize = 0
				req.SubRealm = ""
				res, err := server.QueryTests(ctx, req)
				So(err, ShouldBeNil)
				So(res, ShouldResembleProto, &pb.QueryTestsResponse{
					TestIds: []string{"test_id", "test_id1", "test_id2", "test_id3"},
				})
			})

			Convey("e2e", func() {
				res, err := server.QueryTests(ctx, req)
				So(err, ShouldBeNil)
				So(res, ShouldResembleProto, &pb.QueryTestsResponse{
					TestIds:       []string{"test_id", "test_id1"},
					NextPageToken: res.NextPageToken,
				})
				So(res.NextPageToken, ShouldNotBeEmpty)

				req.PageToken = res.NextPageToken
				res, err = server.QueryTests(ctx, req)
				So(err, ShouldBeNil)
				So(res, ShouldResembleProto, &pb.QueryTestsResponse{
					TestIds: []string{"test_id2"},
				})
			})
		})
	})
}

func TestValidateQueryTestHistoryRequest(t *testing.T) {
	t.Parallel()

	Convey("validateQueryTestHistoryRequest", t, func() {
		req := &pb.QueryTestHistoryRequest{
			Project: "project",
			TestId:  "test_id",
			Predicate: &pb.TestVerdictPredicate{
				SubRealm: "realm",
			},
			PageSize: 5,
		}

		Convey("valid", func() {
			err := validateQueryTestHistoryRequest(req)
			So(err, ShouldBeNil)
		})

		Convey("no project", func() {
			req.Project = ""
			err := validateQueryTestHistoryRequest(req)
			So(err, ShouldErrLike, "project: unspecified")
		})

		Convey("invalid project", func() {
			req.Project = "project:realm"
			err := validateQueryTestHistoryRequest(req)
			So(err, ShouldErrLike, `project: must match ^[a-z0-9\-]{1,40}$`)
		})

		Convey("no test_id", func() {
			req.TestId = ""
			err := validateQueryTestHistoryRequest(req)
			So(err, ShouldErrLike, "test_id: unspecified")
		})

		Convey("invalid test_id", func() {
			req.TestId = "\xFF"
			err := validateQueryTestHistoryRequest(req)
			So(err, ShouldErrLike, "test_id: not a valid utf8 string")
		})

		Convey("no predicate", func() {
			req.Predicate = nil
			err := validateQueryTestHistoryRequest(req)
			So(err, ShouldErrLike, "predicate", "unspecified")
		})

		Convey("no page size", func() {
			req.PageSize = 0
			err := validateQueryTestHistoryRequest(req)
			So(err, ShouldBeNil)
		})

		Convey("negative page size", func() {
			req.PageSize = -1
			err := validateQueryTestHistoryRequest(req)
			So(err, ShouldErrLike, "page_size", "negative")
		})
	})
}

func TestValidateQueryTestHistoryStatsRequest(t *testing.T) {
	t.Parallel()

	Convey("validateQueryTestHistoryStatsRequest", t, func() {
		req := &pb.QueryTestHistoryStatsRequest{
			Project: "project",
			TestId:  "test_id",
			Predicate: &pb.TestVerdictPredicate{
				SubRealm: "realm",
			},
			PageSize: 5,
		}

		Convey("valid", func() {
			err := validateQueryTestHistoryStatsRequest(req)
			So(err, ShouldBeNil)
		})

		Convey("no project", func() {
			req.Project = ""
			err := validateQueryTestHistoryStatsRequest(req)
			So(err, ShouldErrLike, "project: unspecified")
		})

		Convey("invalid project", func() {
			req.Project = "project:realm"
			err := validateQueryTestHistoryStatsRequest(req)
			So(err, ShouldErrLike, `project: must match ^[a-z0-9\-]{1,40}$`)
		})

		Convey("no test_id", func() {
			req.TestId = ""
			err := validateQueryTestHistoryStatsRequest(req)
			So(err, ShouldErrLike, "test_id: unspecified")
		})

		Convey("invalid test_id", func() {
			req.TestId = "\xFF"
			err := validateQueryTestHistoryStatsRequest(req)
			So(err, ShouldErrLike, "test_id: not a valid utf8 string")
		})

		Convey("no predicate", func() {
			req.Predicate = nil
			err := validateQueryTestHistoryStatsRequest(req)
			So(err, ShouldErrLike, "predicate", "unspecified")
		})

		Convey("no page size", func() {
			req.PageSize = 0
			err := validateQueryTestHistoryStatsRequest(req)
			So(err, ShouldBeNil)
		})

		Convey("negative page size", func() {
			req.PageSize = -1
			err := validateQueryTestHistoryStatsRequest(req)
			So(err, ShouldErrLike, "page_size", "negative")
		})
	})
}

func TestValidateQueryVariantsRequest(t *testing.T) {
	t.Parallel()

	Convey("validateQueryVariantsRequest", t, func() {
		req := &pb.QueryVariantsRequest{
			Project:  "project",
			TestId:   "test_id",
			PageSize: 5,
		}

		Convey("valid", func() {
			err := validateQueryVariantsRequest(req)
			So(err, ShouldBeNil)
		})

		Convey("no project", func() {
			req.Project = ""
			err := validateQueryVariantsRequest(req)
			So(err, ShouldErrLike, "project: unspecified")
		})

		Convey("invalid project", func() {
			req.Project = "project:realm"
			err := validateQueryVariantsRequest(req)
			So(err, ShouldErrLike, `project: must match ^[a-z0-9\-]{1,40}$`)
		})

		Convey("no test_id", func() {
			req.TestId = ""
			err := validateQueryVariantsRequest(req)
			So(err, ShouldErrLike, "test_id: unspecified")
		})

		Convey("invalid test_id", func() {
			req.TestId = "\xFF"
			err := validateQueryVariantsRequest(req)
			So(err, ShouldErrLike, "test_id: not a valid utf8 string")
		})

		Convey("bad sub_realm", func() {
			req.SubRealm = "a:realm"
			err := validateQueryVariantsRequest(req)
			So(err, ShouldErrLike, "sub_realm: bad project-scoped realm name")
		})

		Convey("no page size", func() {
			req.PageSize = 0
			err := validateQueryVariantsRequest(req)
			So(err, ShouldBeNil)
		})

		Convey("negative page size", func() {
			req.PageSize = -1
			err := validateQueryVariantsRequest(req)
			So(err, ShouldErrLike, "page_size", "negative")
		})
	})
}

func TestValidateQueryTestsRequest(t *testing.T) {
	t.Parallel()

	Convey("validateQueryTestsRequest", t, func() {
		req := &pb.QueryTestsRequest{
			Project:         "project",
			TestIdSubstring: "test_id",
			PageSize:        5,
		}

		Convey("valid", func() {
			err := validateQueryTestsRequest(req)
			So(err, ShouldBeNil)
		})

		Convey("no project", func() {
			req.Project = ""
			err := validateQueryTestsRequest(req)
			So(err, ShouldErrLike, "project: unspecified")
		})

		Convey("invalid project", func() {
			req.Project = "project:realm"
			err := validateQueryTestsRequest(req)
			So(err, ShouldErrLike, `project: must match ^[a-z0-9\-]{1,40}$`)
		})

		Convey("no test_id_substring", func() {
			req.TestIdSubstring = ""
			err := validateQueryTestsRequest(req)
			So(err, ShouldErrLike, "test_id_substring: unspecified")
		})

		Convey("bad test_id_substring", func() {
			req.TestIdSubstring = "\xFF"
			err := validateQueryTestsRequest(req)
			So(err, ShouldErrLike, "test_id_substring: not a valid utf8 string")
		})

		Convey("bad sub_realm", func() {
			req.SubRealm = "a:realm"
			err := validateQueryTestsRequest(req)
			So(err, ShouldErrLike, "sub_realm: bad project-scoped realm name")
		})

		Convey("no page size", func() {
			req.PageSize = 0
			err := validateQueryTestsRequest(req)
			So(err, ShouldBeNil)
		})

		Convey("negative page size", func() {
			req.PageSize = -1
			err := validateQueryTestsRequest(req)
			So(err, ShouldErrLike, "page_size", "negative")
		})
	})
}
