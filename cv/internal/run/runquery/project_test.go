// Copyright 2020 The LUCI Authors.
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

package runquery

import (
	"testing"
	"time"

	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/cvtesting"
	"go.chromium.org/luci/cv/internal/run"

	. "github.com/smartystreets/goconvey/convey"
)

func TestProjectQueryBuilder(t *testing.T) {
	t.Parallel()

	Convey("ProjectQueryBuilder works", t, func() {
		ct := cvtesting.Test{}
		ctx, cancel := ct.SetUp(t)
		defer cancel()

		getAll := func(qb ProjectQueryBuilder) common.RunIDs {
			return execQueryInTestSameRunsAndKeys(ctx, qb)
		}

		makeRun := func(proj string, delay time.Duration, s run.Status) common.RunID {
			createdAt := ct.Clock.Now().Add(delay)
			runID := common.MakeRunID(proj, createdAt, 1, []byte{0, byte(delay / time.Millisecond)})
			So(datastore.Put(ctx, &run.Run{ID: runID, Status: s}), ShouldBeNil)
			return runID
		}

		// RunID below are ordered lexicographically.
		bond9 := makeRun("bond", 9*time.Millisecond, run.Status_RUNNING)
		bond4 := makeRun("bond", 4*time.Millisecond, run.Status_FAILED)
		bond2 := makeRun("bond", 2*time.Millisecond, run.Status_CANCELLED)
		xero7 := makeRun("xero", 7*time.Millisecond, run.Status_RUNNING)
		xero6 := makeRun("xero", 6*time.Millisecond, run.Status_RUNNING)
		xero5 := makeRun("xero", 5*time.Millisecond, run.Status_SUCCEEDED)

		Convey("Project without Runs", func() {
			qb := ProjectQueryBuilder{Project: "missing"}
			So(getAll(qb), ShouldBeEmpty)
		})

		Convey("Project with some Runs", func() {
			qb := ProjectQueryBuilder{Project: "bond"}
			So(getAll(qb), ShouldResemble, common.RunIDs{bond9, bond4, bond2})
		})

		Convey("Obeys limit and returns correct page token", func() {
			qb := ProjectQueryBuilder{Project: "bond", Limit: 2}
			runs1, pageToken1, err := qb.LoadRuns(ctx)
			So(err, ShouldBeNil)
			So(idsOf(runs1), ShouldResemble, common.RunIDs{bond9, bond4})
			So(pageToken1, ShouldNotBeNil)

			qb = qb.PageToken(pageToken1)
			So(qb.MinExcl, ShouldResemble, bond4)
			runs2, pageToken2, err := qb.LoadRuns(ctx)
			So(err, ShouldBeNil)
			So(idsOf(runs2), ShouldResemble, common.RunIDs{bond2})
			So(pageToken2, ShouldBeNil)
		})

		Convey("Filters by Status", func() {
			Convey("Simple", func() {
				qb := ProjectQueryBuilder{Project: "xero", Status: run.Status_RUNNING}
				So(getAll(qb), ShouldResemble, common.RunIDs{xero7, xero6})

				qb = ProjectQueryBuilder{Project: "xero", Status: run.Status_SUCCEEDED}
				So(getAll(qb), ShouldResemble, common.RunIDs{xero5})
			})
			Convey("Status_ENDED_MASK", func() {
				qb := ProjectQueryBuilder{Project: "bond", Status: run.Status_ENDED_MASK}
				So(getAll(qb), ShouldResemble, common.RunIDs{bond4, bond2})

				Convey("Obeys limit", func() {
					qb.Limit = 1
					So(getAll(qb), ShouldResemble, common.RunIDs{bond4})
				})
			})
		})

		Convey("Min", func() {
			qb := ProjectQueryBuilder{Project: "bond", MinExcl: bond9}
			So(getAll(qb), ShouldResemble, common.RunIDs{bond4, bond2})

			Convey("same as Before", func() {
				qb2 := ProjectQueryBuilder{}.Before(bond9)
				So(qb, ShouldResemble, qb2)
			})
		})

		Convey("Max", func() {
			qb := ProjectQueryBuilder{Project: "bond", MaxExcl: bond2}
			So(getAll(qb), ShouldResemble, common.RunIDs{bond9, bond4})

			Convey("same as After", func() {
				qb2 := ProjectQueryBuilder{}.After(bond2)
				So(qb, ShouldResemble, qb2)
			})
		})

		Convey("After .. Before", func() {
			Convey("Some", func() {
				qb := ProjectQueryBuilder{}.After(bond2).Before(bond9)
				So(getAll(qb), ShouldResemble, common.RunIDs{bond4})
			})

			Convey("Empty", func() {
				qb := ProjectQueryBuilder{Project: "bond"}.After(bond4).Before(bond9)
				So(getAll(qb), ShouldHaveLength, 0)
			})

			Convey("Overconstrained", func() {
				qb := ProjectQueryBuilder{Project: "bond"}.After(bond9).Before(bond2)
				_, err := qb.BuildKeysOnly(ctx).Finalize()
				So(err, ShouldEqual, datastore.ErrNullQuery)
			})

			Convey("With status", func() {
				qb := ProjectQueryBuilder{Status: run.Status_FAILED}.After(bond2).Before(bond9)
				So(getAll(qb), ShouldResemble, common.RunIDs{bond4})
				qb = ProjectQueryBuilder{Status: run.Status_SUCCEEDED}.After(bond2).Before(bond9)
				So(getAll(qb), ShouldHaveLength, 0)
			})

		})

		Convey("Invalid usage panics", func() {
			So(func() { ProjectQueryBuilder{}.BuildKeysOnly(ctx) }, ShouldPanic)
			So(func() { ProjectQueryBuilder{Project: "not-bond", MinExcl: bond4}.BuildKeysOnly(ctx) }, ShouldPanic)
			So(func() { ProjectQueryBuilder{Project: "not-bond", MaxExcl: bond4}.BuildKeysOnly(ctx) }, ShouldPanic)
			So(func() { ProjectQueryBuilder{Project: "not-bond"}.Before(bond4) }, ShouldPanic)
			So(func() { ProjectQueryBuilder{Project: "not-bond"}.After(bond4) }, ShouldPanic)
			So(func() { ProjectQueryBuilder{}.After(bond4).Before(xero7) }, ShouldPanic)
		})
	})
}
