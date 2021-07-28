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

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/durationpb"

	cvbqpb "go.chromium.org/luci/cv/api/bigquery/v1"
	cfgpb "go.chromium.org/luci/cv/api/config/v2"
	migrationpb "go.chromium.org/luci/cv/api/migration"
	"go.chromium.org/luci/cv/internal/configs/prjcfg/prjcfgtest"
	gf "go.chromium.org/luci/cv/internal/gerrit/gerritfake"
	"go.chromium.org/luci/cv/internal/run"

	. "github.com/smartystreets/goconvey/convey"
)

func TestSubmissionObeySubmitOptions(t *testing.T) {
	t.Parallel()

	Convey("Burst requests to sumbit", t, func() {
		ct := Test{}
		ctx, cancel := ct.SetUp()
		defer cancel()

		const lProject = "infra"
		const gHost = "g-review"
		const gRepo = "re/po"
		const gRef = "refs/heads/main"
		const burstN = 20

		cfg := MakeCfgSingular("cg0", gHost, gRepo, gRef)
		maxBurst := 2
		burstDelay := 1 * time.Minute
		cfg.SubmitOptions = &cfgpb.SubmitOptions{
			MaxBurst:   int32(maxBurst),
			BurstDelay: durationpb.New(burstDelay),
		}
		prjcfgtest.Create(ctx, lProject, cfg)
		So(ct.PMNotifier.UpdateConfig(ctx, lProject), ShouldBeNil)

		// burstN CLs request FULL_RUN at the same time.
		for gChange := 1; gChange <= burstN; gChange++ {
			ct.GFake.AddFrom(gf.WithCIs(gHost, gf.ACLRestricted(lProject), gf.CI(
				gChange, gf.Project(gRepo), gf.Ref(gRef),
				gf.Owner("user-1"),
				gf.CQ(+2, ct.Clock.Now(), gf.U("user-2")), gf.Updated(ct.Clock.Now()),
			)))
		}

		// Start CQDaemon and make it succeed the Run immediately.
		ct.MustCQD(ctx, lProject).SetVerifyClbk(
			func(r *migrationpb.ReportedRun) *migrationpb.ReportedRun {
				r = proto.Clone(r).(*migrationpb.ReportedRun)
				r.Attempt.Status = cvbqpb.AttemptStatus_SUCCESS
				r.Attempt.Substatus = cvbqpb.AttemptSubstatus_NO_SUBSTATUS
				return r
			},
		)

		ct.LogPhase(ctx, fmt.Sprintf("CV starts and successfully submitted %d Runs", burstN))
		ct.RunUntil(ctx, func() bool {
			runs := ct.LoadRunsOf(ctx, lProject)
			if len(runs) != burstN {
				return false
			}
			for _, r := range runs {
				if r.Status != run.Status_SUCCEEDED {
					return false
				}
			}
			return true
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
			So(submittedTimes[i+maxBurst], ShouldHappenOnOrAfter, submittedTimes[i].Add(burstDelay))
		}
	})
}
