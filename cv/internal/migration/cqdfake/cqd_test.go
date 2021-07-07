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

package cqdfake

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"go.chromium.org/luci/common/clock"
	gerritpb "go.chromium.org/luci/common/proto/gerrit"
	"go.chromium.org/luci/gae/service/datastore"
	"google.golang.org/protobuf/proto"

	cvbqpb "go.chromium.org/luci/cv/api/bigquery/v1"
	migrationpb "go.chromium.org/luci/cv/api/migration"
	"go.chromium.org/luci/cv/internal/cvtesting"
	gf "go.chromium.org/luci/cv/internal/gerrit/gerritfake"
	"go.chromium.org/luci/cv/internal/gerrit/trigger"
	"go.chromium.org/luci/cv/internal/migration"

	. "github.com/smartystreets/goconvey/convey"
)

func TestCQDFakeInactiveCV(t *testing.T) {
	t.Parallel()

	Convey("CQDFake processes candidates in CQDaemon active mode", t, func() {
		ct := cvtesting.Test{}
		ctx, cancel := ct.SetUp()
		defer cancel()

		const lProject = "test"
		const gHost = "gerrit.example.com"
		ct.GFake.AddFrom(gf.WithCIs(
			gHost, gf.ACLPublic(),
			gf.CI(1, gf.CQ(+1, ct.Clock.Now(), "user-1")),
			gf.CI(2, gf.CQ(+2, ct.Clock.Now(), "user-2")),
			gf.CI(3, gf.CQ(+1, ct.Clock.Now(), "user-3"), gf.Vote("Quick-Dry-Run", +1, ct.Clock.Now(), "user-3")),
		))
		cqd := CQDFake{
			LUCIProject: lProject,
			CV: &migration.MigrationServer{
				GFactory: ct.GFake.Factory(),
			},
			GFake: ct.GFake,
		}
		ct.DisableCVRunManagement(ctx)

		done4iterations := make(chan struct{})
		var progress []string

		candidatesCalls := 0
		cqd.SetCandidatesClbk(func() []*migrationpb.ReportedRun {
			var res []*migrationpb.ReportedRun
			candidatesCalls++
			switch candidatesCalls {
			case 1:
				res = []*migrationpb.ReportedRun{
					{
						Attempt: &cvbqpb.Attempt{
							Key:           "to-abort",
							GerritChanges: []*cvbqpb.GerritChange{{Host: gHost, Change: 1, Mode: cvbqpb.Mode_DRY_RUN}},
							Status:        cvbqpb.AttemptStatus_STARTED,
						},
					},
				}
			case 2:
				res = []*migrationpb.ReportedRun{
					{
						Attempt: &cvbqpb.Attempt{
							Key:           "to-submit",
							GerritChanges: []*cvbqpb.GerritChange{{Host: gHost, Change: 2, Mode: cvbqpb.Mode_FULL_RUN}},
							Status:        cvbqpb.AttemptStatus_STARTED,
						},
					},
				}
			case 3:
				res = []*migrationpb.ReportedRun{
					{
						Attempt: &cvbqpb.Attempt{
							Key:           "to-fail",
							GerritChanges: []*cvbqpb.GerritChange{{Host: gHost, Change: 3, Mode: cvbqpb.Mode_QUICK_DRY_RUN}},
							Status:        cvbqpb.AttemptStatus_STARTED,
						},
					},
				}
			case 4:
				close(done4iterations)
			default:
				// Don't record results of the rest of iterations, which may occur since
				// there is a race between closing done4iterations channel and
				// cqd.Close() taking effect.
				return nil
			}
			prior := strings.Join(cqd.ActiveAttemptKeys(), " ")
			progress = append(progress, fmt.Sprintf("candidates #%d (prior: [%s])", candidatesCalls, prior))
			return res
		})

		cqd.SetVerifyClbk(func(r *migrationpb.ReportedRun, _ bool) *migrationpb.ReportedRun {
			k := r.GetAttempt().GetKey()
			progress = append(progress, "verify "+k)
			r = proto.Clone(r).(*migrationpb.ReportedRun)
			switch k {
			case "to-submit":
				r.GetAttempt().Status = cvbqpb.AttemptStatus_SUCCESS
			case "to-fail":
				r.GetAttempt().Status = cvbqpb.AttemptStatus_FAILURE
			}
			return r
		})

		// Override cvtesting timer callback to immediately unblock waiting CQDFake.
		ct.Clock.SetTimerCallback(func(dur time.Duration, timer clock.Timer) {
			ct.Clock.Add(dur)
		})

		// Start & wait for 4 iterations.
		cqd.Start(ctx)
		select {
		case <-done4iterations:
		case <-ctx.Done(): // avoid hanging forever.
			panic(ctx.Err())
		}
		cqd.Close()

		So(progress, ShouldResemble, []string{
			"candidates #1 (prior: [])",
			"verify to-abort", // noop, "to-abort" is left for the next loop

			"candidates #2 (prior: [to-abort])",
			"verify to-submit", // "to-abort" is gone before verification

			// to-submit was gone from `attempts` before the start of this loop
			"candidates #3 (prior: [])",
			"verify to-fail",

			// to-fail was gone from `attempts` before the start of this loop
			"candidates #4 (prior: [])",
		})

		// Change #1 was aborted, so it should not be touched.
		So(gf.NonZeroVotes(ct.GFake.GetChange(gHost, 1).Info, trigger.CQLabelName), ShouldNotBeEmpty)
		// Change #2 was submitted.
		So(ct.GFake.GetChange(gHost, 2).Info.Status, ShouldResemble, gerritpb.ChangeStatus_MERGED)
		// Change #3 failed, so its votes must be removed.
		So(gf.NonZeroVotes(ct.GFake.GetChange(gHost, 3).Info, trigger.CQLabelName), ShouldBeEmpty)
		So(gf.NonZeroVotes(ct.GFake.GetChange(gHost, 3).Info, "Quick-Dry-Run"), ShouldBeEmpty)

		// FinishedCQDRun entity must exist for each completed attempt.
		for _, k := range []string{"to-abort", "to-submit", "to-fail"} {
			So(datastore.Get(ctx, &migration.FinishedCQDRun{AttemptKey: k}), ShouldBeNil)
		}
	})
}
