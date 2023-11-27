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

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/analysis/internal/clustering/algorithms"
	"go.chromium.org/luci/analysis/internal/clustering/rules"
	"go.chromium.org/luci/analysis/internal/clustering/shards"
	"go.chromium.org/luci/analysis/internal/config"
	"go.chromium.org/luci/analysis/internal/testutil"
)

func TestSpan(t *testing.T) {
	Convey(`With Spanner Test Database`, t, func() {
		ctx := testutil.SpannerTestContext(t)

		Convey(`Reads`, func() {
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
			err := SetRunsForTesting(ctx, runs)
			So(err, ShouldBeNil)

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

			Convey(`Read`, func() {
				Convey(`Not Exists`, func() {
					run, err := Read(span.Single(ctx), testProject, reference)
					So(err, ShouldEqual, NotFound)
					So(run, ShouldBeNil)
				})
				Convey(`Exists`, func() {
					run, err := Read(span.Single(ctx), testProject, runs[2].AttemptTimestamp)
					So(err, ShouldBeNil)
					So(run, ShouldResemble, runs[2])
				})
			})
			Convey(`ReadLastUpTo`, func() {
				Convey(`Not Exists`, func() {
					run, err := ReadLastUpTo(span.Single(ctx), "emptyproject", MaxAttemptTimestamp)
					So(err, ShouldBeNil)
					So(run, ShouldResemble, expectedFake)
				})
				Convey(`Latest`, func() {
					run, err := ReadLastUpTo(span.Single(ctx), testProject, MaxAttemptTimestamp)
					So(err, ShouldBeNil)
					So(run, ShouldResemble, runs[1])
				})
				Convey(`Stale`, func() {
					run, err := ReadLastUpTo(span.Single(ctx), testProject, reference.Add(-10*time.Minute))
					So(err, ShouldBeNil)
					So(run, ShouldResemble, runs[2])
				})
			})
			Convey(`ReadLastWithProgressUpTo`, func() {
				Convey(`Not Exists`, func() {
					run, err := ReadLastWithProgressUpTo(span.Single(ctx), "emptyproject", MaxAttemptTimestamp)
					So(err, ShouldBeNil)
					So(run, ShouldResemble, expectedFake)
				})
				Convey(`Exists`, func() {
					run, err := ReadLastWithProgressUpTo(span.Single(ctx), testProject, MaxAttemptTimestamp)
					So(err, ShouldBeNil)
					So(run, ShouldResemble, runs[3])
				})
				Convey(`Stale`, func() {
					run, err := ReadLastWithProgressUpTo(span.Single(ctx), testProject, reference.Add(-30*time.Minute))
					So(err, ShouldBeNil)
					So(run, ShouldResemble, runs[4])
				})
			})
			Convey(`ReadLastCompleteUpTo`, func() {
				Convey(`Not Exists`, func() {
					run, err := ReadLastCompleteUpTo(span.Single(ctx), "emptyproject", MaxAttemptTimestamp)
					So(err, ShouldBeNil)
					So(run, ShouldResemble, expectedFake)
				})
				Convey(`Exists`, func() {
					run, err := ReadLastCompleteUpTo(span.Single(ctx), testProject, MaxAttemptTimestamp)
					So(err, ShouldBeNil)
					So(run, ShouldResemble, runs[5])
				})
				Convey(`Stale`, func() {
					run, err := ReadLastCompleteUpTo(span.Single(ctx), testProject, reference.Add(-50*time.Minute))
					So(err, ShouldBeNil)
					So(run, ShouldResemble, runs[6])
				})
			})
		})
		Convey(`Query Progress`, func() {
			Convey(`Rule Progress`, func() {
				rulesVersion := time.Date(2021, time.January, 1, 1, 0, 0, 0, time.UTC)

				reference := time.Date(2020, time.January, 1, 1, 0, 0, 0, time.UTC)
				runs := []*ReclusteringRun{
					NewRun(0).WithAttemptTimestamp(reference.Add(-5 * time.Minute)).WithRulesVersion(rulesVersion).WithNoReportedProgress().Build(),
					NewRun(1).WithAttemptTimestamp(reference.Add(-10 * time.Minute)).WithRulesVersion(rulesVersion).WithReportedProgress(500).Build(),
					NewRun(2).WithAttemptTimestamp(reference.Add(-20 * time.Minute)).WithRulesVersion(rulesVersion.Add(-1 * time.Hour)).WithCompletedProgress().Build(),
				}
				err := SetRunsForTesting(ctx, runs)
				So(err, ShouldBeNil)

				progress, err := ReadReclusteringProgress(ctx, testProject)
				So(err, ShouldBeNil)

				So(progress.IncorporatesRulesVersion(rulesVersion.Add(1*time.Hour)), ShouldBeFalse)
				So(progress.IncorporatesRulesVersion(rulesVersion), ShouldBeFalse)
				So(progress.IncorporatesRulesVersion(rulesVersion.Add(-1*time.Minute)), ShouldBeFalse)
				So(progress.IncorporatesRulesVersion(rulesVersion.Add(-1*time.Hour)), ShouldBeTrue)
				So(progress.IncorporatesRulesVersion(rulesVersion.Add(-2*time.Hour)), ShouldBeTrue)
			})
			Convey(`Algorithms Upgrading`, func() {
				reference := time.Date(2020, time.January, 1, 1, 0, 0, 0, time.UTC)
				runs := []*ReclusteringRun{
					NewRun(0).WithAttemptTimestamp(reference.Add(-5 * time.Minute)).WithAlgorithmsVersion(algorithms.AlgorithmsVersion + 1).WithNoReportedProgress().Build(),
					NewRun(1).WithAttemptTimestamp(reference.Add(-10 * time.Minute)).WithAlgorithmsVersion(algorithms.AlgorithmsVersion + 1).WithReportedProgress(500).Build(),
					NewRun(2).WithAttemptTimestamp(reference.Add(-20 * time.Minute)).WithAlgorithmsVersion(algorithms.AlgorithmsVersion).WithCompletedProgress().Build(),
				}
				err := SetRunsForTesting(ctx, runs)
				So(err, ShouldBeNil)

				progress, err := ReadReclusteringProgress(ctx, testProject)
				So(err, ShouldBeNil)

				So(progress.Next.AlgorithmsVersion, ShouldEqual, algorithms.AlgorithmsVersion+1)
				So(progress.IsReclusteringToNewAlgorithms(), ShouldBeTrue)
			})
			Convey(`Config Upgrading`, func() {
				configVersion := time.Date(2025, time.January, 1, 1, 0, 0, 0, time.UTC)

				reference := time.Date(2020, time.January, 1, 1, 0, 0, 0, time.UTC)
				runs := []*ReclusteringRun{
					NewRun(0).WithAttemptTimestamp(reference.Add(-5 * time.Minute)).WithConfigVersion(configVersion).WithNoReportedProgress().Build(),
					NewRun(1).WithAttemptTimestamp(reference.Add(-10 * time.Minute)).WithConfigVersion(configVersion).WithReportedProgress(500).Build(),
					NewRun(2).WithAttemptTimestamp(reference.Add(-20 * time.Minute)).WithConfigVersion(configVersion.Add(-1 * time.Hour)).WithCompletedProgress().Build(),
				}
				err := SetRunsForTesting(ctx, runs)
				So(err, ShouldBeNil)

				progress, err := ReadReclusteringProgress(ctx, testProject)
				So(err, ShouldBeNil)

				So(progress.Next.ConfigVersion, ShouldEqual, configVersion)
				So(progress.IsReclusteringToNewConfig(), ShouldBeTrue)
			})
			Convey(`Algorithms and Config Stable`, func() {
				reference := time.Date(2020, time.January, 1, 1, 0, 0, 0, time.UTC)
				configVersion := time.Date(2025, time.January, 1, 1, 0, 0, 0, time.UTC)
				algVersion := int64(2)
				runs := []*ReclusteringRun{
					NewRun(0).WithAttemptTimestamp(reference.Add(-5 * time.Minute)).WithAlgorithmsVersion(algVersion).WithConfigVersion(configVersion).WithNoReportedProgress().Build(),
					NewRun(1).WithAttemptTimestamp(reference.Add(-10 * time.Minute)).WithAlgorithmsVersion(algVersion).WithConfigVersion(configVersion).WithReportedProgress(500).Build(),
					NewRun(2).WithAttemptTimestamp(reference.Add(-20 * time.Minute)).WithAlgorithmsVersion(algVersion).WithConfigVersion(configVersion).WithCompletedProgress().Build(),
				}
				err := SetRunsForTesting(ctx, runs)
				So(err, ShouldBeNil)

				progress, err := ReadReclusteringProgress(ctx, testProject)
				So(err, ShouldBeNil)

				So(progress.Next.AlgorithmsVersion, ShouldEqual, algVersion)
				So(progress.Next.ConfigVersion, ShouldEqual, configVersion)
				So(progress.IsReclusteringToNewAlgorithms(), ShouldBeFalse)
				So(progress.IsReclusteringToNewConfig(), ShouldBeFalse)
			})
			Convey(`Stale Progress Read`, func() {
				rulesVersion := time.Date(2021, time.January, 1, 1, 0, 0, 0, time.UTC)

				reference := time.Date(2020, time.January, 1, 1, 0, 0, 0, time.UTC)
				runs := []*ReclusteringRun{
					NewRun(0).WithAttemptTimestamp(reference.Add(-1 * time.Minute)).WithRulesVersion(rulesVersion.Add(1 * time.Hour)).WithCompletedProgress().Build(),
					NewRun(1).WithAttemptTimestamp(reference.Add(-5 * time.Minute)).WithRulesVersion(rulesVersion).WithNoReportedProgress().Build(),
					NewRun(2).WithAttemptTimestamp(reference.Add(-10 * time.Minute)).WithRulesVersion(rulesVersion).WithReportedProgress(500).Build(),
					NewRun(3).WithAttemptTimestamp(reference.Add(-20 * time.Minute)).WithRulesVersion(rulesVersion.Add(-1 * time.Hour)).WithCompletedProgress().Build(),
				}
				err := SetRunsForTesting(ctx, runs)
				So(err, ShouldBeNil)

				progress, err := ReadReclusteringProgressUpTo(ctx, testProject, reference.Add(-2*time.Minute))
				So(err, ShouldBeNil)

				So(progress.IncorporatesRulesVersion(rulesVersion), ShouldBeFalse)
				So(progress.IncorporatesRulesVersion(rulesVersion.Add(-1*time.Hour)), ShouldBeTrue)
			})
			Convey(`Uses live progress`, func() {
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
				err := SetRunsForTesting(ctx, runs)
				So(err, ShouldBeNil)

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

				Convey(`No live progress available`, func() {
					// No reclustering shard entries to use to calculate progress.

					progress, err := ReadReclusteringProgress(ctx, testProject)
					So(err, ShouldBeNil)

					So(progress, ShouldResemble, expectedProgress)
				})
				Convey(`No shard progress`, func() {
					// Not all shards have reported progress.
					shds := []shards.ReclusteringShard{
						shards.NewShard(0).WithProject(testProject).WithAttemptTimestamp(reference.Add(-1 * time.Minute)).WithNoProgress().Build(),
						shards.NewShard(1).WithProject(testProject).WithAttemptTimestamp(reference.Add(-1 * time.Minute)).WithProgress(100).Build(),
					}
					err := shards.SetShardsForTesting(ctx, shds)
					So(err, ShouldBeNil)

					progress, err := ReadReclusteringProgress(ctx, testProject)
					So(err, ShouldBeNil)

					So(progress, ShouldResemble, expectedProgress)
				})
				Convey(`Partial progress`, func() {
					// All shards have reported partial progress.
					shds := []shards.ReclusteringShard{
						shards.NewShard(0).WithProject(testProject).WithAttemptTimestamp(reference.Add(-1 * time.Minute)).WithProgress(200).Build(),
						shards.NewShard(1).WithProject(testProject).WithAttemptTimestamp(reference.Add(-1 * time.Minute)).WithProgress(100).Build(),
					}
					err := shards.SetShardsForTesting(ctx, shds)
					So(err, ShouldBeNil)

					expectedProgress.ProgressPerMille = 150
					progress, err := ReadReclusteringProgress(ctx, testProject)
					So(err, ShouldBeNil)
					So(progress, ShouldResemble, expectedProgress)
				})
				Convey(`Complete progress`, func() {
					// All shards have reported progress as being complete.
					shds := []shards.ReclusteringShard{
						shards.NewShard(0).WithProject(testProject).WithAttemptTimestamp(reference.Add(-1 * time.Minute)).WithProgress(shards.MaxProgress).Build(),
						shards.NewShard(1).WithProject(testProject).WithAttemptTimestamp(reference.Add(-1 * time.Minute)).WithProgress(shards.MaxProgress).Build(),
					}
					err := shards.SetShardsForTesting(ctx, shds)
					So(err, ShouldBeNil)

					expectedProgress.ProgressPerMille = 1000
					expectedProgress.Last = ReclusteringTarget{
						RulesVersion:      rulesVersion,
						ConfigVersion:     configVersion,
						AlgorithmsVersion: algorithms.AlgorithmsVersion + 1,
					}
					progress, err := ReadReclusteringProgress(ctx, testProject)
					So(err, ShouldBeNil)
					So(progress, ShouldResemble, expectedProgress)
				})
			})
		})
		Convey(`UpdateProgress`, func() {
			reference := time.Date(2020, time.January, 1, 1, 0, 0, 0, time.UTC)
			assertProgress := func(shardsReported, progress int64) {
				run, err := Read(span.Single(ctx), testProject, reference)
				So(err, ShouldBeNil)
				So(run.ShardsReported, ShouldEqual, shardsReported)
				So(run.Progress, ShouldEqual, progress)
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
			err := SetRunsForTesting(ctx, runs)
			So(err, ShouldBeNil)

			err = updateProgress(0, 0)
			So(err, ShouldBeNil)
			assertProgress(0, 0)

			err = updateProgress(2, 2000)
			So(err, ShouldBeNil)
			assertProgress(2, 2000)
		})
		Convey(`Create`, func() {
			testCreate := func(bc *ReclusteringRun) error {
				_, err := span.ReadWriteTransaction(ctx, func(ctx context.Context) error {
					return Create(ctx, bc)
				})
				return err
			}
			r := NewRun(100).Build()
			Convey(`Valid`, func() {
				testExists := func(expectedRun *ReclusteringRun) {
					txn, cancel := span.ReadOnlyTransaction(ctx)
					defer cancel()
					run, err := Read(txn, expectedRun.Project, expectedRun.AttemptTimestamp)

					So(err, ShouldBeNil)
					So(run, ShouldResemble, expectedRun)
				}

				err := testCreate(r)
				So(err, ShouldBeNil)
				testExists(r)
			})
			Convey(`With invalid Project`, func() {
				Convey(`Unspecified`, func() {
					r.Project = ""
					err := testCreate(r)
					So(err, ShouldErrLike, "project: unspecified")
				})
				Convey(`Invalid`, func() {
					r.Project = "!"
					err := testCreate(r)
					So(err, ShouldErrLike, `project: must match ^[a-z0-9\-]{1,40}$`)
				})
			})
			Convey(`With invalid Attempt Timestamp`, func() {
				r.AttemptTimestamp = time.Time{}
				err := testCreate(r)
				So(err, ShouldErrLike, "attempt timestamp must be valid")
			})
			Convey(`With invalid Algorithms Version`, func() {
				r.AlgorithmsVersion = 0
				err := testCreate(r)
				So(err, ShouldErrLike, "algorithms version must be valid")
			})
			Convey(`With invalid Rules Version`, func() {
				r.RulesVersion = time.Time{}
				err := testCreate(r)
				So(err, ShouldErrLike, "rules version must be valid")
			})
			Convey(`With invalid Shard Count`, func() {
				r.ShardCount = 0
				err := testCreate(r)
				So(err, ShouldErrLike, "shard count must be valid")
			})
			Convey(`With invalid Shards Reported`, func() {
				r.ShardsReported = r.ShardCount + 1
				err := testCreate(r)
				So(err, ShouldErrLike, "shards reported must be valid")
			})
			Convey(`With invalid Progress`, func() {
				r.Progress = r.ShardCount*1000 + 1
				err := testCreate(r)
				So(err, ShouldErrLike, "progress must be valid")
			})
		})
	})
}
