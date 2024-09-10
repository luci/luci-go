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

	"google.golang.org/protobuf/types/known/durationpb"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	cfgpb "go.chromium.org/luci/cv/api/config/v2"
	"go.chromium.org/luci/cv/internal/configs/prjcfg/prjcfgtest"
	gf "go.chromium.org/luci/cv/internal/gerrit/gerritfake"
	"go.chromium.org/luci/cv/internal/run"
	"go.chromium.org/luci/cv/internal/run/runtest"
	"go.chromium.org/luci/gae/service/datastore"
)

func TestSubmissionObeySubmitOptions(t *testing.T) {
	t.Parallel()

	ftt.Run("Burst requests to submit", t, func(t *ftt.Test) {
		ct := Test{}
		ctx := ct.SetUp(t)

		const lProject = "infra"
		const gHost = "g-review"
		const gRepo = "re/po"
		const gRef = "refs/heads/main"
		const burstN = 20

		cfg := MakeCfgSingular("cg0", gHost, gRepo, gRef)
		maxBurst := 2
		burstDelay := 10 * time.Second
		cfg.SubmitOptions = &cfgpb.SubmitOptions{
			MaxBurst:   int32(maxBurst),
			BurstDelay: durationpb.New(burstDelay),
		}
		prjcfgtest.Create(ctx, lProject, cfg)
		assert.Loosely(t, ct.PMNotifier.UpdateConfig(ctx, lProject), should.BeNil)

		// burstN CLs request FULL_RUN at the same time.
		for gChange := 1; gChange <= burstN; gChange++ {
			ct.GFake.AddFrom(gf.WithCIs(gHost, gf.ACLRestricted(lProject), gf.CI(
				gChange, gf.Project(gRepo), gf.Ref(gRef),
				gf.Owner("user-1"),
				gf.CQ(+2, ct.Clock.Now(), gf.U("user-2")),
				gf.Approve(),
				gf.Updated(ct.Clock.Now()),
			)))
		}
		// Only a committer can trigger a FullRun for someone else' CL.
		ct.AddCommitter("user-2")
		ct.LogPhase(ctx, fmt.Sprintf("CV starts and creates %d Runs", burstN))
		ct.RunUntilT(ctx, burstN*5 /* ~5 tasks per Run */, func() bool {
			return len(ct.LoadRunsOf(ctx, lProject)) == burstN
		})

		ct.LogPhase(ctx, fmt.Sprintf("CV successfully submits %d Runs", burstN))
		remaining := ct.LoadRunsOf(ctx, lProject)
		ct.RunUntilT(ctx, burstN*25 /* ~25 tasks per Run */, func() bool {
			switch err := datastore.Get(ctx, remaining); {
			case err != nil:
				panic(err)
			default:
				remaining = runtest.FilterNot(run.Status_SUCCEEDED, remaining...)
				return len(remaining) == 0
			}
		})

		var submittedTimes []time.Time
		for gChange := 1; gChange <= burstN; gChange++ {
			c := ct.GFake.GetChange(gHost, gChange)
			// last updated time of a CL should be submitted time.
			submittedTimes = append(submittedTimes, c.Info.GetUpdated().AsTime())
		}

		sort.Slice(submittedTimes, func(i, j int) bool {
			return submittedTimes[i].Before(submittedTimes[j])
		})
		for i := 0; i < burstN-maxBurst; i++ {
			assert.Loosely(t, submittedTimes[i+maxBurst], should.HappenOnOrAfter(submittedTimes[i].Add(burstDelay)))
		}
	})
}
