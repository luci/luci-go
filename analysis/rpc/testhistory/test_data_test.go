// Copyright 2026 The LUCI Authors.
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

package testhistory

import (
	"context"
	"time"

	"go.chromium.org/luci/common/testing/ftt"
	rdbpb "go.chromium.org/luci/resultdb/proto/v1"
	"go.chromium.org/luci/resultdb/rdbperms"
	"go.chromium.org/luci/server/auth/authtest"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/analysis/internal/resultdb"
	"go.chromium.org/luci/analysis/internal/testresults"
	"go.chromium.org/luci/analysis/pbutil"
	pb "go.chromium.org/luci/analysis/proto/v1"
)

var (
	referenceTime = time.Date(2025, time.February, 12, 0, 0, 0, 0, time.UTC)
	day           = 24 * time.Hour

	var1 = pbutil.Variant("key1", "val1", "key2", "val1")
	var2 = pbutil.Variant("key1", "val2", "key2", "val1")
	var3 = pbutil.Variant("key1", "val2", "key2", "val2")
	var4 = pbutil.Variant("key1", "val1", "key2", "val2")
	var5 = pbutil.Variant("key1", "val3", "key2", "val2")
)

var defaultAuthPermissions = []authtest.RealmPermission{
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
}

// createTestResultData creates test data useful for a number of test history RPCs that
// are based on the TestResults table.
func createTestResultData(ctx context.Context, t *ftt.Test) error {
	// Set up test data in separate transactions to avoid interfering with each other.
	_, err := span.ReadWriteTransaction(ctx, func(ctx context.Context) error {
		// Data for Query and QueryStats.
		insertTVR := func(testID, subRealm string, variant *pb.Variant) {
			span.BufferWrite(ctx, (&testresults.TestVariantRealm{
				Project:     "project",
				TestID:      testID,
				SubRealm:    subRealm,
				Variant:     variant,
				VariantHash: pbutil.VariantHash(variant),
			}).SaveUnverified())
		}

		insertTVR("test_id", "realm", var1)
		insertTVR("previous_test_id", "realm", var1)
		insertTVR("test_id", "realm", var2)
		insertTVR("previous_test_id", "realm", var3)
		insertTVR("test_id", "other-realm", var4)
		insertTVR("test_id", "forbidden-realm", var5)

		insertTV := func(partitionTime time.Time, testID string, variant *pb.Variant, invId string, hasUnsubmittedChanges bool, isFromBisection bool, subRealm string) {
			baseTestResult := testresults.NewTestResult().
				WithProject("project").
				WithTestID(testID).
				WithVariantHash(pbutil.VariantHash(variant)).
				WithPartitionTime(partitionTime).
				WithIngestedInvocationID(invId).
				WithSubRealm(subRealm).
				WithStatus(pb.TestResultStatus_PASS).
				WithStatusV2(pb.TestResult_PASSED).
				WithIsFromBisection(isFromBisection).
				WithoutRunDuration()
			if hasUnsubmittedChanges {
				baseTestResult = baseTestResult.WithSources(testresults.Sources{
					Changelists: []testresults.Changelist{
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
					},
				})
			} else {
				baseTestResult = baseTestResult.WithSources(testresults.Sources{})
			}

			trs := testresults.NewTestVerdict().
				WithBaseTestResult(baseTestResult.Build()).
				WithStatus(pb.TestVerdict_PASSED).
				WithPassedAvgDuration(nil).
				Build()
			for _, tr := range trs {
				span.BufferWrite(ctx, tr.SaveUnverified())
			}
		}

		insertTV(referenceTime.Add(-1*day), "test_id", var1, "inv1", false, false, "realm")
		insertTV(referenceTime.Add(-1*day), "test_id", var1, "inv2", false, false, "realm")
		insertTV(referenceTime.Add(-1*day), "test_id", var2, "inv1", false, false, "realm")
		insertTV(referenceTime.Add(-1*day), "test_id", var2, "inv2", false, true, "realm")

		insertTV(referenceTime.Add(-2*day), "previous_test_id", var1, "inv1", false, false, "realm")
		insertTV(referenceTime.Add(-2*day), "test_id", var1, "inv2", true, false, "realm")
		insertTV(referenceTime.Add(-2*day), "test_id", var2, "inv1", true, false, "realm")

		insertTV(referenceTime.Add(-3*day), "previous_test_id", var3, "inv1", true, false, "realm")

		insertTV(referenceTime.Add(-4*day), "test_id", var4, "inv2", false, false, "other-realm")
		insertTV(referenceTime.Add(-5*day), "test_id", var5, "inv3", false, false, "forbidden-realm")
		return nil
	})
	return err
}

// createFakeTestMetadataClient creates a fake test metadata client which
// redirects "previous_test_id" to "test_id".
func createFakeTestMetadataClient() *resultdb.FakeClient {
	fakeRDBClient := &resultdb.FakeClient{
		TestMetadata: []*rdbpb.TestMetadataDetail{
			{
				Project: "project",
				TestId:  "test_id",
				SourceRef: &rdbpb.SourceRef{
					System: &rdbpb.SourceRef_Gitiles{
						Gitiles: &rdbpb.GitilesRef{
							Host: "chromium.googlesource.com",
							Ref:  "refs/heads/other",
						},
					},
				},
			},
			{
				Project: "project",
				TestId:  "test_id",
				SourceRef: &rdbpb.SourceRef{
					System: &rdbpb.SourceRef_Gitiles{
						Gitiles: &rdbpb.GitilesRef{
							Host: "chromium.googlesource.com",
							Ref:  "refs/heads/main",
						},
					},
				},
				TestMetadata: &rdbpb.TestMetadata{
					PreviousTestId: "previous_test_id",
				},
			},
		},
	}
	return fakeRDBClient
}
