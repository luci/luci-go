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

package runs

import (
	"context"
	"testing"
	"time"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/analysis/internal/clustering/algorithms"
	"go.chromium.org/luci/analysis/internal/clustering/rules"
	"go.chromium.org/luci/analysis/internal/clustering/shards"
	"go.chromium.org/luci/analysis/internal/config"
	"go.chromium.org/luci/analysis/internal/testutil"
)

func TestSpan(t *testing.T) {
	ftt.Run(`With Spanner Test Database`, t, func(t *ftt.Test) {
		ctx := testutil.SpannerTestContext(t)

		t.Run(`Reads`, func(t *ftt.Test) {
			reference := time.Date(2020, time.January, 1, 1, 0, 0, 0, time.UTC)
			runs := []*ReclusteringRun{
				NewRun(0).WithProject("otherproject").WithAttemptTimestamp(reference).WithCompletedProgress().Build(),
				NewRun(1).WithAttemptTimestamp(reference.Add(-5 * time.Minute)).Build(),
				NewRun(2).WithAttemptTimestamp(reference.Add(-10 * time.Minute)).Build(),
				NewRun(3).WithAttemptTimestamp(reference.Add(-20 * time.Minute)).WithReportedProgress(500).Build(),
				NewRun(4).WithAttemptTimestamp(reference.Add(-30 * time.Minute)).WithReportedProgress(500).Build(),
				NewRun(5).WithAttemptTimestamp(reference.Add(-40 * time.Minute)).WithCompletedProgress().Build(),
				NewRun(6).WithAttemptTimestamp(reference.Add(-50 * time.Minute)).WithCompletedProgress().Build(),
			}
			err := SetRunsForTesting(ctx, t, runs)
			assert.Loosely(t, err, should.BeNil)

			// For ReadLast... methods, this is the fake row that is expected
			// to be returned if no row exists.
			expectedFake := &ReclusteringRun{
				Project:           "emptyproject",
				AttemptTimestamp:  time.Date(1900, 1, 1, 0, 0, 0, 0, time.UTC),
				AlgorithmsVersion: 1,
				ConfigVersion:     config.StartingEpoch,
				RulesVersion:      rules.StartingEpoch,
				ShardCount:        1,
				ShardsReported:    1,
				Progress:          1000,
			}

			t.Run(`Read`, func(t *ftt.Test) {
				t.Run(`Not Exists`, func(t *ftt.Test) {
					run, err := Read(span.Single(ctx), testProject, reference)
					assert.Loosely(t, err, should.Equal(NotFound))
					assert.Loosely(t, run, should.BeNil)
				})
				t.Run(`Exists`, func(t *ftt.Test) {
					run, err := Read(span.Single(ctx), testProject, runs[2].AttemptTimestamp)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, run, should.Resemble(runs[2]))
				})
			})
			t.Run(`ReadLastUpTo`, func(t *ftt.Test) {
				t.Run(`Not Exists`, func(t *ftt.Test) {
					run, err := ReadLastUpTo(span.Single(ctx), "emptyproject", MaxAttemptTimestamp)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, run, should.Resemble(expectedFake))
				})
				t.Run(`Latest`, func(t *ftt.Test) {
					run, err := ReadLastUpTo(span.Single(ctx), testProject, MaxAttemptTimestamp)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, run, should.Resemble(runs[1]))
				})
				t.Run(`Stale`, func(t *ftt.Test) {
					run, err := ReadLastUpTo(span.Single(ctx), testProject, reference.Add(-10*time.Minute))
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, run, should.Resemble(runs[2]))
				})
			})
			t.Run(`ReadLastWithProgressUpTo`, func(t *ftt.Test) {
				t.Run(`Not Exists`, func(t *ftt.Test) {
					run, err := ReadLastWithProgressUpTo(span.Single(ctx), "emptyproject", MaxAttemptTimestamp)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, run, should.Resemble(expectedFake))
				})
				t.Run(`Exists`, func(t *ftt.Test) {
					run, err := ReadLastWithProgressUpTo(span.Single(ctx), testProject, MaxAttemptTimestamp)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, run, should.Resemble(runs[3]))
				})
				t.Run(`Stale`, func(t *ftt.Test) {
					run, err := ReadLastWithProgressUpTo(span.Single(ctx), testProject, reference.Add(-30*time.Minute))
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, run, should.Resemble(runs[4]))
				})
			})
			t.Run(`ReadLastCompleteUpTo`, func(t *ftt.Test) {
				t.Run(`Not Exists`, func(t *ftt.Test) {
					run, err := ReadLastCompleteUpTo(span.Single(ctx), "emptyproject", MaxAttemptTimestamp)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, run, should.Resemble(expectedFake))
				})
				t.Run(`Exists`, func(t *ftt.Test) {
					run, err := ReadLastCompleteUpTo(span.Single(ctx), testProject, MaxAttemptTimestamp)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, run, should.Resemble(runs[5]))
				})
				t.Run(`Stale`, func(t *ftt.Test) {
					run, err := ReadLastCompleteUpTo(span.Single(ctx), testProject, reference.Add(-50*time.Minute))
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, run, should.Resemble(runs[6]))
				})
			})
		})
		t.Run(`Query Progress`, func(t *ftt.Test) {
			t.Run(`Rule Progress`, func(t *ftt.Test) {
				rulesVersion := time.Date(2021, time.January, 1, 1, 0, 0, 0, time.UTC)

				reference := time.Date(2020, time.January, 1, 1, 0, 0, 0, time.UTC)
				runs := []*ReclusteringRun{
					NewRun(0).WithAttemptTimestamp(reference.Add(-5 * time.Minute)).WithRulesVersion(rulesVersion).WithNoReportedProgress().Build(),
					NewRun(1).WithAttemptTimestamp(reference.Add(-10 * time.Minute)).WithRulesVersion(rulesVersion).WithReportedProgress(500).Build(),
					NewRun(2).WithAttemptTimestamp(reference.Add(-20 * time.Minute)).WithRulesVersion(rulesVersion.Add(-1 * time.Hour)).WithCompletedProgress().Build(),
				}
				err := SetRunsForTesting(ctx, t, runs)
				assert.Loosely(t, err, should.BeNil)

				progress, err := ReadReclusteringProgress(ctx, testProject)
				assert.Loosely(t, err, should.BeNil)

				assert.Loosely(t, progress.IncorporatesRulesVersion(rulesVersion.Add(1*time.Hour)), should.BeFalse)
				assert.Loosely(t, progress.IncorporatesRulesVersion(rulesVersion), should.BeFalse)
				assert.Loosely(t, progress.IncorporatesRulesVersion(rulesVersion.Add(-1*time.Minute)), should.BeFalse)
				assert.Loosely(t, progress.IncorporatesRulesVersion(rulesVersion.Add(-1*time.Hour)), should.BeTrue)
				assert.Loosely(t, progress.IncorporatesRulesVersion(rulesVersion.Add(-2*time.Hour)), should.BeTrue)
			})
			t.Run(`Algorithms Upgrading`, func(t *ftt.Test) {
				reference := time.Date(2020, time.January, 1, 1, 0, 0, 0, time.UTC)
				runs := []*ReclusteringRun{
					NewRun(0).WithAttemptTimestamp(reference.Add(-5 * time.Minute)).WithAlgorithmsVersion(algorithms.AlgorithmsVersion + 1).WithNoReportedProgress().Build(),
					NewRun(1).WithAttemptTimestamp(reference.Add(-10 * time.Minute)).WithAlgorithmsVersion(algorithms.AlgorithmsVersion + 1).WithReportedProgress(500).Build(),
					NewRun(2).WithAttemptTimestamp(reference.Add(-20 * time.Minute)).WithAlgorithmsVersion(algorithms.AlgorithmsVersion).WithCompletedProgress().Build(),
				}
				err := SetRunsForTesting(ctx, t, runs)
				assert.Loosely(t, err, should.BeNil)

				progress, err := ReadReclusteringProgress(ctx, testProject)
				assert.Loosely(t, err, should.BeNil)

				assert.Loosely(t, progress.Next.AlgorithmsVersion, should.Equal(algorithms.AlgorithmsVersion+1))
				assert.Loosely(t, progress.IsReclusteringToNewAlgorithms(), should.BeTrue)
			})
			t.Run(`Config Upgrading`, func(t *ftt.Test) {
				configVersion := time.Date(2025, time.January, 1, 1, 0, 0, 0, time.UTC)

				reference := time.Date(2020, time.January, 1, 1, 0, 0, 0, time.UTC)
				runs := []*ReclusteringRun{
					NewRun(0).WithAttemptTimestamp(reference.Add(-5 * time.Minute)).WithConfigVersion(configVersion).WithNoReportedProgress().Build(),
					NewRun(1).WithAttemptTimestamp(reference.Add(-10 * time.Minute)).WithConfigVersion(configVersion).WithReportedProgress(500).Build(),
					NewRun(2).WithAttemptTimestamp(reference.Add(-20 * time.Minute)).WithConfigVersion(configVersion.Add(-1 * time.Hour)).WithCompletedProgress().Build(),
				}
				err := SetRunsForTesting(ctx, t, runs)
				assert.Loosely(t, err, should.BeNil)

				progress, err := ReadReclusteringProgress(ctx, testProject)
				assert.Loosely(t, err, should.BeNil)

				assert.That(t, progress.Next.ConfigVersion, should.Match(configVersion))
				assert.Loosely(t, progress.IsReclusteringToNewConfig(), should.BeTrue)
			})
			t.Run(`Algorithms and Config Stable`, func(t *ftt.Test) {
				reference := time.Date(2020, time.January, 1, 1, 0, 0, 0, time.UTC)
				configVersion := time.Date(2025, time.January, 1, 1, 0, 0, 0, time.UTC)
				algVersion := int64(2)
				runs := []*ReclusteringRun{
					NewRun(0).WithAttemptTimestamp(reference.Add(-5 * time.Minute)).WithAlgorithmsVersion(algVersion).WithConfigVersion(configVersion).WithNoReportedProgress().Build(),
					NewRun(1).WithAttemptTimestamp(reference.Add(-10 * time.Minute)).WithAlgorithmsVersion(algVersion).WithConfigVersion(configVersion).WithReportedProgress(500).Build(),
					NewRun(2).WithAttemptTimestamp(reference.Add(-20 * time.Minute)).WithAlgorithmsVersion(algVersion).WithConfigVersion(configVersion).WithCompletedProgress().Build(),
				}
				err := SetRunsForTesting(ctx, t, runs)
				assert.Loosely(t, err, should.BeNil)

				progress, err := ReadReclusteringProgress(ctx, testProject)
				assert.Loosely(t, err, should.BeNil)

				assert.Loosely(t, progress.Next.AlgorithmsVersion, should.Equal(algVersion))
				assert.That(t, progress.Next.ConfigVersion, should.Match(configVersion))
				assert.Loosely(t, progress.IsReclusteringToNewAlgorithms(), should.BeFalse)
				assert.Loosely(t, progress.IsReclusteringToNewConfig(), should.BeFalse)
			})
			t.Run(`Stale Progress Read`, func(t *ftt.Test) {
				rulesVersion := time.Date(2021, time.January, 1, 1, 0, 0, 0, time.UTC)

				reference := time.Date(2020, time.January, 1, 1, 0, 0, 0, time.UTC)
				runs := []*ReclusteringRun{
					NewRun(0).WithAttemptTimestamp(reference.Add(-1 * time.Minute)).WithRulesVersion(rulesVersion.Add(1 * time.Hour)).WithCompletedProgress().Build(),
					NewRun(1).WithAttemptTimestamp(reference.Add(-5 * time.Minute)).WithRulesVersion(rulesVersion).WithNoReportedProgress().Build(),
					NewRun(2).WithAttemptTimestamp(reference.Add(-10 * time.Minute)).WithRulesVersion(rulesVersion).WithReportedProgress(500).Build(),
					NewRun(3).WithAttemptTimestamp(reference.Add(-20 * time.Minute)).WithRulesVersion(rulesVersion.Add(-1 * time.Hour)).WithCompletedProgress().Build(),
				}
				err := SetRunsForTesting(ctx, t, runs)
				assert.Loosely(t, err, should.BeNil)

				progress, err := ReadReclusteringProgressUpTo(ctx, testProject, reference.Add(-2*time.Minute))
				assert.Loosely(t, err, should.BeNil)

				assert.Loosely(t, progress.IncorporatesRulesVersion(rulesVersion), should.BeFalse)
				assert.Loosely(t, progress.IncorporatesRulesVersion(rulesVersion.Add(-1*time.Hour)), should.BeTrue)
			})
			t.Run(`Uses live progress`, func(t *ftt.Test) {
				rulesVersion := time.Date(2021, time.January, 1, 1, 0, 0, 0, time.UTC)
				configVersion := time.Date(2025, time.January, 1, 1, 0, 0, 0, time.UTC)

				reference := time.Date(2020, time.January, 1, 1, 0, 0, 0, time.UTC)
				runs := []*ReclusteringRun{
					NewRun(0).WithAttemptTimestamp(reference.Add(-1 * time.Minute)).
						WithRulesVersion(rulesVersion).
						WithConfigVersion(configVersion).
						WithAlgorithmsVersion(algorithms.AlgorithmsVersion + 1).
						WithShardCount(2).
						WithNoReportedProgress().Build(),
					NewRun(1).WithAttemptTimestamp(reference.Add(-2 * time.Minute)).
						WithRulesVersion(rulesVersion).
						WithConfigVersion(configVersion).
						WithAlgorithmsVersion(algorithms.AlgorithmsVersion + 1).
						WithReportedProgress(500).Build(),
					NewRun(2).WithAttemptTimestamp(reference.Add(-3 * time.Minute)).
						WithRulesVersion(rulesVersion.Add(-1 * time.Hour)).
						WithConfigVersion(configVersion.Add(-2 * time.Hour)).
						WithAlgorithmsVersion(algorithms.AlgorithmsVersion).
						WithCompletedProgress().Build(),
				}
				err := SetRunsForTesting(ctx, t, runs)
				assert.Loosely(t, err, should.BeNil)

				expectedProgress := &ReclusteringProgress{
					ProgressPerMille: 500,
					Next: ReclusteringTarget{
						RulesVersion:      rulesVersion,
						ConfigVersion:     configVersion,
						AlgorithmsVersion: algorithms.AlgorithmsVersion + 1,
					},
					Last: ReclusteringTarget{
						RulesVersion:      rulesVersion.Add(-1 * time.Hour),
						ConfigVersion:     configVersion.Add(-2 * time.Hour),
						AlgorithmsVersion: algorithms.AlgorithmsVersion,
					},
				}

				t.Run(`No live progress available`, func(t *ftt.Test) {
					// No reclustering shard entries to use to calculate progress.

					progress, err := ReadReclusteringProgress(ctx, testProject)
					assert.Loosely(t, err, should.BeNil)

					assert.Loosely(t, progress, should.Resemble(expectedProgress))
				})
				t.Run(`No shard progress`, func(t *ftt.Test) {
					// Not all shards have reported progress.
					shds := []shards.ReclusteringShard{
						shards.NewShard(0).WithProject(testProject).WithAttemptTimestamp(reference.Add(-1 * time.Minute)).WithNoProgress().Build(),
						shards.NewShard(1).WithProject(testProject).WithAttemptTimestamp(reference.Add(-1 * time.Minute)).WithProgress(100).Build(),
					}
					err := shards.SetShardsForTesting(ctx, t, shds)
					assert.Loosely(t, err, should.BeNil)

					progress, err := ReadReclusteringProgress(ctx, testProject)
					assert.Loosely(t, err, should.BeNil)

					assert.Loosely(t, progress, should.Resemble(expectedProgress))
				})
				t.Run(`Partial progress`, func(t *ftt.Test) {
					// All shards have reported partial progress.
					shds := []shards.ReclusteringShard{
						shards.NewShard(0).WithProject(testProject).WithAttemptTimestamp(reference.Add(-1 * time.Minute)).WithProgress(200).Build(),
						shards.NewShard(1).WithProject(testProject).WithAttemptTimestamp(reference.Add(-1 * time.Minute)).WithProgress(100).Build(),
					}
					err := shards.SetShardsForTesting(ctx, t, shds)
					assert.Loosely(t, err, should.BeNil)

					expectedProgress.ProgressPerMille = 150
					progress, err := ReadReclusteringProgress(ctx, testProject)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, progress, should.Resemble(expectedProgress))
				})
				t.Run(`Complete progress`, func(t *ftt.Test) {
					// All shards have reported progress as being complete.
					shds := []shards.ReclusteringShard{
						shards.NewShard(0).WithProject(testProject).WithAttemptTimestamp(reference.Add(-1 * time.Minute)).WithProgress(shards.MaxProgress).Build(),
						shards.NewShard(1).WithProject(testProject).WithAttemptTimestamp(reference.Add(-1 * time.Minute)).WithProgress(shards.MaxProgress).Build(),
					}
					err := shards.SetShardsForTesting(ctx, t, shds)
					assert.Loosely(t, err, should.BeNil)

					expectedProgress.ProgressPerMille = 1000
					expectedProgress.Last = ReclusteringTarget{
						RulesVersion:      rulesVersion,
						ConfigVersion:     configVersion,
						AlgorithmsVersion: algorithms.AlgorithmsVersion + 1,
					}
					progress, err := ReadReclusteringProgress(ctx, testProject)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, progress, should.Resemble(expectedProgress))
				})
			})
		})
		t.Run(`UpdateProgress`, func(t *ftt.Test) {
			reference := time.Date(2020, time.January, 1, 1, 0, 0, 0, time.UTC)
			assertProgress := func(shardsReported, progress int64) {
				run, err := Read(span.Single(ctx), testProject, reference)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, run.ShardsReported, should.Equal(shardsReported))
				assert.Loosely(t, run.Progress, should.Equal(progress))
			}
			updateProgress := func(shardsReported, progress int64) error {
				_, err := span.ReadWriteTransaction(ctx, func(ctx context.Context) error {
					return UpdateProgress(ctx, testProject, reference, shardsReported, progress)
				})
				return err
			}

			runs := []*ReclusteringRun{
				NewRun(0).WithAttemptTimestamp(reference).WithShardCount(2).WithNoReportedProgress().Build(),
			}
			err := SetRunsForTesting(ctx, t, runs)
			assert.Loosely(t, err, should.BeNil)

			err = updateProgress(0, 0)
			assert.Loosely(t, err, should.BeNil)
			assertProgress(0, 0)

			err = updateProgress(2, 2000)
			assert.Loosely(t, err, should.BeNil)
			assertProgress(2, 2000)
		})
		t.Run(`Create`, func(t *ftt.Test) {
			testCreate := func(bc *ReclusteringRun) error {
				_, err := span.ReadWriteTransaction(ctx, func(ctx context.Context) error {
					return Create(ctx, bc)
				})
				return err
			}
			r := NewRun(100).Build()
			t.Run(`Valid`, func(t *ftt.Test) {
				testExists := func(expectedRun *ReclusteringRun) {
					txn, cancel := span.ReadOnlyTransaction(ctx)
					defer cancel()
					run, err := Read(txn, expectedRun.Project, expectedRun.AttemptTimestamp)

					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, run, should.Resemble(expectedRun))
				}

				err := testCreate(r)
				assert.Loosely(t, err, should.BeNil)
				testExists(r)
			})
			t.Run(`With invalid Project`, func(t *ftt.Test) {
				t.Run(`Unspecified`, func(t *ftt.Test) {
					r.Project = ""
					err := testCreate(r)
					assert.Loosely(t, err, should.ErrLike("project: unspecified"))
				})
				t.Run(`Invalid`, func(t *ftt.Test) {
					r.Project = "!"
					err := testCreate(r)
					assert.Loosely(t, err, should.ErrLike(`project: must match ^[a-z0-9\-]{1,40}$`))
				})
			})
			t.Run(`With invalid Attempt Timestamp`, func(t *ftt.Test) {
				r.AttemptTimestamp = time.Time{}
				err := testCreate(r)
				assert.Loosely(t, err, should.ErrLike("attempt timestamp must be valid"))
			})
			t.Run(`With invalid Algorithms Version`, func(t *ftt.Test) {
				r.AlgorithmsVersion = 0
				err := testCreate(r)
				assert.Loosely(t, err, should.ErrLike("algorithms version must be valid"))
			})
			t.Run(`With invalid Rules Version`, func(t *ftt.Test) {
				r.RulesVersion = time.Time{}
				err := testCreate(r)
				assert.Loosely(t, err, should.ErrLike("rules version must be valid"))
			})
			t.Run(`With invalid Shard Count`, func(t *ftt.Test) {
				r.ShardCount = 0
				err := testCreate(r)
				assert.Loosely(t, err, should.ErrLike("shard count must be valid"))
			})
			t.Run(`With invalid Shards Reported`, func(t *ftt.Test) {
				r.ShardsReported = r.ShardCount + 1
				err := testCreate(r)
				assert.Loosely(t, err, should.ErrLike("shards reported must be valid"))
			})
			t.Run(`With invalid Progress`, func(t *ftt.Test) {
				r.Progress = r.ShardCount*1000 + 1
				err := testCreate(r)
				assert.Loosely(t, err, should.ErrLike("progress must be valid"))
			})
		})
	})
}
