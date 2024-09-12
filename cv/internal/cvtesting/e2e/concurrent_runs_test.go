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

package e2e

import (
	"fmt"
	"sort"
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	bbpb "go.chromium.org/luci/buildbucket/proto"
	bbutil "go.chromium.org/luci/buildbucket/protoutil"
	"go.chromium.org/luci/common/clock"
	gerritpb "go.chromium.org/luci/common/proto/gerrit"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/service/datastore"

	cfgpb "go.chromium.org/luci/cv/api/config/v2"
	"go.chromium.org/luci/cv/internal/configs/prjcfg/prjcfgtest"
	gf "go.chromium.org/luci/cv/internal/gerrit/gerritfake"
	"go.chromium.org/luci/cv/internal/run"
)

// TODO(tandrii): this is a slow test (~0.6s on my laptop),
// but it will become faster once LoadGerritRuns is optimized.
func TestConcurrentRunsSingular(t *testing.T) {
	t.Parallel()

	ftt.Run("CV juggles a bunch of concurrent Runs", t, func(t *ftt.Test) {
		ct := Test{}
		ctx := ct.SetUp(t)

		const lProject = "infra"
		const gHost = "g-review"
		const gRepo = "re/po"
		const gRef = "refs/heads/main"
		const gChangeFirst = 1001
		const N = 50

		cfg := MakeCfgSingular("cg0", gHost, gRepo, gRef, &cfgpb.Verifiers_Tryjob_Builder{
			Host: buildbucketHost,
			Name: fmt.Sprintf("%s/try/test-builder", lProject),
		})
		ct.BuildbucketFake.EnsureBuilders(cfg)
		prjcfgtest.Create(ctx, lProject, cfg)
		assert.Loosely(t, ct.PMNotifier.UpdateConfig(ctx, lProject), should.BeNil)

		// Prepare a bunch of actions to play out over time.
		actions := make([]struct {
			gChange     int
			user        string
			mode        run.Mode
			triggerTime time.Time // ever-increasing
			finishTime  time.Time // pseudo-random
			finalStatus run.Status
		}, N)
		var expectSubmitted, expectFinished, expectFailed []int
		for i := range actions {
			a := &actions[i]
			a.gChange = gChangeFirst + i
			a.user = fmt.Sprintf("user-%d", (i%10)+1)
			ct.GFake.AddLinkedAccountMapping([]*gerritpb.EmailInfo{
				&gerritpb.EmailInfo{Email: fmt.Sprintf("%s@example.com", a.user)},
			})
			ct.GFake.AddFrom(gf.WithCIs(gHost, gf.ACLRestricted(lProject), gf.CI(
				a.gChange, gf.Project(gRepo), gf.Ref(gRef), gf.PS(1), gf.Owner(a.user),
				gf.Updated(ct.Clock.Now()),
			)))
			// DryRunner(s) can trigger a DryRun w/o an approval and
			// a FullRun w/ approval.
			a.mode = run.DryRun
			ct.AddDryRunner(a.user)
			if i%3 == 0 {
				a.mode = run.FullRun
			}
			a.triggerTime = ct.Clock.Now().Add(time.Duration(i*3) * time.Second)
			a.finishTime = a.triggerTime.Add(time.Duration(i*13%5) * time.Minute)
			if i%2 == 0 {
				a.finalStatus = run.Status_SUCCEEDED
				if a.mode == run.FullRun {
					expectSubmitted = append(expectSubmitted, a.gChange)
				} else {
					expectFinished = append(expectFinished, a.gChange)
				}
			} else {
				a.finalStatus = run.Status_FAILED
				expectFailed = append(expectFailed, a.gChange)
			}
		}
		indexesByFinishTime := make([]int, len(actions))
		for i := range actions {
			indexesByFinishTime[i] = i
		}
		sort.Slice(indexesByFinishTime, func(i, j int) bool {
			idxI, idxJ := indexesByFinishTime[i], indexesByFinishTime[j]
			return actions[idxI].finishTime.Before(actions[idxJ].finishTime)
		})

		// Making sure the Tryjob ends at ~the finish time.
		bbClient := ct.BuildbucketFake.MustNewClient(ctx, buildbucketHost, lProject)
		go func() {
			timer := clock.NewTimer(ctx)
			for {
				timer.Reset(1 * time.Minute)
				select {
				case <-ctx.Done():
					return
				case <-timer.GetC():
					req := &bbpb.SearchBuildsRequest{
						Predicate: &bbpb.BuildPredicate{},
					}
					for {
						res, err := bbClient.SearchBuilds(ctx, req)
						if err != nil {
							panic(err)
						}
						for _, b := range res.GetBuilds() {
							if bbutil.IsEnded(b.Status) {
								continue
							}
							gChange := b.GetInput().GetGerritChanges()[0].GetChange()
							action := actions[gChange-gChangeFirst]
							if !clock.Now(ctx).Before(action.finishTime) {
								ct.BuildbucketFake.MutateBuild(ctx, buildbucketHost, b.Id, func(b *bbpb.Build) {
									if action.finalStatus == run.Status_SUCCEEDED {
										b.Status = bbpb.Status_SUCCESS
									} else {
										b.Status = bbpb.Status_FAILURE
									}
									b.StartTime = timestamppb.New(ct.Clock.Now())
									b.EndTime = timestamppb.New(ct.Clock.Now())
								})
							}
						}
						req.PageToken = res.GetNextPageToken()
						if req.GetPageToken() == "" {
							break
						}
					}
				}
			}
		}()

		ct.LogPhase(ctx, fmt.Sprintf("Triggering CQ on %d CLs", len(actions)))
		for i := range actions {
			a := &actions[i]
			if !ct.Clock.Now().After(a.triggerTime) {
				ct.RunUntil(ctx, func() bool { return ct.Clock.Now().After(a.triggerTime) })
			}
			ct.GFake.MutateChange(gHost, a.gChange, func(c *gf.Change) {
				val := 1
				if a.mode == run.FullRun {
					val = 2
					// FullRun requires an approval; self-stamp it
					gf.Approve()(c.Info)
				}
				gf.CQ(val, a.triggerTime, gf.U(a.user))(c.Info)
				gf.Updated(a.triggerTime)(c.Info)
			})
		}

		// Now iterate in increasing finishAt, checking state of Gerrit CL.
		var actualSubmitted, actualFinished, actualFailed, actualWeird []int
		for _, i := range indexesByFinishTime {
			a := actions[i]
			ct.LogPhase(ctx, fmt.Sprintf("Checking state of #%d %s expected state %s", a.gChange, a.mode, a.finalStatus))
			var runs []*run.Run
			ct.RunUntil(ctx, func() bool {
				if !ct.Clock.Now().After(a.finishTime) {
					return false
				}
				runs = ct.LoadGerritRuns(ctx, gHost, int64(a.gChange))
				return len(runs) > 0
			})
			assert.Loosely(t, runs, should.HaveLength(1))
			r := runs[0]
			ct.RunUntil(ctx, func() bool {
				r = ct.LoadRun(ctx, runs[0].ID)
				return run.IsEnded(r.Status)
			})
			switch {
			case r.Status == run.Status_FAILED:
				actualFailed = append(actualFailed, a.gChange)
				assert.Loosely(t, ct.MaxCQVote(ctx, gHost, int64(a.gChange)), should.BeZero)
			case r.Status == run.Status_SUCCEEDED && a.mode == run.DryRun:
				actualFinished = append(actualFinished, a.gChange)
				assert.Loosely(t, ct.MaxCQVote(ctx, gHost, int64(a.gChange)), should.BeZero)
			case r.Status == run.Status_SUCCEEDED && a.mode == run.FullRun:
				actualSubmitted = append(actualSubmitted, a.gChange)
				assert.Loosely(t, ct.GFake.GetChange(gHost, a.gChange).Info.GetStatus(), should.Equal(gerritpb.ChangeStatus_MERGED))
			default:
				actualWeird = append(actualWeird, a.gChange)
			}
			assert.Loosely(t, r.CreateTime, should.Match(datastore.RoundTime(a.triggerTime.UTC())))
			assert.Loosely(t, r.EndTime, should.HappenAfter(a.finishTime))
		}

		sort.Ints(actualSubmitted)
		sort.Ints(actualFailed)
		sort.Ints(actualFinished)
		assert.Loosely(t, actualSubmitted, should.Resemble(expectSubmitted))
		assert.Loosely(t, actualFailed, should.Resemble(expectFailed))
		assert.Loosely(t, actualFinished, should.Resemble(expectFinished))
		assert.Loosely(t, actualWeird, should.BeEmpty)

		assert.Loosely(t, ct.LoadRunsOf(ctx, lProject), should.HaveLength(len(actions)))

		ct.LogPhase(ctx, "Wait for all BQ exports to complete")
		ct.RunUntil(ctx, func() bool { return ct.ExportedBQAttemptsCount() == len(actions) })

		ct.LogPhase(ctx, "Wait for all PubSub messages are sent")
		ct.RunUntil(ctx, func() bool { return len(ct.RunEndedPubSubTasks()) == len(actions) })
	})
}
