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

package aggrmetrics

import (
	"fmt"
	"math/rand"
	"strconv"
	"testing"
	"time"

	"go.chromium.org/luci/common/tsmon/distribution"
	"go.chromium.org/luci/gae/service/datastore"

	cfgpb "go.chromium.org/luci/cv/api/config/v2"
	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/configs/prjcfg"
	"go.chromium.org/luci/cv/internal/configs/prjcfg/prjcfgtest"
	"go.chromium.org/luci/cv/internal/cvtesting"
	"go.chromium.org/luci/cv/internal/metrics"
	"go.chromium.org/luci/cv/internal/run"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestRunAggregator(t *testing.T) {
	t.Parallel()

	Convey("runAggregator works", t, func() {
		ct := cvtesting.Test{}
		ctx, cancel := ct.SetUp(t)
		defer cancel()
		// Truncate current time to seconds to deal with integer delays.
		ct.Clock.Set(ct.Clock.Now().UTC().Add(time.Second).Truncate(time.Second))

		pendingRunCountSent := func(project, configGroup string, mode run.Mode) any {
			return ct.TSMonSentValue(ctx, metrics.Public.PendingRunCount, project, configGroup, string(mode))
		}
		pendingRunDurationSent := func(project, configGroup string, mode run.Mode) *distribution.Distribution {
			return ct.TSMonSentDistr(ctx, metrics.Public.PendingRunDuration, project, configGroup, string(mode))
		}
		maxPendingRunAgeSent := func(project, configGroup string, mode run.Mode) any {
			return ct.TSMonSentValue(ctx, metrics.Public.MaxPendingRunAge, project, configGroup, string(mode))
		}
		activeRunCountSent := func(project, configGroup string, mode run.Mode) any {
			return ct.TSMonSentValue(ctx, metrics.Public.ActiveRunCount, project, configGroup, string(mode))
		}
		activeRunDurationSent := func(project, configGroup string, mode run.Mode) *distribution.Distribution {
			return ct.TSMonSentDistr(ctx, metrics.Public.ActiveRunDuration, project, configGroup, string(mode))
		}

		const lProject = "test_proj"
		const configGroupName = "foo"
		seededRand := rand.New(rand.NewSource(ct.Clock.Now().Unix()))

		prjcfgtest.Create(ctx, lProject, &cfgpb.Config{
			ConfigGroups: []*cfgpb.ConfigGroup{
				{
					Name: configGroupName,
				},
			},
		})

		putRun := func(project, configGroup string, mode run.Mode, s run.Status, ct, st time.Time) {
			err := datastore.Put(ctx, &run.Run{
				ID:            common.MakeRunID(project, ct, 1, []byte(strconv.Itoa(seededRand.Int()))),
				ConfigGroupID: prjcfg.MakeConfigGroupID("deedbeef", configGroup),
				Mode:          mode,
				CreateTime:    ct,
				StartTime:     st,
				Status:        s,
			})
			if err != nil {
				panic(err)
			}
		}

		mustReport := func(active ...string) {
			ra := runsAggregator{}
			err := ra.report(ctx, active)
			So(err, ShouldBeNil)
		}

		Convey("Skip reporting for disabled project", func() {
			prjcfgtest.Disable(ctx, lProject)
			mustReport(lProject)
			So(ct.TSMonStore.GetAll(ctx), ShouldBeEmpty)
		})

		Convey("Enabled projects get zero data reported when no interested Runs", func() {
			mustReport(lProject)
			So(pendingRunCountSent(lProject, configGroupName, run.DryRun), ShouldEqual, 0)
			So(pendingRunCountSent(lProject, configGroupName, run.FullRun), ShouldEqual, 0)
			So(pendingRunDurationSent(lProject, configGroupName, run.DryRun).Count(), ShouldEqual, 0)
			So(pendingRunDurationSent(lProject, configGroupName, run.FullRun).Count(), ShouldEqual, 0)
			So(maxPendingRunAgeSent(lProject, configGroupName, run.DryRun), ShouldEqual, 0)
			So(maxPendingRunAgeSent(lProject, configGroupName, run.FullRun), ShouldEqual, 0)
			So(activeRunCountSent(lProject, configGroupName, run.DryRun), ShouldEqual, 0)
			So(activeRunCountSent(lProject, configGroupName, run.FullRun), ShouldEqual, 0)
			So(activeRunDurationSent(lProject, configGroupName, run.DryRun).Count(), ShouldEqual, 0)
			So(activeRunDurationSent(lProject, configGroupName, run.FullRun).Count(), ShouldEqual, 0)

			So(ct.TSMonStore.GetAll(ctx), ShouldHaveLength, 10)
		})

		Convey("Report pending Runs", func() {
			now := ct.Clock.Now()
			// Dry Run
			putRun(lProject, configGroupName, run.DryRun, run.Status_PENDING, now.Add(-time.Second), time.Time{})
			putRun(lProject, configGroupName, run.DryRun, run.Status_PENDING, now.Add(-time.Minute), time.Time{})
			putRun(lProject, configGroupName, run.DryRun, run.Status_RUNNING, now.Add(-time.Hour), now.Add(-time.Hour)) // active run won't be reported

			// New Patchset Run
			putRun(lProject, configGroupName, run.NewPatchsetRun, run.Status_PENDING, now.Add(-2*time.Second), time.Time{})

			// Full Run
			putRun(lProject, configGroupName, run.FullRun, run.Status_PENDING, now.Add(-time.Minute), time.Time{})
			putRun(lProject, configGroupName, run.FullRun, run.Status_PENDING, now.Add(-time.Hour), time.Time{})
			putRun(lProject, configGroupName, run.FullRun, run.Status_SUCCEEDED, now.Add(-time.Hour), now.Add(-time.Hour)) // ended run won't be reported

			mustReport(lProject)
			So(pendingRunCountSent(lProject, configGroupName, run.DryRun), ShouldEqual, 2)
			So(pendingRunDurationSent(lProject, configGroupName, run.DryRun).Count(), ShouldEqual, 2)
			So(pendingRunDurationSent(lProject, configGroupName, run.DryRun).Sum(), ShouldEqual, float64((time.Second + time.Minute).Milliseconds()))
			So(maxPendingRunAgeSent(lProject, configGroupName, run.DryRun), ShouldEqual, time.Minute.Milliseconds())

			So(pendingRunCountSent(lProject, configGroupName, run.NewPatchsetRun), ShouldEqual, 1)
			So(pendingRunDurationSent(lProject, configGroupName, run.NewPatchsetRun).Count(), ShouldEqual, 1)
			So(pendingRunDurationSent(lProject, configGroupName, run.NewPatchsetRun).Sum(), ShouldEqual, float64((2 * time.Second).Milliseconds()))
			So(maxPendingRunAgeSent(lProject, configGroupName, run.NewPatchsetRun), ShouldEqual, (2 * time.Second).Milliseconds())

			So(pendingRunCountSent(lProject, configGroupName, run.FullRun), ShouldEqual, 2)
			So(pendingRunDurationSent(lProject, configGroupName, run.FullRun).Count(), ShouldEqual, 2)
			So(pendingRunDurationSent(lProject, configGroupName, run.FullRun).Sum(), ShouldEqual, float64((time.Hour + time.Minute).Milliseconds()))
			So(maxPendingRunAgeSent(lProject, configGroupName, run.FullRun), ShouldEqual, time.Hour.Milliseconds())
		})

		Convey("Report active Runs", func() {
			now := ct.Clock.Now()
			// Dry Run
			putRun(lProject, configGroupName, run.DryRun, run.Status_PENDING, now.Add(-time.Second), time.Time{}) // pending runs won't be reported
			putRun(lProject, configGroupName, run.DryRun, run.Status_RUNNING, now.Add(-time.Minute), now.Add(-time.Minute))
			putRun(lProject, configGroupName, run.DryRun, run.Status_SUCCEEDED, now.Add(-time.Hour), now.Add(-time.Hour)) // ended run won't be reported

			// New Patchset Run
			putRun(lProject, configGroupName, run.NewPatchsetRun, run.Status_RUNNING, now.Add(-2*time.Second), now.Add(-time.Second))

			// Full Run
			putRun(lProject, configGroupName, run.FullRun, run.Status_WAITING_FOR_SUBMISSION, now.Add(-time.Minute), now.Add(-time.Minute))
			putRun(lProject, configGroupName, run.FullRun, run.Status_SUBMITTING, now.Add(-time.Hour), now.Add(-time.Hour))
			putRun(lProject, configGroupName, run.FullRun, run.Status_CANCELLED, now.Add(-time.Hour), now.Add(-time.Hour)) // ended run won't be reported

			mustReport(lProject)
			So(activeRunCountSent(lProject, configGroupName, run.DryRun), ShouldEqual, 1)
			So(activeRunDurationSent(lProject, configGroupName, run.DryRun).Count(), ShouldEqual, 1)
			So(activeRunDurationSent(lProject, configGroupName, run.DryRun).Sum(), ShouldEqual, (time.Minute).Seconds())

			So(activeRunCountSent(lProject, configGroupName, run.NewPatchsetRun), ShouldEqual, 1)
			So(activeRunDurationSent(lProject, configGroupName, run.NewPatchsetRun).Count(), ShouldEqual, 1)
			So(activeRunDurationSent(lProject, configGroupName, run.NewPatchsetRun).Sum(), ShouldEqual, (time.Second).Seconds())

			So(activeRunCountSent(lProject, configGroupName, run.FullRun), ShouldEqual, 2)
			So(activeRunDurationSent(lProject, configGroupName, run.FullRun).Count(), ShouldEqual, 2)
			So(activeRunDurationSent(lProject, configGroupName, run.FullRun).Sum(), ShouldEqual, (time.Hour + time.Minute).Seconds())
		})

		putManyRuns := func(n, numProject, numConfigGroup int) {
			for projectID := 0; projectID < numProject; projectID++ {
				cfg := &cfgpb.Config{
					ConfigGroups: make([]*cfgpb.ConfigGroup, numConfigGroup),
				}
				for cgID := 0; cgID < numConfigGroup; cgID++ {
					cfg.ConfigGroups[cgID] = &cfgpb.ConfigGroup{
						Name: fmt.Sprintf("cg-%03d", cgID),
					}
				}
				prjcfgtest.Create(ctx, fmt.Sprintf("p-%03d", projectID), cfg)
			}

			for i := 0; i < n; i++ {
				project := fmt.Sprintf("p-%03d", i%numProject)
				configGroup := fmt.Sprintf("cg-%03d", (i/numProject)%numConfigGroup)
				created := ct.Clock.Now().Add(-time.Duration(i) * time.Second)
				started := ct.Clock.Now().Add(-time.Duration(i) * time.Second)
				putRun(project, configGroup, run.DryRun, run.Status_RUNNING, created, started)
			}
		}

		Convey("Can handle a lot of Runs and projects", func() {
			numProject := 16
			numConfigGroup := 8
			So(numProject*numConfigGroup, ShouldBeLessThan, maxRuns)
			putManyRuns(maxRuns, numProject, numConfigGroup)
			mustReport()

			reportedCnt := 0
			for projectID := 0; projectID < numProject; projectID++ {
				for cgID := 0; cgID < numConfigGroup; cgID++ {
					project := fmt.Sprintf("p-%03d", projectID)
					configGroup := fmt.Sprintf("cg-%03d", cgID)
					So(activeRunCountSent(project, configGroup, run.DryRun), ShouldBeGreaterThan, 0)
					reportedCnt += int(activeRunCountSent(project, configGroup, run.DryRun).(int64))
					So(activeRunDurationSent(project, configGroup, run.DryRun).Sum(), ShouldBeGreaterThan, 0)
					So(activeRunDurationSent(project, configGroup, run.DryRun).Count(), ShouldBeGreaterThan, 0)
				}
			}
			So(reportedCnt, ShouldEqual, maxRuns)
		})

		Convey("Refuses to report anything if there are too many active Runs", func() {
			putManyRuns(maxRuns+1, 16, 8)
			ra := runsAggregator{}
			So(ra.report(ctx, []string{lProject}), ShouldErrLike, "too many active Runs")
			So(ct.TSMonStore.GetAll(ctx), ShouldBeEmpty)
		})
	})
}
