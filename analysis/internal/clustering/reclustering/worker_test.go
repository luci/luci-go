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

package reclustering

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"testing"
	"time"

	"cloud.google.com/go/spanner"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/server/caching"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/analysis/internal/analysis"
	"go.chromium.org/luci/analysis/internal/analysis/clusteredfailures"
	"go.chromium.org/luci/analysis/internal/clustering"
	"go.chromium.org/luci/analysis/internal/clustering/algorithms"
	"go.chromium.org/luci/analysis/internal/clustering/algorithms/failurereason"
	"go.chromium.org/luci/analysis/internal/clustering/algorithms/rulesalgorithm"
	"go.chromium.org/luci/analysis/internal/clustering/algorithms/testname"
	"go.chromium.org/luci/analysis/internal/clustering/chunkstore"
	cpb "go.chromium.org/luci/analysis/internal/clustering/proto"
	"go.chromium.org/luci/analysis/internal/clustering/rules"
	"go.chromium.org/luci/analysis/internal/clustering/rules/cache"
	"go.chromium.org/luci/analysis/internal/clustering/shards"
	"go.chromium.org/luci/analysis/internal/clustering/state"
	"go.chromium.org/luci/analysis/internal/config"
	"go.chromium.org/luci/analysis/internal/config/compiledcfg"
	spanutil "go.chromium.org/luci/analysis/internal/span"
	"go.chromium.org/luci/analysis/internal/tasks/taskspb"
	"go.chromium.org/luci/analysis/internal/testutil"
	"go.chromium.org/luci/analysis/pbutil"
	bqpb "go.chromium.org/luci/analysis/proto/bq"
	configpb "go.chromium.org/luci/analysis/proto/config"
	pb "go.chromium.org/luci/analysis/proto/v1"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

const testProject = "testproject"

// scenario represents a LUCI Analysis system state used for testing.
type scenario struct {
	// clusteringState stores the test result-cluster inclusions
	// for each test result in each chunk, and related metadata.
	clusteringState []*state.Entry
	// netBQExports are the test result-cluster insertions recorded
	// in BigQuery, net of any deletions/updates.
	netBQExports []*bqpb.ClusteredFailureRow
	// config is the clustering configuration.
	config *configpb.Clustering
	// configVersion is the last updated time of the configuration.
	configVersion time.Time
	// rulesVersion is version of failure association rules.
	rulesVersion rules.Version
	// rules are the failure association rules.
	rules []*rules.Entry
	// testResults are the actual test failures ingested by LUCI Analysis,
	// organised in chunks by object ID.
	testResultsByObjectID map[string]*cpb.Chunk
	// noProjectConfig set to true to not set up any project configuration.
	noProjectConfig bool
}

