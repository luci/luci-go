// Copyright 2025 The LUCI Authors.
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

package verdictingester

import (
	"context"
	"testing"
	"time"

	"cloud.google.com/go/spanner"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/impl/memory"
	rdbpbutil "go.chromium.org/luci/resultdb/pbutil"
	rdbpb "go.chromium.org/luci/resultdb/proto/v1"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/analysis/internal/config"
	ctrlpb "go.chromium.org/luci/analysis/internal/ingestion/control/proto"
	"go.chromium.org/luci/analysis/internal/tasks/taskspb"
	"go.chromium.org/luci/analysis/internal/testresults"
	"go.chromium.org/luci/analysis/internal/testutil"
	"go.chromium.org/luci/analysis/pbutil"
	configpb "go.chromium.org/luci/analysis/proto/config"
	pb "go.chromium.org/luci/analysis/proto/v1"
)

func TestExportTestResults(t *testing.T) {
	ftt.Run("TestExportTestVerdicts", t, func(t *ftt.Test) {
		ctx := testutil.IntegrationTestContext(t)
		partitionTime := clock.Now(ctx).Add(-1 * time.Hour)

		payload := &taskspb.IngestTestVerdicts{
			Build: &ctrlpb.BuildResult{
				Id:      testBuildID,
				Project: "project",
				Changelists: []*pb.Changelist{
					{
						Host:      "anothergerrit.gerrit.instance",
						Change:    77788,
						Patchset:  19,
						OwnerKind: pb.ChangelistOwnerKind_HUMAN,
					},
					{
						Host:      "mygerrit-review.googlesource.com",
						Change:    12345,
						Patchset:  5,
						OwnerKind: pb.ChangelistOwnerKind_AUTOMATION,
					},
				},
			},
			PartitionTime: timestamppb.New(partitionTime),
		}
		input := Inputs{
			Invocation: &rdbpb.Invocation{
				Name:         testInvocation,
				Realm:        testRealm,
				IsExportRoot: true,
				FinalizeTime: timestamppb.New(time.Date(2025, 4, 8, 0, 0, 0, 0, time.UTC)),
			},
			Verdicts: mockedQueryTestVariantsRsp().TestVariants,
			SourcesByID: map[string]*pb.Sources{
				"sources1": {
					GitilesCommit: &pb.GitilesCommit{
						Host:       "project.googlesource.com",
						Project:    "myproject/src",
						Ref:        "refs/heads/main",
						CommitHash: "abcdefabcd1234567890abcdefabcd1234567890",
						Position:   16801,
					},
					Changelists: []*pb.GerritChange{
						{
							Host:      "project-review.googlesource.com",
							Project:   "myproject/src2",
							Change:    9991,
							Patchset:  82,
							OwnerKind: pb.ChangelistOwnerKind_HUMAN,
						},
					},
					IsDirty: false,
				},
			},
			Payload: payload,
		}
		ingester := TestResultsRecorder{}
		cfg := &configpb.Config{
			TestVariantAnalysis: &configpb.TestVariantAnalysis{
				Enabled:               true,
				BigqueryExportEnabled: true,
			},
			TestVerdictExport: &configpb.TestVerdictExport{
				Enabled: true,
			},
			Clustering: &configpb.ClusteringSystem{
				QueryTestVariantAnalysisEnabled: true,
			},
		}
		ctx = memory.Use(ctx)
		err := config.SetTestConfig(ctx, cfg)
		assert.Loosely(t, err, should.BeNil)
		t.Run("without build", func(t *ftt.Test) {
			payload.Build = nil
			payload.PresubmitRun = nil

			err := ingester.Ingest(ctx, input)
			assert.NoErr(t, err)
			verifyTestResults(ctx, t, partitionTime, false)
		})
		t.Run("with build", func(t *ftt.Test) {
			err := ingester.Ingest(ctx, input)
			assert.NoErr(t, err)
			verifyTestResults(ctx, t, partitionTime, true)
		})
	})
}

