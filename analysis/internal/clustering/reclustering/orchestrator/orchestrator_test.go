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

package orchestrator

import (
	"context"
	"sort"
	"strings"
	"testing"
	"time"

	"cloud.google.com/go/spanner"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/server/span"
	"go.chromium.org/luci/server/tq"
	"go.chromium.org/luci/server/tq/tqtesting"

	"go.chromium.org/luci/analysis/internal/clustering/algorithms"
	"go.chromium.org/luci/analysis/internal/clustering/rules"
	"go.chromium.org/luci/analysis/internal/clustering/runs"
	"go.chromium.org/luci/analysis/internal/clustering/shards"
	"go.chromium.org/luci/analysis/internal/clustering/state"
	"go.chromium.org/luci/analysis/internal/config"
	"go.chromium.org/luci/analysis/internal/tasks/taskspb"
	"go.chromium.org/luci/analysis/internal/testutil"
	configpb "go.chromium.org/luci/analysis/proto/config"

	_ "go.chromium.org/luci/server/tq/txn/spanner"
)

func TestOrchestrator(t *testing.T) {
	ftt.Run(`With Spanner Test Database`, t, func(t *ftt.Test) {
		ctx := testutil.IntegrationTestContext(t)

		// Simulate the Orchestrator job running one second past the hour.
		startTime := testclock.TestRecentTimeUTC.Truncate(time.Hour).Add(time.Second)
		ctx, tc := testclock.UseTime(ctx, startTime)

		ctx = memory.Use(ctx) // For config cache.
		ctx, skdr := tq.TestingContext(ctx, nil)

		cfg := &configpb.Config{
			ReclusteringWorkers: 5,
		}
		config.SetTestConfig(ctx, cfg)

		testProjects := []string{"project-a", "project-b", "project-c"}

		testOrchestratorDoesNothing := func(t testing.TB) {
			t.Helper()
			beforeTasks := tasks(skdr)
			beforeRuns := readRuns(ctx, t, testProjects)

			err := CronHandler(ctx)
			assert.Loosely(t, err, should.BeNil)

			afterTasks := tasks(skdr)
			afterRuns := readRuns(ctx, t, testProjects)
			assert.Loosely(t, afterTasks, should.Match(beforeTasks), truth.LineContext())
			assert.Loosely(t, afterRuns, should.Match(beforeRuns), truth.LineContext())
		}

		t.Run("Without Projects", func(t *ftt.Test) {
			testutil.MustApply(ctx, t,
				spanner.Delete("ClusteringState", spanner.AllKeys()))

			testOrchestratorDoesNothing(t)
		})
		t.Run("With Projects", func(t *ftt.Test) {
			// Orchestrator only looks at the projects in ClusteringState table.
			var projectEntries []*state.Entry
			for _, p := range testProjects {
				projectEntries = append(projectEntries, state.NewEntry(0).WithProject(p).Build())
			}
			_, err := state.CreateEntriesForTesting(ctx, projectEntries)
			assert.Loosely(t, err, should.BeNil)

			// Some projects have config.
			configVersionA := time.Date(2029, time.April, 1, 0, 0, 0, 1, time.UTC)
			configVersionB := time.Date(2029, time.May, 1, 0, 0, 0, 1, time.UTC)
			projectCfg := make(map[string]*configpb.ProjectConfig)
			projectCfg["project-a"] = &configpb.ProjectConfig{
				LastUpdated: timestamppb.New(configVersionA),
			}
			projectCfg["project-b"] = &configpb.ProjectConfig{
				LastUpdated: timestamppb.New(configVersionB),
			}
			config.SetTestProjectConfig(ctx, projectCfg)

			// Create chunks in project-b. After this, the row estimates
			// for the projects should be:
			// project-a: ~100
			// project-b: ~450
			// project-c: ~100
			var entries []*state.Entry
			for i := 1; i < 450; i++ {
				entries = append(entries, state.NewEntry(i).WithProject("project-b").Build())
			}
			_, err = state.CreateEntriesForTesting(ctx, entries)
			assert.Loosely(t, err, should.BeNil)

			rulesVersionB := time.Date(2020, time.January, 10, 9, 8, 7, 0, time.UTC)
			rule := rules.NewRule(1).WithProject("project-b").WithPredicateLastUpdateTime(rulesVersionB).Build()
			err = rules.SetForTesting(ctx, t, []*rules.Entry{rule})
			assert.Loosely(t, err, should.BeNil)

			expectedRunStartTime := tc.Now().Truncate(time.Minute)
			expectedRunEndTime := expectedRunStartTime.Add(time.Minute)
			expectedTasks := []*taskspb.ReclusterChunks{
				{
					Project:      "project-a",
					AttemptTime:  timestamppb.New(expectedRunEndTime),
					StartChunkId: "",
					EndChunkId:   state.EndOfTable,
					State: &taskspb.ReclusterChunkState{
						CurrentChunkId: "",
						NextReportDue:  timestamppb.New(expectedRunStartTime),
					},
					ShardNumber: 1,
				},
				{
					Project:      "project-b",
					AttemptTime:  timestamppb.New(expectedRunEndTime),
					StartChunkId: "",
					EndChunkId:   strings.Repeat("55", 15) + "54",
					State: &taskspb.ReclusterChunkState{
						CurrentChunkId: "",
						NextReportDue:  timestamppb.New(expectedRunStartTime),
					},
					ShardNumber: 2,
				},
				{
					Project:      "project-b",
					AttemptTime:  timestamppb.New(expectedRunEndTime),
					StartChunkId: strings.Repeat("55", 15) + "54",
					EndChunkId:   strings.Repeat("aa", 15) + "a9",
					State: &taskspb.ReclusterChunkState{
						CurrentChunkId: strings.Repeat("55", 15) + "54",
						NextReportDue:  timestamppb.New(expectedRunStartTime.Add(5 * time.Second / 3)),
					},
					ShardNumber: 3,
				},
				{
					Project:      "project-b",
					AttemptTime:  timestamppb.New(expectedRunEndTime),
					StartChunkId: strings.Repeat("aa", 15) + "a9",
					EndChunkId:   state.EndOfTable,
					State: &taskspb.ReclusterChunkState{
						CurrentChunkId: strings.Repeat("aa", 15) + "a9",
						NextReportDue:  timestamppb.New(expectedRunStartTime.Add((5 * time.Second * 2) / 3)),
					},
					ShardNumber: 4,
				},
				{
					Project:      "project-c",
					AttemptTime:  timestamppb.New(expectedRunEndTime),
					StartChunkId: "",
					EndChunkId:   state.EndOfTable,
					State: &taskspb.ReclusterChunkState{
						CurrentChunkId: "",
						NextReportDue:  timestamppb.New(expectedRunStartTime),
					},
					ShardNumber: 5,
				},
			}

			expectedShards := []shards.ReclusteringShard{
				{
					ShardNumber:      1,
					AttemptTimestamp: expectedRunEndTime,
					Project:          "project-a",
					Progress:         spanner.NullInt64{},
				},
				{
					ShardNumber:      2,
					AttemptTimestamp: expectedRunEndTime,
					Project:          "project-b",
					Progress:         spanner.NullInt64{},
				},
				{
					ShardNumber:      3,
					AttemptTimestamp: expectedRunEndTime,
					Project:          "project-b",
					Progress:         spanner.NullInt64{},
				},
				{
					ShardNumber:      4,
					AttemptTimestamp: expectedRunEndTime,
					Project:          "project-b",
					Progress:         spanner.NullInt64{},
				},
				{
					ShardNumber:      5,
					AttemptTimestamp: expectedRunEndTime,
					Project:          "project-c",
					Progress:         spanner.NullInt64{},
				},
			}

			expectedRunA := &runs.ReclusteringRun{
				Project:           "project-a",
				AttemptTimestamp:  expectedRunEndTime,
				AlgorithmsVersion: algorithms.AlgorithmsVersion,
				ConfigVersion:     configVersionA,
				RulesVersion:      rules.StartingEpoch,
				ShardCount:        1,
				ShardsReported:    0,
				Progress:          0,
			}
			expectedRunB := &runs.ReclusteringRun{
				Project:           "project-b",
				AttemptTimestamp:  expectedRunEndTime,
				AlgorithmsVersion: algorithms.AlgorithmsVersion,
				ConfigVersion:     configVersionB,
				RulesVersion:      rulesVersionB,
				ShardCount:        3,
				ShardsReported:    0,
				Progress:          0,
			}
			expectedRunC := &runs.ReclusteringRun{
				Project:           "project-c",
				AttemptTimestamp:  expectedRunEndTime,
				AlgorithmsVersion: algorithms.AlgorithmsVersion,
				ConfigVersion:     config.StartingEpoch,
				RulesVersion:      rules.StartingEpoch,
				ShardCount:        1,
				ShardsReported:    0,
				Progress:          0,
			}
			expectedRuns := make(map[string]*runs.ReclusteringRun)
			expectedRuns["project-a"] = expectedRunA
			expectedRuns["project-b"] = expectedRunB
			expectedRuns["project-c"] = expectedRunC

			// updateExpectedTasks sets the Algorithms Version,
			// Rules Version and Config Version of expected tasks
			// to match those of the expected runs.
			updateExpectedTasks := func() {
				for _, t := range expectedTasks {
					run := expectedRuns[t.Project]
					t.AlgorithmsVersion = run.AlgorithmsVersion
					t.RulesVersion = timestamppb.New(run.RulesVersion)
					t.ConfigVersion = timestamppb.New(run.ConfigVersion)
				}
			}
			updateExpectedTasks()

			t.Run("Disabled orchestrator does nothing", func(t *ftt.Test) {
				t.Run("Workers is zero", func(t *ftt.Test) {
					cfg.ReclusteringWorkers = 0
					config.SetTestConfig(ctx, cfg)

					testOrchestratorDoesNothing(t)
				})
			})
			t.Run("Schedules successfully without existing runs", func(t *ftt.Test) {
				err := CronHandler(ctx)
				assert.Loosely(t, err, should.BeNil)

				actualTasks := tasks(skdr)
				assert.Loosely(t, actualTasks, should.Match(expectedTasks))

				actualRuns := readRuns(ctx, t, testProjects)
				assert.Loosely(t, actualRuns, should.Match(expectedRuns))

				actualShards, err := shards.ReadAll(span.Single(ctx))
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, actualShards, should.Match(expectedShards))
			})
			t.Run("Schedules successfully with a previous run", func(t *ftt.Test) {
				previousRunB := &runs.ReclusteringRun{
					Project:           "project-b",
					AttemptTimestamp:  expectedRunEndTime.Add(-1 * time.Minute),
					AlgorithmsVersion: 1,
					ConfigVersion:     configVersionB.Add(-1 * time.Hour),
					RulesVersion:      rulesVersionB.Add(-1 * time.Hour),
					ShardCount:        10,
				}
				var previousShards []shards.ReclusteringShard
				for i := 0; i < 10; i++ {
					previousShards = append(previousShards, shards.ReclusteringShard{
						ShardNumber:      int64(50 + i),
						AttemptTimestamp: expectedRunEndTime.Add(-1 * time.Minute),
						Project:          "project-b",
						Progress:         spanner.NullInt64{Valid: true, Int64: 1000},
					})
				}

				expectedProgress := 10 * 1000
				expectedShardsReported := 10
				test := func() {
					err = CronHandler(ctx)
					assert.Loosely(t, err, should.BeNil)

					// Verify that the previous run had its progress set correctly.
					updatedPreviousRun, err := runs.Read(span.Single(ctx), previousRunB.Project, previousRunB.AttemptTimestamp)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, updatedPreviousRun.Progress, should.Equal(expectedProgress))
					assert.Loosely(t, updatedPreviousRun.ShardsReported, should.Equal(expectedShardsReported))

					// Verify that correct shards were created and that shards
					// from previous runs were deleted.
					actualShards, err := shards.ReadAll(span.Single(ctx))
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, actualShards, should.Match(expectedShards))

					actualTasks := tasks(skdr)
					assert.Loosely(t, actualTasks, should.Match(expectedTasks))

					actualRuns := readRuns(ctx, t, testProjects)
					assert.Loosely(t, actualRuns, should.Match(expectedRuns))
				}

				t.Run("existing complete run", func(t *ftt.Test) {
					err := runs.SetRunsForTesting(ctx, t, []*runs.ReclusteringRun{previousRunB})
					assert.Loosely(t, err, should.BeNil)

					err = shards.SetShardsForTesting(ctx, t, previousShards)
					assert.Loosely(t, err, should.BeNil)

					// A run scheduled after an existing complete run should
					// use the latest algorithms, config and rules available. So
					// our expectations are unchanged.
					test()
				})
				t.Run("existing incomplete run", func(t *ftt.Test) {
					for i := range previousShards {
						previousShards[i].Progress = spanner.NullInt64{Valid: true, Int64: 500}
					}
					expectedProgress = 10 * 500
					expectedShardsReported = 10

					err := runs.SetRunsForTesting(ctx, t, []*runs.ReclusteringRun{previousRunB})
					assert.Loosely(t, err, should.BeNil)

					err = shards.SetShardsForTesting(ctx, t, previousShards)
					assert.Loosely(t, err, should.BeNil)

					sds, err := shards.ReadAll(span.Single(ctx))
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, sds, should.Match(previousShards))

					// Expect the same algorithms and rules version to be used as
					// the previous run, to ensure forward progress (if new rules
					// are being constantly created, we don't want to be
					// reclustering only the beginning of the workers' keyspaces).
					expectedRunB.AlgorithmsVersion = previousRunB.AlgorithmsVersion
					expectedRunB.ConfigVersion = previousRunB.ConfigVersion
					expectedRunB.RulesVersion = previousRunB.RulesVersion
					updateExpectedTasks()
					test()
				})
				t.Run("existing unreported run", func(t *ftt.Test) {
					for i := range previousShards {
						// Assume the shards did not report progress at all.
						previousShards[i].Progress = spanner.NullInt64{}
					}
					expectedProgress = 0
					expectedShardsReported = 0

					err := runs.SetRunsForTesting(ctx, t, []*runs.ReclusteringRun{previousRunB})
					assert.Loosely(t, err, should.BeNil)

					err = shards.SetShardsForTesting(ctx, t, previousShards)
					assert.Loosely(t, err, should.BeNil)

					// Expect the same algorithms and rules version to be used as
					// the previous run, to ensure forward progress (if new rules
					// are being constantly created, we don't want to be
					// reclustering only the beginning of the workers' keyspaces).
					expectedRunB.AlgorithmsVersion = previousRunB.AlgorithmsVersion
					expectedRunB.ConfigVersion = previousRunB.ConfigVersion
					expectedRunB.RulesVersion = previousRunB.RulesVersion
					updateExpectedTasks()
					test()
				})
				t.Run("existing complete run with later algorithms version", func(t *ftt.Test) {
					previousRunB.AlgorithmsVersion = algorithms.AlgorithmsVersion + 5

					err := runs.SetRunsForTesting(ctx, t, []*runs.ReclusteringRun{previousRunB})
					assert.Loosely(t, err, should.BeNil)

					err = shards.SetShardsForTesting(ctx, t, previousShards)
					assert.Loosely(t, err, should.BeNil)

					// If new algorithms are being rolled out, some GAE instances
					// may be running old code. This includes the instance that
					// runs the orchestrator.
					// To simplify reasoning about re-clustering runs, and ensure
					// correctness of re-clustering progress logic, we require
					// the algorithms version of subsequent runs to always be
					// non-decreasing.
					expectedRunB.AlgorithmsVersion = previousRunB.AlgorithmsVersion
					updateExpectedTasks()
					test()
				})
				t.Run("existing complete run with later config version", func(t *ftt.Test) {
					previousRunB.ConfigVersion = configVersionB.Add(time.Hour)

					err := runs.SetRunsForTesting(ctx, t, []*runs.ReclusteringRun{previousRunB})
					assert.Loosely(t, err, should.BeNil)

					err = shards.SetShardsForTesting(ctx, t, previousShards)
					assert.Loosely(t, err, should.BeNil)

					// If new config is being rolled out, some GAE instances
					// may still have old config cached. This includes the instance
					// that runs the orchestrator.
					// To simplify reasoning about re-clustering runs, and ensure
					// correctness of re-clustering progress logic, we require
					// the config version of subsequent runs to always be
					// non-decreasing.
					expectedRunB.ConfigVersion = previousRunB.ConfigVersion
					updateExpectedTasks()
					test()
				})
			})
		})
	})
}

func tasks(s *tqtesting.Scheduler) []*taskspb.ReclusterChunks {
	var tasks []*taskspb.ReclusterChunks
	for _, pl := range s.Tasks().Payloads() {
		task := pl.(*taskspb.ReclusterChunks)
		tasks = append(tasks, task)
	}
	sort.Slice(tasks, func(i, j int) bool {
		return tasks[i].ShardNumber < tasks[j].ShardNumber
	})
	return tasks
}

func readRuns(ctx context.Context, t testing.TB, projects []string) map[string]*runs.ReclusteringRun {
	t.Helper()
	txn, cancel := span.ReadOnlyTransaction(ctx)
	defer cancel()

	result := make(map[string]*runs.ReclusteringRun)
	for _, project := range projects {
		run, err := runs.ReadLastUpTo(txn, project, runs.MaxAttemptTimestamp)
		assert.Loosely(t, err, should.BeNil, truth.LineContext())
		result[project] = run
	}
	return result
}