func TestReclustering(t *testing.T) {
	Convey(`With Worker`, t, func() {
		ctx := testutil.IntegrationTestContext(t)
		ctx, tc := testclock.UseTime(ctx, testclock.TestRecentTimeUTC)
		ctx = caching.WithEmptyProcessCache(ctx) // For rules cache.
		ctx = memory.Use(ctx)                    // For project config.

		chunkStore := chunkstore.NewFakeClient()
		clusteredFailures := clusteredfailures.NewFakeClient()
		analysis := analysis.NewClusteringHandler(clusteredFailures)

		worker := NewWorker(chunkStore, analysis)

		runEndTime := tc.Now().Add(time.Minute * 10)
		shard := shards.ReclusteringShard{
			ShardNumber:      5,
			AttemptTimestamp: runEndTime,
			Project:          testProject,
		}
		task := &taskspb.ReclusterChunks{
			ShardNumber:  shard.ShardNumber,
			Project:      testProject,
			AttemptTime:  timestamppb.New(runEndTime),
			StartChunkId: "",
			EndChunkId:   state.EndOfTable,
			State: &taskspb.ReclusterChunkState{
				CurrentChunkId: "",
				NextReportDue:  timestamppb.New(tc.Now()),
			},
			AlgorithmsVersion: algorithms.AlgorithmsVersion,
		}

		setupScenario := func(s *scenario) {
			task.RulesVersion = timestamppb.New(s.rulesVersion.Predicates)
			task.ConfigVersion = timestamppb.New(s.configVersion)

			// Create a shard entry corresponding to the task.
			So(shards.SetShardsForTesting(ctx, []shards.ReclusteringShard{shard}), ShouldBeNil)

			// Set stored failure association rules.
			So(rules.SetForTesting(ctx, s.rules), ShouldBeNil)

			cfg := map[string]*configpb.ProjectConfig{
				testProject: {
					Clustering:  s.config,
					LastUpdated: timestamppb.New(s.configVersion),
				},
			}
			if s.noProjectConfig {
				cfg = map[string]*configpb.ProjectConfig{}
			}
			So(config.SetTestProjectConfig(ctx, cfg), ShouldBeNil)

			// Set stored test result chunks.
			for objectID, chunk := range s.testResultsByObjectID {
				chunkStore.Contents[chunkstore.FileName(testProject, objectID)] = chunk
			}

			// Set stored clustering state.
			commitTime, err := state.CreateEntriesForTesting(ctx, s.clusteringState)
			for _, e := range s.clusteringState {
				e.LastUpdated = commitTime.In(time.UTC)
			}
			So(err, ShouldBeNil)
		}

		Convey(`Re-clustering`, func() {
			testReclustering := func(initial *scenario, expected *scenario) {
				setupScenario(initial)

				// Run the task.
				continuation, err := worker.Do(ctx, task, TargetTaskDuration)
				So(err, ShouldBeNil)
				So(continuation, ShouldBeNil)

				// Final clustering state should be equal expected state.
				actualState, err := state.ReadAllForTesting(ctx, testProject)
				So(err, ShouldBeNil)
				for _, as := range actualState {
					// Clear last updated time to compare actual vs expected
					// state based on row contents, not when the row was updated.
					as.LastUpdated = time.Time{}
				}
				So(actualState, ShouldResemble, expected.clusteringState)

				// BigQuery exports should correctly reflect the new
				// test result-cluster inclusions.
				exports := clusteredFailures.Insertions
				sortBQExport(exports)
				netExports := flattenBigQueryExports(append(initial.netBQExports, exports...))
				So(netExports, ShouldResembleProto, expected.netBQExports)

				// Run is reported as complete.
				actualShards, err := shards.ReadAll(span.Single(ctx))
				So(err, ShouldBeNil)
				So(actualShards, ShouldHaveLength, 1)
				So(actualShards[0].Progress, ShouldResemble, spanner.NullInt64{Valid: true, Int64: 1000})
			}

			Convey("Already up-to-date", func() {
				expected := newScenario().build()

				// Start with up-to-date clustering.
				s := newScenario().build()

				testReclustering(s, expected)

				// Further bound the expected behaviour. Not only
				// should there be zero net changes to the BigQuery
				// export, no changes should be written to BigQuery
				// at all.
				So(clusteredFailures.Insertions, ShouldBeEmpty)
			})
			Convey("From old algorithms", func() {
				expected := newScenario().build()

				// Start with an out of date clustering.
				s := newScenario().withOldAlgorithms(true).build()

				testReclustering(s, expected)
			})
			Convey("From old configuration", func() {
				expected := newScenario().build()

				// Start with clustering based on old configuration.
				s := newScenario().withOldConfig(true).build()
				s.config = expected.config
				s.configVersion = expected.configVersion

				testReclustering(s, expected)
			})
			Convey("From old rules", func() {
				expected := newScenario().build()

				// Start with clustering based on old rules.
				s := newScenario().withOldRules(true).build()
				s.rules = expected.rules
				s.rulesVersion = expected.rulesVersion

				testReclustering(s, expected)
			})
			Convey("From old rules with no project config", func() {
				expected := newScenario().withNoConfig(true).build()

				// Start with clustering based on old rules.
				s := newScenario().withNoConfig(true).withOldRules(true).build()
				s.rules = expected.rules
				s.rulesVersion = expected.rulesVersion

				testReclustering(s, expected)
			})
		})
		Convey(`Worker respects end time`, func() {
			expected := newScenario().build()

			// Start with an out of date clustering.
			s := newScenario().withOldAlgorithms(true).build()
			s.rules = expected.rules
			s.rulesVersion = expected.rulesVersion
			setupScenario(s)

			// Start the worker after the run end time.
			tc.Add(11 * time.Minute)
			So(tc.Now(), ShouldHappenAfter, task.AttemptTime.AsTime())

			// Run the task.
			continuation, err := worker.Do(ctx, task, TargetTaskDuration)
			So(err, ShouldBeNil)
			So(continuation, ShouldBeNil)

			// Clustering state should be same as the initial state.
			actualState, err := state.ReadAllForTesting(ctx, testProject)
			So(err, ShouldBeNil)
			So(actualState, ShouldResemble, s.clusteringState)

			// No changes written to BigQuery.
			So(clusteredFailures.Insertions, ShouldBeEmpty)

			// No progress is reported.
			actualShards, err := shards.ReadAll(span.Single(ctx))
			So(err, ShouldBeNil)
			So(actualShards, ShouldHaveLength, 1)
			So(actualShards[0].Progress, ShouldResemble, spanner.NullInt64{Valid: false, Int64: 0})
		})
		Convey(`Handles update/update races`, func() {
			finalState := newScenario().build()

			// Start with an out of date clustering.
			s := newScenario().withOldAlgorithms(true).build()
			s.rules = finalState.rules
			s.rulesVersion = finalState.rulesVersion
			setupScenario(s)

			// Make reading a chunk's test results trigger updating
			// its clustering state Spanner, to simulate an update/update race.
			chunkIDByObjectID := make(map[string]string)
			for _, state := range s.clusteringState {
				chunkIDByObjectID[state.ObjectID] = state.ChunkID
			}
			chunkStore.GetCallack = func(objectID string) {
				chunkID, ok := chunkIDByObjectID[objectID]

				// Only simulate the update/update race once per chunk.
				if !ok {
					return
				}
				delete(chunkIDByObjectID, objectID)

				_, err := span.ReadWriteTransaction(ctx, func(ctx context.Context) error {
					span.BufferWrite(ctx, spanutil.UpdateMap("ClusteringState", map[string]any{
						"Project": testProject,
						"ChunkID": chunkID,
						// Simulate a race with another update, that
						// re-clustered the chunk to an algorithms version
						// later than the one we know about.
						"AlgorithmsVersion": algorithms.AlgorithmsVersion + 1,
						"LastUpdated":       spanner.CommitTimestamp,
					}))
					return nil
				})
				So(err, ShouldBeNil)
			}

			// Run the worker with time advancing at 100 times speed,
			// as the transaction retry logic sets timers which must be
			// triggered.
			runWithTimeAdvancing(tc, func() {
				continuation, err := worker.Do(ctx, task, TargetTaskDuration)
				So(err, ShouldBeNil)
				So(continuation, ShouldBeNil)
			})

			// Because of update races, none of the chunks should have been
			// re-clustered further.
			expected := newScenario().withOldAlgorithms(true).build()
			for _, es := range expected.clusteringState {
				es.Clustering.AlgorithmsVersion = algorithms.AlgorithmsVersion + 1
			}

			actualState, err := state.ReadAllForTesting(ctx, testProject)
			So(err, ShouldBeNil)
			for _, as := range actualState {
				as.LastUpdated = time.Time{}
			}
			So(actualState, ShouldResemble, expected.clusteringState)

			// No changes written to BigQuery.
			So(clusteredFailures.Insertions, ShouldBeEmpty)

			// Shard is reported as complete.
			actualShards, err := shards.ReadAll(span.Single(ctx))
			So(err, ShouldBeNil)
			So(actualShards, ShouldHaveLength, 1)
			So(actualShards[0].Progress, ShouldResemble, spanner.NullInt64{Valid: true, Int64: 1000})
		})
		Convey(`Worker running out of date algorithms`, func() {
			task.AlgorithmsVersion = algorithms.AlgorithmsVersion + 1
			task.ConfigVersion = timestamppb.New(config.StartingEpoch)
			task.RulesVersion = timestamppb.New(rules.StartingEpoch)

			continuation, err := worker.Do(ctx, task, TargetTaskDuration)
			So(err, ShouldErrLike, "running out-of-date algorithms version")
			So(continuation, ShouldBeNil)
		})
		Convey(`Continuation correctly scheduled`, func() {
			task.RulesVersion = timestamppb.New(rules.StartingEpoch)
			task.ConfigVersion = timestamppb.New(config.StartingEpoch)

			// Leave no time for the task to run.
			continuation, err := worker.Do(ctx, task, 0*time.Second)
			So(err, ShouldBeNil)

			// Continuation should be scheduled, matching original task.
			So(continuation, ShouldResembleProto, task)
		})
	})
}