func verifyTestResults(ctx context.Context, t testing.TB, expectedPartitionTime time.Time, withBuildSource bool) {
	trBuilder := testresults.NewTestResult().
		WithProject("project").
		WithPartitionTime(expectedPartitionTime.In(time.UTC)).
		WithIngestedInvocationID("build-87654321").
		WithSubRealm("ci").
		WithSources(testresults.Sources{})

	if withBuildSource {
		trBuilder = trBuilder.
			WithSources(testresults.Sources{
				Changelists: []testresults.Changelist{
					{
						Host:      "anothergerrit.gerrit.instance",
						Change:    77788,
						Patchset:  19,
						OwnerKind: pb.ChangelistOwnerKind_HUMAN,
					},
					{
						Host:      "mygerrit-review.googlesource.com",
						Change:    12345,
						Patchset:  5,
						OwnerKind: pb.ChangelistOwnerKind_AUTOMATION,
					},
				},
			})
	}

	rdbSources := testresults.Sources{
		RefHash: pbutil.SourceRefHash(&pb.SourceRef{
			System: &pb.SourceRef_Gitiles{
				Gitiles: &pb.GitilesRef{
					Host:    "project.googlesource.com",
					Project: "myproject/src",
					Ref:     "refs/heads/main",
				},
			},
		}),
		Position: 16801,
		Changelists: []testresults.Changelist{
			{
				Host:      "project-review.googlesource.com",
				Change:    9991,
				Patchset:  82,
				OwnerKind: pb.ChangelistOwnerKind_HUMAN,
			},
		},
	}

	expectedTRs := []*testresults.TestResult{
		trBuilder.WithTestID(":module!junit:package:class#test_consistent_failure").
			WithVariantHash("hash").
			WithRunIndex(0).
			WithResultIndex(0).
			WithIsUnexpected(true).
			WithStatusV2(pb.TestResult_FAILED).
			WithStatus(pb.TestResultStatus_FAIL).
			WithRunDuration(3*time.Second+1*time.Microsecond).
			WithExonerationReasons(pb.ExonerationReason_OCCURS_ON_OTHER_CLS, pb.ExonerationReason_NOT_CRITICAL, pb.ExonerationReason_OCCURS_ON_MAINLINE).
			WithIsFromBisection(false).
			WithSources(rdbSources).
			Build(),
		trBuilder.WithTestID(":module!junit:package:class#test_expected").
			WithVariantHash("hash").
			WithRunIndex(0).
			WithResultIndex(0).
			WithIsUnexpected(false).
			WithStatusV2(pb.TestResult_PASSED).
			WithStatus(pb.TestResultStatus_PASS).
			WithRunDuration(5 * time.Second).
			WithoutExoneration().
			WithIsFromBisection(false).
			Build(),
		trBuilder.WithTestID(":module!junit:package:class#test_filtering_event").
			WithVariantHash("hash").
			WithRunIndex(0).
			WithResultIndex(0).
			WithIsUnexpected(false).
			WithStatusV2(pb.TestResult_SKIPPED).
			WithStatus(pb.TestResultStatus_SKIP).
			WithoutRunDuration().
			WithoutExoneration().
			WithIsFromBisection(false).
			Build(),
		trBuilder.WithTestID(":module!junit:package:class#test_from_luci_bisection").
			WithVariantHash("hash").
			WithRunIndex(0).
			WithResultIndex(0).
			WithIsUnexpected(true).
			WithStatusV2(pb.TestResult_FAILED).
			WithStatus(pb.TestResultStatus_PASS).
			WithoutRunDuration().
			WithoutExoneration().
			WithIsFromBisection(true).
			Build(),
		trBuilder.WithTestID(":module!junit:package:class#test_has_unexpected").
			WithVariantHash("hash").
			WithRunIndex(0).
			WithResultIndex(0).
			WithIsUnexpected(true).
			WithStatusV2(pb.TestResult_FAILED).
			WithStatus(pb.TestResultStatus_FAIL).
			WithoutRunDuration().
			WithoutExoneration().
			WithIsFromBisection(false).
			Build(),
		trBuilder.WithTestID(":module!junit:package:class#test_has_unexpected").
			WithVariantHash("hash").
			WithRunIndex(1).
			WithResultIndex(0).
			WithIsUnexpected(false).
			WithStatusV2(pb.TestResult_PASSED).
			WithStatus(pb.TestResultStatus_PASS).
			WithoutRunDuration().
			WithoutExoneration().
			WithIsFromBisection(false).
			Build(),
		trBuilder.WithTestID(":module!junit:package:class#test_known_flake").
			WithVariantHash("hash_2").
			WithRunIndex(0).
			WithResultIndex(0).
			WithIsUnexpected(true).
			WithStatusV2(pb.TestResult_FAILED).
			WithStatus(pb.TestResultStatus_FAIL).
			WithRunDuration(2 * time.Second).
			WithoutExoneration().
			WithIsFromBisection(false).
			Build(),
		trBuilder.WithTestID(":module!junit:package:class#test_new_failure").
			WithVariantHash("hash_1").
			WithRunIndex(0).
			WithResultIndex(0).
			WithIsUnexpected(true).
			WithStatusV2(pb.TestResult_FAILED).
			WithStatus(pb.TestResultStatus_FAIL).
			WithRunDuration(1 * time.Second).
			WithoutExoneration().
			WithIsFromBisection(false).
			Build(),
		trBuilder.WithTestID(":module!junit:package:class#test_new_flake").
			WithVariantHash("hash").
			WithRunIndex(0).
			WithResultIndex(0).
			WithIsUnexpected(true).
			WithStatusV2(pb.TestResult_FAILED).
			WithStatus(pb.TestResultStatus_FAIL).
			WithRunDuration(10 * time.Second).
			WithoutExoneration().
			WithIsFromBisection(false).
			Build(),
		trBuilder.WithTestID(":module!junit:package:class#test_new_flake").
			WithVariantHash("hash").
			WithRunIndex(0).
			WithResultIndex(1).
			WithIsUnexpected(true).
			WithStatusV2(pb.TestResult_FAILED).
			WithStatus(pb.TestResultStatus_FAIL).
			WithRunDuration(11 * time.Second).
			WithoutExoneration().
			WithIsFromBisection(false).
			Build(),
		trBuilder.WithTestID(":module!junit:package:class#test_new_flake").
			WithVariantHash("hash").
			WithRunIndex(1).
			WithResultIndex(0).
			WithIsUnexpected(false).
			WithStatusV2(pb.TestResult_PASSED).
			WithStatus(pb.TestResultStatus_PASS).
			WithRunDuration(12 * time.Second).
			WithoutExoneration().
			WithIsFromBisection(false).
			Build(),
		trBuilder.WithTestID(":module!junit:package:class#test_no_new_results").
			WithVariantHash("hash").
			WithRunIndex(0).
			WithResultIndex(0).
			WithIsUnexpected(true).
			WithStatusV2(pb.TestResult_FAILED).
			WithStatus(pb.TestResultStatus_FAIL).
			WithRunDuration(4 * time.Second).
			WithoutExoneration().
			WithIsFromBisection(false).
			Build(),
		trBuilder.WithTestID(":module!junit:package:class#test_skip").
			WithVariantHash("hash").
			WithRunIndex(0).
			WithResultIndex(0).
			WithIsUnexpected(true).
			WithStatusV2(pb.TestResult_EXECUTION_ERRORED).
			WithStatus(pb.TestResultStatus_SKIP).
			WithoutRunDuration().
			WithoutExoneration().
			WithIsFromBisection(false).
			Build(),
		trBuilder.WithTestID(":module!junit:package:class#test_unexpected_pass").
			WithVariantHash("hash").
			WithRunIndex(0).
			WithResultIndex(0).
			WithIsUnexpected(true).
			WithStatusV2(pb.TestResult_FAILED).
			WithStatus(pb.TestResultStatus_PASS).
			WithoutRunDuration().
			WithoutExoneration().
			WithIsFromBisection(false).
			Build(),
	}

	// Validate TestResults table is populated.
	var actualTRs []*testresults.TestResult
	err := testresults.ReadTestResults(span.Single(ctx), spanner.AllKeys(), func(tr *testresults.TestResult) error {
		actualTRs = append(actualTRs, tr)
		return nil
	})
	assert.Loosely(t, err, should.BeNil, truth.LineContext())
	assert.Loosely(t, actualTRs, should.Match(expectedTRs), truth.LineContext())

	// Validate TestVariantRealms table is populated.
	tvrs := make([]*testresults.TestVariantRealm, 0)
	err = testresults.ReadTestVariantRealms(span.Single(ctx), spanner.AllKeys(), func(tvr *testresults.TestVariantRealm) error {
		tvrs = append(tvrs, tvr)
		return nil
	})
	assert.Loosely(t, err, should.BeNil, truth.LineContext())

	expectedRealms := []*testresults.TestVariantRealm{
		{
			Project:     "project",
			TestID:      ":module!junit:package:class#test_consistent_failure",
			VariantHash: "hash",
			SubRealm:    "ci",
			Variant:     &pb.Variant{},
		},
		{
			Project:     "project",
			TestID:      ":module!junit:package:class#test_expected",
			VariantHash: "hash",
			SubRealm:    "ci",
			Variant:     &pb.Variant{},
		},
		{
			Project:     "project",
			TestID:      ":module!junit:package:class#test_filtering_event",
			VariantHash: "hash",
			SubRealm:    "ci",
			Variant:     &pb.Variant{},
		},
		{
			Project:     "project",
			TestID:      ":module!junit:package:class#test_from_luci_bisection",
			VariantHash: "hash",
			SubRealm:    "ci",
			Variant:     &pb.Variant{},
		},
		{
			Project:     "project",
			TestID:      ":module!junit:package:class#test_has_unexpected",
			VariantHash: "hash",
			SubRealm:    "ci",
			Variant:     &pb.Variant{},
		},
		{
			Project:     "project",
			TestID:      ":module!junit:package:class#test_known_flake",
			VariantHash: "hash_2",
			SubRealm:    "ci",
			Variant:     pbutil.VariantFromResultDB(rdbpbutil.Variant("k1", "v2")),
		},
		{
			Project:     "project",
			TestID:      ":module!junit:package:class#test_new_failure",
			VariantHash: "hash_1",
			SubRealm:    "ci",
			Variant:     pbutil.VariantFromResultDB(rdbpbutil.Variant("k1", "v1")),
		},
		{
			Project:     "project",
			TestID:      ":module!junit:package:class#test_new_flake",
			VariantHash: "hash",
			SubRealm:    "ci",
			Variant:     &pb.Variant{},
		},
		{
			Project:     "project",
			TestID:      ":module!junit:package:class#test_no_new_results",
			VariantHash: "hash",
			SubRealm:    "ci",
			Variant:     &pb.Variant{},
		},
		{
			Project:     "project",
			TestID:      ":module!junit:package:class#test_skip",
			VariantHash: "hash",
			SubRealm:    "ci",
			Variant:     &pb.Variant{},
		},
		{
			Project:     "project",
			TestID:      ":module!junit:package:class#test_unexpected_pass",
			VariantHash: "hash",
			SubRealm:    "ci",
			Variant:     &pb.Variant{},
		},
	}

	assert.Loosely(t, tvrs, should.HaveLength(len(expectedRealms)), truth.LineContext())
	for i, tvr := range tvrs {
		expectedTVR := expectedRealms[i]
		assert.Loosely(t, tvr.LastIngestionTime, should.NotBeZero, truth.LineContext())
		expectedTVR.LastIngestionTime = tvr.LastIngestionTime
		assert.Loosely(t, tvr, should.Match(expectedTVR), truth.LineContext())
	}
}
