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
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/impl/memory"
	rdbpb "go.chromium.org/luci/resultdb/proto/v1"
	"go.chromium.org/luci/server/caching"

	"go.chromium.org/luci/analysis/internal/analysis"
	"go.chromium.org/luci/analysis/internal/analysis/clusteredfailures"
	"go.chromium.org/luci/analysis/internal/clustering/chunkstore"
	"go.chromium.org/luci/analysis/internal/clustering/ingestion"
	"go.chromium.org/luci/analysis/internal/config"
	ctrlpb "go.chromium.org/luci/analysis/internal/ingestion/control/proto"
	"go.chromium.org/luci/analysis/internal/tasks/taskspb"
	"go.chromium.org/luci/analysis/internal/testutil"
	bqpb "go.chromium.org/luci/analysis/proto/bq"
	configpb "go.chromium.org/luci/analysis/proto/config"
	pb "go.chromium.org/luci/analysis/proto/v1"
)

func TestExportClusetering(t *testing.T) {
	ftt.Run("TestExportTestVerdicts", t, func(t *ftt.Test) {
		ctx := testutil.IntegrationTestContext(t)
		ctx = memory.Use(ctx)
		ctx = caching.WithEmptyProcessCache(ctx) // For failure association rules cache.

		cfg := &configpb.Config{
			Clustering: &configpb.ClusteringSystem{
				QueryTestVariantAnalysisEnabled: true,
			},
			TestVariantAnalysis: &configpb.TestVariantAnalysis{
				Enabled: true,
			},
		}
		err := config.SetTestConfig(ctx, cfg)
		assert.Loosely(t, err, should.BeNil)
		partitionTime := clock.Now(ctx).Add(-1 * time.Hour)
		invocationCreationTime := partitionTime.Add(-3 * time.Hour)

		payload := &taskspb.IngestTestVerdicts{
			IngestionId: "ingestion-id",
			Project:     "project",
			Invocation: &ctrlpb.InvocationResult{
				ResultdbHost: "rdb-host",
				InvocationId: invocationID,
				CreationTime: timestamppb.New(invocationCreationTime),
			},
			Build: &ctrlpb.BuildResult{
				Id:                testBuildID,
				GardenerRotations: []string{"rotation1", "rotation2"},
			},
			PartitionTime:        timestamppb.New(partitionTime),
			PageToken:            "expected_token",
			TaskIndex:            1,
			UseNewIngestionOrder: true,
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
					BaseSources: &pb.Sources_GitilesCommit{
						GitilesCommit: &pb.GitilesCommit{
							Host:       "project.googlesource.com",
							Project:    "myproject/src",
							Ref:        "refs/heads/main",
							CommitHash: "abcdefabcd1234567890abcdefabcd1234567890",
							Position:   16801,
						},
					},
					IsDirty: false,
				},
			},
			Payload: payload,
		}
		chunkStore := chunkstore.NewFakeClient()
		clusteredFailures := clusteredfailures.NewFakeClient()
		analysis := analysis.NewClusteringHandler(clusteredFailures)
		ingester := ClusteringExporter{clustering: ingestion.New(chunkStore, analysis)}
		// Populate some existing test variant analysis.
		// Test variant changepoint analysis uses invocation creation time
		// as partition time.
		setupTestVariantAnalysis(ctx, t, invocationCreationTime)
		t.Run("with build", func(t *ftt.Test) {
			err := ingester.Ingest(ctx, input)
			assert.NoErr(t, err)
			verifyClustering(t, chunkStore, clusteredFailures)
		})

		t.Run("without build", func(t *ftt.Test) {
			payload.Build = nil
			payload.PresubmitRun = nil

			err := ingester.Ingest(ctx, input)
			assert.NoErr(t, err)
			// Clustering not enabled - no chunk has been written to GCS.
			assert.Loosely(t, len(chunkStore.Contents), should.BeZero)
		})
	})
}

func verifyClustering(t testing.TB, chunkStore *chunkstore.FakeClient, clusteredFailures *clusteredfailures.FakeClient) {
	t.Helper()
	// Confirm chunks have been written to GCS.
	assert.Loosely(t, len(chunkStore.Contents), should.Equal(1), truth.LineContext())

	// Confirm clustering has occurred, with each test result in at
	// least one cluster.
	actualClusteredFailures := make(map[string]int)
	for _, f := range clusteredFailures.Insertions {
		assert.Loosely(t, f.Project, should.Equal("project"), truth.LineContext())
		actualClusteredFailures[f.TestId] += 1
	}
	expectedClusteredFailures := map[string]int{
		":module!junit:package:class#test_new_failure":        1,
		":module!junit:package:class#test_known_flake":        1,
		":module!junit:package:class#test_consistent_failure": 2, // One failure is in two clusters due it having a failure reason.
		":module!junit:package:class#test_no_new_results":     1,
		":module!junit:package:class#test_new_flake":          2,
		":module!junit:package:class#test_has_unexpected":     1,
	}
	assert.Loosely(t, actualClusteredFailures, should.Match(expectedClusteredFailures), truth.LineContext())

	for _, cf := range clusteredFailures.Insertions {
		assert.Loosely(t, cf.BuildGardenerRotations, should.Match([]string{"rotation1", "rotation2"}), truth.LineContext())

		// Verify test variant branch stats were correctly populated.
		if cf.TestId == ":module!junit:package:class#test_consistent_failure" {
			assert.Loosely(t, cf.TestVariantBranch, should.Match(&bqpb.ClusteredFailureRow_TestVariantBranch{
				UnexpectedVerdicts_24H: 123,
				FlakyVerdicts_24H:      456,
				TotalVerdicts_24H:      1999,
			}), truth.LineContext())
		} else {
			assert.Loosely(t, cf.TestVariantBranch, should.BeNil, truth.LineContext())
		}
	}
}