func TestProgress(t *testing.T) {
	Convey(`Task assigned entire keyspace`, t, func() {
		task := &taskspb.ReclusterChunks{
			StartChunkId: "",
			EndChunkId:   strings.Repeat("ff", 16),
		}

		progress, err := calculateProgress(task, strings.Repeat("00", 16))
		So(err, ShouldBeNil)
		So(progress, ShouldEqual, 0)

		progress, err = calculateProgress(task, "80"+strings.Repeat("00", 15))
		So(err, ShouldBeNil)
		So(progress, ShouldEqual, 500)

		progress, err = calculateProgress(task, strings.Repeat("ff", 16))
		So(err, ShouldBeNil)
		So(progress, ShouldEqual, 999)
	})
	Convey(`Task assigned partial keyspace`, t, func() {
		// Consistent with the second shard, if the keyspace is split into
		// three.
		task := &taskspb.ReclusterChunks{
			StartChunkId: strings.Repeat("55", 15) + "54",
			EndChunkId:   strings.Repeat("aa", 15) + "a9",
		}

		progress, err := calculateProgress(task, strings.Repeat("55", 16))
		So(err, ShouldBeNil)
		So(progress, ShouldEqual, 0)

		progress, err = calculateProgress(task, strings.Repeat("77", 16))
		So(err, ShouldBeNil)
		So(progress, ShouldEqual, 400)

		progress, err = calculateProgress(task, strings.Repeat("aa", 15)+"a9")
		So(err, ShouldBeNil)
		So(progress, ShouldEqual, 999)
	})
}

