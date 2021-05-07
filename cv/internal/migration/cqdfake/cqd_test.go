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
	bqpb "go.chromium.org/luci/cv/api/bigquery/v1"
	cvbqpb "go.chromium.org/luci/cv/api/bigquery/v1"
	migrationpb "go.chromium.org/luci/cv/api/migration"
	"go.chromium.org/luci/cv/internal/cvtesting"
	"go.chromium.org/luci/cv/internal/migration"

	. "github.com/smartystreets/goconvey/convey"
)

func TestCQDFake(t *testing.T) {
	t.Parallel()

	Convey("CQDFake processes candidates in CQDaemon active mode", t, func() {
		ct := cvtesting.Test{}
		ctx, cancel := ct.SetUp()
		defer cancel()

		const lProject = "test"
		const gHost = "gerrit.example.com"
		cqd := CQDFake{
			LUCIProject: lProject,
			CV:          &migration.MigrationServer{},
			GFake:       ct.GFake,
		}
		ct.DisableCVRunManagement(ctx)

		done4iterations := make(chan struct{})
		var progress []string

		candidatesCalls := 0
		cqd.SetCandidatesClbk(func() []*migrationpb.Run {
			var res []*migrationpb.Run
			candidatesCalls++
			switch candidatesCalls {
			case 1:
				res = []*migrationpb.Run{
					{
						Attempt: &bqpb.Attempt{
							Key:           "beef01",
							GerritChanges: []*cvbqpb.GerritChange{{Host: gHost, Change: 1}},
						},
					},
				}
			case 2:
				res = []*migrationpb.Run{
					{
						Attempt: &bqpb.Attempt{
							Key:           "beef02",
							GerritChanges: []*cvbqpb.GerritChange{{Host: gHost, Change: 2}},
						},
					},
				}
			case 3:
			case 4:
				close(done4iterations)
			default:
				// Don't record results of >4th iterations, which may occur since there
				// is a race between closing done4iterations channel and cqd.Close()
				// taking effect.
				return nil
			}
			keys := strings.Join(cqd.ActiveAttemptKeys(), " ")
			progress = append(progress, fmt.Sprintf("candidates #%d (prior: [%s])", candidatesCalls, keys))
			return res
		})

		cqd.SetVerifyClbk(func(r *migrationpb.Run, _ bool) *migrationpb.Run {
			progress = append(progress, fmt.Sprintf("verify %s", r.GetAttempt().GetKey()))
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
			`candidates #1 (prior: [])`,
			`verify beef01`,
			`candidates #2 (prior: [beef01])`,
			`verify beef02`,
			`candidates #3 (prior: [beef02])`,
			`candidates #4 (prior: [])`,
		})
	})
}
