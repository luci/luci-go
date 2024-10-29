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

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/common/tsmon/distribution"
	"go.chromium.org/luci/gae/service/datastore"

	cfgpb "go.chromium.org/luci/cv/api/config/v2"
	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/configs/prjcfg"
	"go.chromium.org/luci/cv/internal/configs/prjcfg/prjcfgtest"
	"go.chromium.org/luci/cv/internal/cvtesting"
	"go.chromium.org/luci/cv/internal/metrics"
	"go.chromium.org/luci/cv/internal/run"
)

func TestRunAggregator(t *testing.T) {
	t.Parallel()

	ftt.Run("runAggregator works", t, func(t *ftt.Test) {
		ct := cvtesting.Test{}
		ctx := ct.SetUp(t)
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
			assert.NoErr(t, err)
		}

		t.Run("Skip reporting for disabled project", func(t *ftt.Test) {
			prjcfgtest.Disable(ctx, lProject)
			mustReport(lProject)
			assert.Loosely(t, ct.TSMonStore.GetAll(ctx), should.BeEmpty)
		})

		t.Run("Enabled projects get zero data reported when no interested Runs", func(t *ftt.Test) {
			mustReport(lProject)
			assert.Loosely(t, pendingRunCountSent(lProject, configGroupName, run.DryRun), should.BeZero)
			assert.Loosely(t, pendingRunCountSent(lProject, configGroupName, run.FullRun), should.BeZero)
			assert.Loosely(t, pendingRunDurationSent(lProject, configGroupName, run.DryRun).Count(), should.BeZero)
			assert.Loosely(t, pendingRunDurationSent(lProject, configGroupName, run.FullRun).Count(), should.BeZero)
			assert.Loosely(t, maxPendingRunAgeSent(lProject, configGroupName, run.DryRun), should.BeZero)
			assert.Loosely(t, maxPendingRunAgeSent(lProject, configGroupName, run.FullRun), should.BeZero)
			assert.Loosely(t, activeRunCountSent(lProject, configGroupName, run.DryRun), should.BeZero)
			assert.Loosely(t, activeRunCountSent(lProject, configGroupName, run.FullRun), should.BeZero)
			assert.Loosely(t, activeRunDurationSent(lProject, configGroupName, run.DryRun).Count(), should.BeZero)
			assert.Loosely(t, activeRunDurationSent(lProject, configGroupName, run.FullRun).Count(), should.BeZero)

			assert.Loosely(t, ct.TSMonStore.GetAll(ctx), should.HaveLength(10))
		})

		t.Run("Report pending Runs", func(t *ftt.Test) {
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
			assert.Loosely(t, pendingRunCountSent(lProject, configGroupName, run.DryRun), should.Equal(2))
			assert.Loosely(t, pendingRunDurationSent(lProject, configGroupName, run.DryRun).Count(), should.Equal(2))
			assert.Loosely(t, pendingRunDurationSent(lProject, configGroupName, run.DryRun).Sum(), should.Equal(float64((time.Second + time.Minute).Milliseconds())))
			assert.Loosely(t, maxPendingRunAgeSent(lProject, configGroupName, run.DryRun), should.Equal(time.Minute.Milliseconds()))

			assert.Loosely(t, pendingRunCountSent(lProject, configGroupName, run.NewPatchsetRun), should.Equal(1))
			assert.Loosely(t, pendingRunDurationSent(lProject, configGroupName, run.NewPatchsetRun).Count(), should.Equal(1))
			assert.Loosely(t, pendingRunDurationSent(lProject, configGroupName, run.NewPatchsetRun).Sum(), should.Equal(float64((2 * time.Second).Milliseconds())))
			assert.Loosely(t, maxPendingRunAgeSent(lProject, configGroupName, run.NewPatchsetRun), should.Equal((2 * time.Second).Milliseconds()))

			assert.Loosely(t, pendingRunCountSent(lProject, configGroupName, run.FullRun), should.Equal(2))
			assert.Loosely(t, pendingRunDurationSent(lProject, configGroupName, run.FullRun).Count(), should.Equal(2))
			assert.Loosely(t, pendingRunDurationSent(lProject, configGroupName, run.FullRun).Sum(), should.Equal(float64((time.Hour + time.Minute).Milliseconds())))
			assert.Loosely(t, maxPendingRunAgeSent(lProject, configGroupName, run.FullRun), should.Equal(time.Hour.Milliseconds()))
		})

		t.Run("Report active Runs", func(t *ftt.Test) {
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
			assert.Loosely(t, activeRunCountSent(lProject, configGroupName, run.DryRun), should.Equal(1))
			assert.Loosely(t, activeRunDurationSent(lProject, configGroupName, run.DryRun).Count(), should.Equal(1))
			assert.Loosely(t, activeRunDurationSent(lProject, configGroupName, run.DryRun).Sum(), should.Equal((time.Minute).Seconds()))

			assert.Loosely(t, activeRunCountSent(lProject, configGroupName, run.NewPatchsetRun), should.Equal(1))
			assert.Loosely(t, activeRunDurationSent(lProject, configGroupName, run.NewPatchsetRun).Count(), should.Equal(1))
			assert.Loosely(t, activeRunDurationSent(lProject, configGroupName, run.NewPatchsetRun).Sum(), should.Equal((time.Second).Seconds()))

			assert.Loosely(t, activeRunCountSent(lProject, configGroupName, run.FullRun), should.Equal(2))
			assert.Loosely(t, activeRunDurationSent(lProject, configGroupName, run.FullRun).Count(), should.Equal(2))
			assert.Loosely(t, activeRunDurationSent(lProject, configGroupName, run.FullRun).Sum(), should.Equal((time.Hour + time.Minute).Seconds()))
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

		t.Run("Can handle a lot of Runs and projects", func(t *ftt.Test) {
			numProject := 16
			numConfigGroup := 8
			assert.Loosely(t, numProject*numConfigGroup, should.BeLessThan(maxRuns))
			putManyRuns(maxRuns, numProject, numConfigGroup)
			mustReport()

			reportedCnt := 0
			for projectID := 0; projectID < numProject; projectID++ {
				for cgID := 0; cgID < numConfigGroup; cgID++ {
					project := fmt.Sprintf("p-%03d", projectID)
					configGroup := fmt.Sprintf("cg-%03d", cgID)
					assert.Loosely(t, activeRunCountSent(project, configGroup, run.DryRun), should.BeGreaterThan(0))
					reportedCnt += int(activeRunCountSent(project, configGroup, run.DryRun).(int64))
					assert.That(t, activeRunDurationSent(project, configGroup, run.DryRun).Sum(), should.BeGreaterThan(0.0))
					assert.That(t, activeRunDurationSent(project, configGroup, run.DryRun).Count(), should.BeGreaterThan[int64](0))
				}
			}
			assert.Loosely(t, reportedCnt, should.Equal(maxRuns))
		})

		t.Run("Refuses to report anything if there are too many active Runs", func(t *ftt.Test) {
			putManyRuns(maxRuns+1, 16, 8)
			ra := runsAggregator{}
			assert.Loosely(t, ra.report(ctx, []string{lProject}), should.ErrLike("too many active Runs"))
			assert.Loosely(t, ct.TSMonStore.GetAll(ctx), should.BeEmpty)
		})
	})
}