func runWithTimeAdvancing(tc testclock.TestClock, cb func()) {
	ticker := time.NewTicker(time.Millisecond)
	done := make(chan bool)
	go func() {
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				// Run with time advancing at 100 times speed, to
				// avoid holding up tests unnecessarily.
				tc.Add(time.Millisecond * 100)
			}
		}
	}()

	cb()

	ticker.Stop()
	done <- true
}

// flattenBigQueryExports returns the latest inclusion row for
// each test result-cluster, from a list of BigQuery exports.
// The returned set of rows do not have last updated time set.
func flattenBigQueryExports(exports []*bqpb.ClusteredFailureRow) []*bqpb.ClusteredFailureRow {
	keyValue := make(map[string]*bqpb.ClusteredFailureRow)
	for _, row := range exports {
		key := bigQueryKey(row)
		existingRow, ok := keyValue[key]
		if ok && existingRow.LastUpdated.AsTime().After(row.LastUpdated.AsTime()) {
			continue
		}
		keyValue[key] = row
	}
	var result []*bqpb.ClusteredFailureRow
	for _, row := range keyValue {
		if row.IsIncluded {
			clonedRow := proto.Clone(row).(*bqpb.ClusteredFailureRow)
			clonedRow.LastUpdated = nil
			result = append(result, clonedRow)
		}
	}
	sortBQExport(result)
	return result
}

func bigQueryKey(row *bqpb.ClusteredFailureRow) string {
	return fmt.Sprintf("%q/%q/%q/%q", row.ClusterAlgorithm, row.ClusterId, row.TestResultSystem, row.TestResultId)
}

type testResultBuilder struct {
	uniqifier     int
	failureReason *pb.FailureReason
	testName      string
}

func newTestResult(uniqifier int) *testResultBuilder {
	return &testResultBuilder{
		uniqifier: uniqifier,
		testName:  fmt.Sprintf("ninja://test_name/%v", uniqifier),
		failureReason: &pb.FailureReason{
			PrimaryErrorMessage: fmt.Sprintf("Failure reason %v.", uniqifier),
		},
	}
}

func (b *testResultBuilder) withTestName(name string) *testResultBuilder {
	b.testName = name
	return b
}

func (b *testResultBuilder) withFailureReason(reason *pb.FailureReason) *testResultBuilder {
	b.failureReason = reason
	return b
}

