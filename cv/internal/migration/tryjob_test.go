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

package migration

import (
	"context"
	"sort"
	"testing"
	"time"

	buildbucketpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/gae/service/datastore"

	migrationpb "go.chromium.org/luci/cv/api/migration"
	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/cvtesting"

	. "github.com/smartystreets/goconvey/convey"
)

func TestReportedTryjobs(t *testing.T) {
	t.Parallel()

	Convey("ReportedTryjobs", t, func() {
		ct := cvtesting.Test{}
		ctx, cancel := ct.SetUp()
		defer cancel()

		const (
			runInfra = common.RunID("infra/111-beef")
			runV8    = common.RunID("v8/222-beef")
		)

		Convey("ID to/from works", func() {
			t := testclock.TestRecentTimeUTC.Add(time.Hour)
			id := makeReportedTryjobsID(runInfra, t)
			e := ReportedTryjobs{ID: id}
			So(e.RunID(), ShouldResemble, runInfra)
			So(e.ReportTime(), ShouldResemble, t)
		})

		Convey("ID are in Run ASC, ReportTime DESC order", func() {
			base := testclock.TestRecentTimeUTC
			future := cqdDeletedAfter.Add(-time.Hour)
			ids := []string{
				makeReportedTryjobsID(runInfra, cqdDeletedAfter),
				makeReportedTryjobsID(runInfra, future),
				makeReportedTryjobsID(runInfra, base.Add(time.Hour)),
				makeReportedTryjobsID(runInfra, base.Add(time.Minute)),
				makeReportedTryjobsID(runInfra, base.Add(time.Second)),
				makeReportedTryjobsID(runV8, base.Add(time.Hour)),
				makeReportedTryjobsID(runV8, base.Add(time.Minute)),
			}
			So(sort.StringsAreSorted(ids), ShouldBeTrue)
		})

		Convey("saveReportedTryjobs works", func() {
			var idStarted, idFailed string
			err := saveReportedTryjobs(ctx, &migrationpb.ReportTryjobsRequest{
				RunId:   string(runInfra),
				Tryjobs: []*migrationpb.Tryjob{{BbStatus: buildbucketpb.Status_STARTED}},
			}, func(_ context.Context, id string) error { idStarted = id; return nil })
			So(err, ShouldBeNil)

			ct.Clock.Add(time.Minute)
			err = saveReportedTryjobs(ctx, &migrationpb.ReportTryjobsRequest{
				RunId:   string(runInfra),
				Tryjobs: []*migrationpb.Tryjob{{BbStatus: buildbucketpb.Status_FAILURE}},
			}, func(_ context.Context, id string) error { idFailed = id; return nil })
			So(err, ShouldBeNil)
			So(datastore.Get(ctx, &ReportedTryjobs{ID: idStarted}), ShouldBeNil)
			So(datastore.Get(ctx, &ReportedTryjobs{ID: idFailed}), ShouldBeNil)

			Convey("LoadReportedTryjobs works", func() {
				load := func(rid common.RunID, after time.Time, limit int32) []*ReportedTryjobs {
					out, err := ListReportedTryjobs(ctx, rid, after, limit)
					So(err, ShouldBeNil)
					return out
				}

				Convey("empty", func() {
					So(load(runV8, time.Time{}, 0), ShouldBeEmpty)
				})

				Convey("correctly ordered", func() {
					out := load(runInfra, time.Time{}, 0)
					So(out, ShouldHaveLength, 2)
					// Latest record first.
					So(out[0].ID, ShouldResemble, idFailed)
					// Oldest record last.
					So(out[1].ID, ShouldResemble, idStarted)
				})
				Convey("limit enforced", func() {
					out := load(runInfra, time.Time{}, 1)
					So(out, ShouldHaveLength, 1)
					// The latest record only.
					So(out[0].ID, ShouldResemble, idFailed)
				})
				Convey("created after a non-inclusive cutoff", func() {
					out := load(runInfra, ct.Clock.Now().Add(-time.Hour), 0)
					So(out, ShouldHaveLength, 2)
					So(out[1].ID, ShouldResemble, idStarted)

					out2 := load(runInfra, out[1].ReportTime(), 0)
					So(out2, ShouldHaveLength, 1)
					So(out2[0].ID, ShouldResemble, idFailed)

					So(load(runInfra, out[0].ReportTime(), 0), ShouldBeEmpty)
				})
			})
		})
	})
}