func (b *testResultBuilder) buildFailure() *cpb.Failure {
	keyHash := sha256.Sum256([]byte("variantkey:value\n"))
	buildCritical := b.uniqifier%2 == 0
	return &cpb.Failure{
		TestResultId:  pbutil.TestResultIDFromResultDB(fmt.Sprintf("invocations/testrun-%v/tests/test-name-%v/results/%v", b.uniqifier, b.uniqifier, b.uniqifier)),
		PartitionTime: timestamppb.New(time.Date(2020, time.April, 1, 2, 3, 4, 0, time.UTC)),
		ChunkIndex:    -1, // To be populated by caller.
		Realm:         "testproject:realm",
		TestId:        b.testName,
		Variant:       &pb.Variant{Def: map[string]string{"variantkey": "value"}},
		VariantHash:   hex.EncodeToString(keyHash[:]),
		FailureReason: b.failureReason,
		BugTrackingComponent: &pb.BugTrackingComponent{
			System:    "monorail",
			Component: "Component>MyComponent",
		},
		StartTime: timestamppb.New(time.Date(2025, time.March, 2, 2, 2, 2, b.uniqifier, time.UTC)),
		Duration:  durationpb.New(time.Duration(b.uniqifier) * time.Second),
		Exonerations: []*cpb.TestExoneration{
			{
				Reason: pb.ExonerationReason(1 + (b.uniqifier % 3)),
			},
		},
		PresubmitRun: &cpb.PresubmitRun{
			PresubmitRunId: &pb.PresubmitRunId{
				System: "luci-cv",
				Id:     fmt.Sprintf("run-%v", b.uniqifier),
			},
			Owner:  fmt.Sprintf("owner-%v", b.uniqifier),
			Mode:   pb.PresubmitRunMode(1 + b.uniqifier%3),
			Status: pb.PresubmitRunStatus(3 - b.uniqifier%3),
		},
		BuildStatus:   pb.BuildStatus(1 + b.uniqifier%4),
		BuildCritical: &buildCritical,

		IngestedInvocationId:          fmt.Sprintf("invocation-%v", b.uniqifier),
		IngestedInvocationResultIndex: int64(b.uniqifier + 1),
		IngestedInvocationResultCount: int64(b.uniqifier*2 + 1),
		IsIngestedInvocationBlocked:   b.uniqifier%3 == 0,

		TestRunId:          fmt.Sprintf("test-run-%v", b.uniqifier),
		TestRunResultIndex: int64((int64(b.uniqifier) / 2) + 1),
		TestRunResultCount: int64(b.uniqifier + 1),
		IsTestRunBlocked:   b.uniqifier%2 == 0,
	}
}

// buildBQExport returns the expected test result-cluster inclusion rows that
// would appear in BigQuery, if the test result was in the given clusters.
// Note that deletions are not returned; these are simply the 'net' rows that
// would be expected.
func (b *testResultBuilder) buildBQExport(clusterIDs []clustering.ClusterID) []*bqpb.ClusteredFailureRow {
	keyHash := sha256.Sum256([]byte("variantkey:value\n"))
	var inBugCluster bool
	for _, cID := range clusterIDs {
		if cID.IsBugCluster() {
			inBugCluster = true
		}
	}

	presubmitRunStatus := pb.PresubmitRunStatus(3 - b.uniqifier%3).String()
	if !strings.HasPrefix(presubmitRunStatus, "PRESUBMIT_RUN_STATUS_") {
		panic("PresubmitRunStatus does not have expected prefix: " + presubmitRunStatus)
	}
	presubmitRunStatus = strings.TrimPrefix(presubmitRunStatus, "PRESUBMIT_RUN_STATUS_")

	var results []*bqpb.ClusteredFailureRow
	for _, cID := range clusterIDs {
		result := &bqpb.ClusteredFailureRow{
			ClusterAlgorithm: cID.Algorithm,
			ClusterId:        cID.ID,
			TestResultSystem: "resultdb",
			TestResultId:     fmt.Sprintf("invocations/testrun-%v/tests/test-name-%v/results/%v", b.uniqifier, b.uniqifier, b.uniqifier),
			LastUpdated:      nil, // To be set by caller.
			Project:          testProject,

			PartitionTime:              timestamppb.New(time.Date(2020, time.April, 1, 2, 3, 4, 0, time.UTC)),
			IsIncluded:                 true,
			IsIncludedWithHighPriority: cID.IsBugCluster() || !inBugCluster,

			ChunkId:    "", // To be set by caller.
			ChunkIndex: 0,  // To be set by caller.

			Realm:  "testproject:realm",
			TestId: b.testName,
			Variant: []*pb.StringPair{
				{
					Key:   "variantkey",
					Value: "value",
				},
			},
			VariantHash:   hex.EncodeToString(keyHash[:]),
			FailureReason: b.failureReason,
			BugTrackingComponent: &pb.BugTrackingComponent{
				System:    "monorail",
				Component: "Component>MyComponent",
			},
			StartTime: timestamppb.New(time.Date(2025, time.March, 2, 2, 2, 2, b.uniqifier, time.UTC)),
			Duration:  float64(b.uniqifier * 1.0),
			Exonerations: []*bqpb.ClusteredFailureRow_TestExoneration{
				{
					Reason: pb.ExonerationReason(1 + (b.uniqifier % 3)),
				},
			},
			PresubmitRunId: &pb.PresubmitRunId{
				System: "luci-cv",
				Id:     fmt.Sprintf("run-%v", b.uniqifier),
			},
			PresubmitRunOwner:  fmt.Sprintf("owner-%v", b.uniqifier),
			PresubmitRunMode:   pb.PresubmitRunMode(1 + b.uniqifier%3).String(),
			PresubmitRunStatus: presubmitRunStatus,
			BuildStatus:        strings.TrimPrefix(pb.BuildStatus(1+b.uniqifier%4).String(), "BUILD_STATUS_"),
			BuildCritical:      b.uniqifier%2 == 0,

			IngestedInvocationId:          fmt.Sprintf("invocation-%v", b.uniqifier),
			IngestedInvocationResultIndex: int64(b.uniqifier + 1),
			IngestedInvocationResultCount: int64(b.uniqifier*2 + 1),
			IsIngestedInvocationBlocked:   b.uniqifier%3 == 0,

			TestRunId:          fmt.Sprintf("test-run-%v", b.uniqifier),
			TestRunResultIndex: int64((int64(b.uniqifier) / 2) + 1),
			TestRunResultCount: int64(b.uniqifier + 1),
			IsTestRunBlocked:   b.uniqifier%2 == 0,
		}
		results = append(results, result)
	}
	return results
}

// buildClusters returns the clusters that would be expected for this test
// result, if current clustering algorithms were used.
func (b *testResultBuilder) buildClusters(rules *cache.Ruleset, config *compiledcfg.ProjectConfig) []clustering.ClusterID {
	var clusters []clustering.ClusterID
	failure := &clustering.Failure{
		TestID: b.testName,
		Reason: b.failureReason,
	}
	testNameAlg := &testname.Algorithm{}
	clusters = append(clusters, clustering.ClusterID{
		Algorithm: testNameAlg.Name(),
		ID:        hex.EncodeToString(testNameAlg.Cluster(config, failure)),
	})
	if b.failureReason != nil && b.failureReason.PrimaryErrorMessage != "" {
		failureReasonAlg := &failurereason.Algorithm{}
		clusters = append(clusters, clustering.ClusterID{
			Algorithm: failureReasonAlg.Name(),
			ID:        hex.EncodeToString(failureReasonAlg.Cluster(config, failure)),
		})
	}
	vals := &clustering.Failure{
		TestID: b.testName,
		Reason: &pb.FailureReason{PrimaryErrorMessage: b.failureReason.GetPrimaryErrorMessage()},
	}
	for _, rule := range rules.ActiveRulesSorted {
		if rule.Expr.Evaluate(vals) {
			clusters = append(clusters, clustering.ClusterID{
				Algorithm: rulesalgorithm.AlgorithmName,
				ID:        rule.Rule.RuleID,
			})
		}
	}
	clustering.SortClusters(clusters)
	return clusters
}

// chunkBuilder is used to build a chunk with test results, clustering state
// and BigQuery exports, for testing.
type chunkBuilder struct {
	project       string
	chunkID       string
	objectID      string
	testResults   []*testResultBuilder
	ruleset       *cache.Ruleset
	config        *compiledcfg.ProjectConfig
	oldAlgorithms bool
}

// newChunk returns a new chunkBuilder for creating a new chunk. Uniqifier
// is used to generate a chunk ID.
func newChunk(uniqifier int) *chunkBuilder {
	chunkID := sha256.Sum256([]byte(fmt.Sprintf("chunk-%v", uniqifier)))
	objectID := sha256.Sum256([]byte(fmt.Sprintf("object-%v", uniqifier)))
	config, err := compiledcfg.NewConfig(&configpb.ProjectConfig{
		LastUpdated: timestamppb.New(time.Date(2022, time.January, 1, 0, 0, 0, 0, time.UTC)),
	})
	if err != nil {
		// This should never occur, as the config should be valid.
		panic(err)
	}
	return &chunkBuilder{
		project:       "testproject",
		chunkID:       hex.EncodeToString(chunkID[:16]),
		objectID:      hex.EncodeToString(objectID[:16]),
		ruleset:       cache.NewRuleset("", nil, rules.StartingVersion, time.Time{}),
		config:        config,
		oldAlgorithms: false,
	}
}

func (b *chunkBuilder) withProject(project string) *chunkBuilder {
	b.project = project
	return b
}

func (b *chunkBuilder) withTestResults(tr ...*testResultBuilder) *chunkBuilder {
	b.testResults = tr
	return b
}

// withOldAlgorithms sets whether out of date algorithms
// should be used instead of current clustering.
func (b *chunkBuilder) withOldAlgorithms(old bool) *chunkBuilder {
	b.oldAlgorithms = old
	return b
}

// withRuleset sets the ruleset to use to determine current clustering
// (only used if out-of-date algorithms is not set).
func (b *chunkBuilder) withRuleset(ruleset *cache.Ruleset) *chunkBuilder {
	b.ruleset = ruleset
	return b
}

// withConfig sets the configuration to use to determine current clustering
// (only used if out-of-date algorithms is not set).
func (b *chunkBuilder) withConfig(config *compiledcfg.ProjectConfig) *chunkBuilder {
	b.config = config
	return b
}

func (b *chunkBuilder) buildTestResults() (chunk *cpb.Chunk) {
	var failures []*cpb.Failure
	for i, tr := range b.testResults {
		failure := tr.buildFailure()
		failure.ChunkIndex = int64(i + 1)
		failures = append(failures, failure)
	}
	return &cpb.Chunk{
		Failures: failures,
	}
}

func (b *chunkBuilder) buildState() *state.Entry {
	var crs clustering.ClusterResults
	if b.oldAlgorithms {
		algs := make(map[string]struct{})
		algs["testname-v1"] = struct{}{}
		algs["rules-v1"] = struct{}{}
		var clusters [][]clustering.ClusterID
		for range b.testResults {
			cs := []clustering.ClusterID{
				{
					Algorithm: "testname-v1",
					ID:        "01dc151e01dc151e01dc151e01dc151e",
				},
				{
					Algorithm: "rules-v1",
					ID:        "12341234123412341234123412341234",
				},
			}
			clustering.SortClusters(cs)
			clusters = append(clusters, cs)
		}
		crs = clustering.ClusterResults{
			AlgorithmsVersion: 1,
			ConfigVersion:     b.config.LastUpdated,
			RulesVersion:      b.ruleset.Version.Predicates,
			Algorithms:        algs,
			Clusters:          clusters,
		}
	} else {
		algs := make(map[string]struct{})
		algs[testname.AlgorithmName] = struct{}{}
		algs[failurereason.AlgorithmName] = struct{}{}
		algs[rulesalgorithm.AlgorithmName] = struct{}{}
		var clusters [][]clustering.ClusterID
		for _, tr := range b.testResults {
			clusters = append(clusters, tr.buildClusters(b.ruleset, b.config))
		}
		crs = clustering.ClusterResults{
			AlgorithmsVersion: algorithms.AlgorithmsVersion,
			ConfigVersion:     b.config.LastUpdated,
			RulesVersion:      b.ruleset.Version.Predicates,
			Algorithms:        algs,
			Clusters:          clusters,
		}
	}

	return &state.Entry{
		Project:       b.project,
		ChunkID:       b.chunkID,
		PartitionTime: time.Date(2020, time.April, 1, 2, 3, 4, 0, time.UTC),
		ObjectID:      b.objectID,
		Clustering:    crs,
	}
}

func (b *chunkBuilder) buildBQExport() []*bqpb.ClusteredFailureRow {
	state := b.buildState()
	var result []*bqpb.ClusteredFailureRow
	for i, tr := range b.testResults {
		cIDs := state.Clustering.Clusters[i]
		rows := tr.buildBQExport(cIDs)
		for _, r := range rows {
			r.ChunkId = b.chunkID
			r.ChunkIndex = int64(i + 1)
		}
		result = append(result, rows...)
	}
	return result
}

// scenarioBuilder is used to generate LUCI Analysis system states used for
// testing. Each scenario represents a consistent state of the LUCI Analysis
// system, i.e.
//   - where the clustering state matches the configured rules, and
//   - the BigQuery exports match the clustering state, and the test results
//     in the chunk store.
type scenarioBuilder struct {
	project       string
	chunkCount    int
	oldAlgorithms bool
	oldRules      bool
	oldConfig     bool
	noConfig      bool
}

func newScenario() *scenarioBuilder {
	return &scenarioBuilder{
		project:    testProject,
		chunkCount: 2,
	}
}

func (b *scenarioBuilder) withOldAlgorithms(value bool) *scenarioBuilder {
	b.oldAlgorithms = value
	return b
}

func (b *scenarioBuilder) withOldRules(value bool) *scenarioBuilder {
	b.oldRules = value
	return b
}

func (b *scenarioBuilder) withOldConfig(value bool) *scenarioBuilder {
	b.oldConfig = value
	return b
}

func (b *scenarioBuilder) withNoConfig(value bool) *scenarioBuilder {
	b.noConfig = value
	return b
}

func (b *scenarioBuilder) build() *scenario {
	var rs []*rules.Entry
	var activeRules []*cache.CachedRule

	rulesVersion := rules.Version{
		Predicates: time.Date(2001, time.January, 1, 0, 0, 0, 1000, time.UTC),
		Total:      time.Date(2001, time.January, 1, 0, 0, 0, 2000, time.UTC),
	}
	ruleOne := rules.NewRule(0).WithProject(b.project).
		WithRuleDefinition(`test = "test_b"`).
		WithPredicateLastUpdateTime(rulesVersion.Predicates).
		WithLastUpdateTime(rulesVersion.Total).
		Build()
	rs = []*rules.Entry{ruleOne}
	if !b.oldRules {
		rulesVersion = rules.Version{
			Predicates: time.Date(2002, time.January, 1, 0, 0, 0, 1000, time.UTC),
			Total:      time.Date(2002, time.January, 1, 0, 0, 0, 2000, time.UTC),
		}
		ruleTwo := rules.NewRule(1).WithProject(b.project).
			WithRuleDefinition(`reason = "reason_b"`).
			WithPredicateLastUpdateTime(rulesVersion.Predicates).
			WithLastUpdateTime(rulesVersion.Total).
			Build()
		rs = append(rs, ruleTwo)
	}
	for _, r := range rs {
		active, err := cache.NewCachedRule(r)
		So(err, ShouldBeNil)
		activeRules = append(activeRules, active)
	}

	configVersion := time.Date(2001, time.January, 2, 0, 0, 0, 1, time.UTC)
	cfgpb := &configpb.Clustering{
		TestNameRules: []*configpb.TestNameClusteringRule{
			{
				Name:         "Test underscore clustering",
				Pattern:      `^(?P<name>\w+)_\w+$`,
				LikeTemplate: `${name}%`,
			},
		},
	}
	if !b.oldConfig {
		configVersion = time.Date(2002, time.January, 2, 0, 0, 0, 1, time.UTC)
		cfgpb = &configpb.Clustering{
			TestNameRules: []*configpb.TestNameClusteringRule{
				{
					Name:         "Test underscore clustering",
					Pattern:      `^(?P<name>\w+)_\w+$`,
					LikeTemplate: `${name}\_%`,
				},
			},
		}
	}

	ruleset := cache.NewRuleset(b.project, activeRules, rulesVersion, time.Time{})
	projectCfg := &configpb.ProjectConfig{
		Clustering:  cfgpb,
		LastUpdated: timestamppb.New(configVersion),
	}
	if b.noConfig {
		projectCfg = config.NewEmptyProject()
		configVersion = projectCfg.LastUpdated.AsTime()
	}
	cfg, err := compiledcfg.NewConfig(projectCfg)
	if err != nil {
		// Should never occur as config should be valid.
		panic(err)
	}
	var state []*state.Entry
	testResultsByObjectID := make(map[string]*cpb.Chunk)
	var bqExports []*bqpb.ClusteredFailureRow
	for i := 0; i < b.chunkCount; i++ {
		trOne := newTestResult(i * 2).withFailureReason(&pb.FailureReason{
			PrimaryErrorMessage: "reason_a",
		}).withTestName("test_a")
		trTwo := newTestResult(i*2 + 1).withFailureReason(&pb.FailureReason{
			PrimaryErrorMessage: "reason_b",
		}).withTestName("test_b")

		cb := newChunk(i).withProject(b.project).
			withOldAlgorithms(b.oldAlgorithms).
			withRuleset(ruleset).
			withConfig(cfg).
			withTestResults(trOne, trTwo)

		s := cb.buildState()
		state = append(state, s)
		bqExports = append(bqExports, cb.buildBQExport()...)
		testResultsByObjectID[s.ObjectID] = cb.buildTestResults()
	}
	sortState(state)
	sortBQExport(bqExports)
	return &scenario{
		config:                cfgpb,
		configVersion:         configVersion,
		rulesVersion:          rulesVersion,
		rules:                 rs,
		testResultsByObjectID: testResultsByObjectID,
		clusteringState:       state,
		netBQExports:          bqExports,
		noProjectConfig:       b.noConfig,
	}
}

// sortState sorts state.Entry elements in ascending ChunkID order.
func sortState(state []*state.Entry) {
	sort.Slice(state, func(i, j int) bool {
		return state[i].ChunkID < state[j].ChunkID
	})
}

// sortBQExport sorts BigQuery export rows in ascending key order.
func sortBQExport(rows []*bqpb.ClusteredFailureRow) {
	sort.Slice(rows, func(i, j int) bool {
		return bigQueryKey(rows[i]) < bigQueryKey(rows[j])
	})
}
