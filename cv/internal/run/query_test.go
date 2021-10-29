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

package run

import (
	"context"
	"encoding/hex"
	"fmt"
	"testing"
	"time"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/grpc/appstatus"
	"google.golang.org/grpc/codes"

	cfgpb "go.chromium.org/luci/cv/api/config/v2"
	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/configs/prjcfg/prjcfgtest"
	"go.chromium.org/luci/cv/internal/cvtesting"

	. "github.com/smartystreets/goconvey/convey"
)

func TestCLQueryBuilder(t *testing.T) {
	t.Parallel()

	Convey("CLQueryBuilder works", t, func() {
		ct := cvtesting.Test{}
		ctx, cancel := ct.SetUp()
		defer cancel()

		getAll := func(qb CLQueryBuilder) common.RunIDs {
			return execQueryInTestSameRunsAndKeys(ctx, qb)
		}

		makeRun := func(proj string, delay time.Duration, clids ...common.CLID) common.RunID {
			createdAt := ct.Clock.Now().Add(delay)
			runID := common.MakeRunID(proj, createdAt, 1, []byte{0, byte(delay / time.Millisecond)})
			So(datastore.Put(ctx, &Run{ID: runID, CLs: clids}), ShouldBeNil)
			for _, clid := range clids {
				So(datastore.Put(ctx, &RunCL{
					Run:       datastore.MakeKey(ctx, RunKind, string(runID)),
					ID:        clid,
					IndexedID: clid,
				}), ShouldBeNil)
			}
			return runID
		}

		clA, clB, clZ := common.CLID(1), common.CLID(2), common.CLID(3)

		// RunID below are ordered lexicographically.
		bond9 := makeRun("bond", 9*time.Millisecond, clA)
		bond4 := makeRun("bond", 4*time.Millisecond, clA, clB)
		bond2 := makeRun("bond", 2*time.Millisecond, clA)
		dart5 := makeRun("dart", 5*time.Millisecond, clA)
		dart3 := makeRun("dart", 3*time.Millisecond, clA)
		rust1 := makeRun("rust", 1*time.Millisecond, clA, clB)
		xero7 := makeRun("xero", 7*time.Millisecond, clA)

		Convey("CL without Runs", func() {
			qb := CLQueryBuilder{CLID: clZ}
			So(getAll(qb), ShouldResemble, common.RunIDs(nil))
		})

		Convey("CL with some Runs", func() {
			qb := CLQueryBuilder{CLID: clB}
			So(getAll(qb), ShouldResemble, common.RunIDs{bond4, rust1})
		})

		Convey("CL with all Runs", func() {
			qb := CLQueryBuilder{CLID: clA}
			So(getAll(qb), ShouldResemble, common.RunIDs{bond9, bond4, bond2, dart5, dart3, rust1, xero7})
		})

		Convey("Filter by Project", func() {
			qb := CLQueryBuilder{CLID: clA, Project: "bond"}
			So(getAll(qb), ShouldResemble, common.RunIDs{bond9, bond4, bond2})
		})

		Convey("Filtering by Project and Min with diff project", func() {
			qb := CLQueryBuilder{CLID: clA, Project: "dart", MinExcl: bond4}
			So(getAll(qb), ShouldResemble, common.RunIDs{dart5, dart3})

			qb = CLQueryBuilder{CLID: clA, Project: "dart", MinExcl: rust1}
			_, err := qb.BuildKeysOnly(ctx).Finalize()
			So(err, ShouldEqual, datastore.ErrNullQuery)
		})

		Convey("Filtering by Project and Max with diff project", func() {
			qb := CLQueryBuilder{CLID: clA, Project: "dart", MaxExcl: xero7}
			So(getAll(qb), ShouldResemble, common.RunIDs{dart5, dart3})

			qb = CLQueryBuilder{CLID: clA, Project: "dart", MaxExcl: bond4}
			_, err := qb.BuildKeysOnly(ctx).Finalize()
			So(err, ShouldEqual, datastore.ErrNullQuery)
		})

		Convey("Before", func() {
			qb := CLQueryBuilder{CLID: clA}.BeforeInProject(bond9)
			So(getAll(qb), ShouldResemble, common.RunIDs{bond4, bond2})

			qb = CLQueryBuilder{CLID: clA}.BeforeInProject(bond4)
			So(getAll(qb), ShouldResemble, common.RunIDs{bond2})

			qb = CLQueryBuilder{CLID: clA}.BeforeInProject(bond2)
			So(getAll(qb), ShouldResemble, common.RunIDs(nil))
		})

		Convey("After", func() {
			qb := CLQueryBuilder{CLID: clA}.AfterInProject(bond2)
			So(getAll(qb), ShouldResemble, common.RunIDs{bond9, bond4})

			qb = CLQueryBuilder{CLID: clA}.AfterInProject(bond4)
			So(getAll(qb), ShouldResemble, common.RunIDs{bond9})

			qb = CLQueryBuilder{CLID: clA}.AfterInProject(bond9)
			So(getAll(qb), ShouldResemble, common.RunIDs(nil))
		})

		Convey("Obeys limit and returns correct page token", func() {
			qb := CLQueryBuilder{CLID: clA, Limit: 1}.AfterInProject(bond2)
			runs1, pageToken1, err := qb.LoadRuns(ctx)
			So(err, ShouldBeNil)
			So(idsOf(runs1), ShouldResemble, common.RunIDs{bond9})
			So(pageToken1, ShouldNotBeNil)

			qb = qb.PageToken(pageToken1)
			So(qb.MinExcl, ShouldResemble, bond9)
			runs2, pageToken2, err := qb.LoadRuns(ctx)
			So(err, ShouldBeNil)
			So(idsOf(runs2), ShouldResemble, common.RunIDs{bond4})
			So(pageToken2, ShouldNotBeNil)

			qb = qb.PageToken(pageToken2)
			So(qb.MinExcl, ShouldResemble, bond4)
			runs3, pageToken3, err := qb.LoadRuns(ctx)
			So(err, ShouldBeNil)
			So(runs3, ShouldBeEmpty)
			So(pageToken3, ShouldBeNil)
		})

		Convey("After and Before", func() {
			qb := CLQueryBuilder{CLID: clA}.AfterInProject(bond2).BeforeInProject(bond9)
			So(getAll(qb), ShouldResemble, common.RunIDs{bond4})
		})

		Convey("Invalid usage panics", func() {
			So(func() { CLQueryBuilder{CLID: clA, Project: "dart"}.BeforeInProject(bond2) }, ShouldPanic)
			So(func() { CLQueryBuilder{CLID: clA, Project: "dart"}.AfterInProject(bond2) }, ShouldPanic)
			So(func() { CLQueryBuilder{CLID: clA}.AfterInProject(dart3).BeforeInProject(xero7) }, ShouldPanic)
		})
	})
}

func TestProjectQueryBuilder(t *testing.T) {
	t.Parallel()

	Convey("ProjectQueryBuilder works", t, func() {
		ct := cvtesting.Test{}
		ctx, cancel := ct.SetUp()
		defer cancel()

		getAll := func(qb ProjectQueryBuilder) common.RunIDs {
			return execQueryInTestSameRunsAndKeys(ctx, qb)
		}

		makeRun := func(proj string, delay time.Duration, s Status) common.RunID {
			createdAt := ct.Clock.Now().Add(delay)
			runID := common.MakeRunID(proj, createdAt, 1, []byte{0, byte(delay / time.Millisecond)})
			So(datastore.Put(ctx, &Run{ID: runID, Status: s}), ShouldBeNil)
			return runID
		}

		// RunID below are ordered lexicographically.
		bond9 := makeRun("bond", 9*time.Millisecond, Status_RUNNING)
		bond4 := makeRun("bond", 4*time.Millisecond, Status_FAILED)
		bond2 := makeRun("bond", 2*time.Millisecond, Status_CANCELLED)
		xero7 := makeRun("xero", 7*time.Millisecond, Status_RUNNING)
		xero6 := makeRun("xero", 6*time.Millisecond, Status_RUNNING)
		xero5 := makeRun("xero", 5*time.Millisecond, Status_SUCCEEDED)

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
				qb := ProjectQueryBuilder{Project: "xero", Status: Status_RUNNING}
				So(getAll(qb), ShouldResemble, common.RunIDs{xero7, xero6})

				qb = ProjectQueryBuilder{Project: "xero", Status: Status_SUCCEEDED}
				So(getAll(qb), ShouldResemble, common.RunIDs{xero5})
			})
			Convey("Status_ENDED_MASK", func() {
				qb := ProjectQueryBuilder{Project: "bond", Status: Status_ENDED_MASK}
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
				qb := ProjectQueryBuilder{Status: Status_FAILED}.After(bond2).Before(bond9)
				So(getAll(qb), ShouldResemble, common.RunIDs{bond4})
				qb = ProjectQueryBuilder{Status: Status_SUCCEEDED}.After(bond2).Before(bond9)
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

func TestRecentQueryBuilder(t *testing.T) {
	t.Parallel()

	Convey("RecentQueryBuilder works", t, func() {
		ct := cvtesting.Test{}
		ctx, cancel := ct.SetUp()
		defer cancel()

		// checkOrder verifies this order:
		//  * DESC Created (== ASC InverseTS, or latest first)
		//  * ASC  Project
		//  * ASC  RunID (the remaining part of RunID)
		checkOrder := func(runs []*Run) {
			for i := range runs {
				if i == 0 {
					continue
				}
				switch l, r := runs[i-1], runs[i]; {
				case !l.CreateTime.Equal(r.CreateTime):
					So(l.CreateTime, ShouldHappenAfter, r.CreateTime)
				case l.ID.LUCIProject() != r.ID.LUCIProject():
					So(l.ID.LUCIProject(), ShouldBeLessThan, r.ID.LUCIProject())
				default:
					// Same CreateTime and Project.
					So(l.ID, ShouldBeLessThan, r.ID)
				}
			}
		}

		getAllWithPageToken := func(qb RecentQueryBuilder) (common.RunIDs, *PageToken) {
			keys, runs, pt := execQueryInTest(ctx, qb)
			checkOrder(runs)
			// Check that loading Runs returns the same values in the same order.
			So(idsOf(runs), ShouldResemble, idsOfKeys(keys))
			assertCorrectPageToken(qb, keys, pt)
			return idsOfKeys(keys), pt
		}

		getAll := func(qb RecentQueryBuilder) common.RunIDs {
			out, _ := getAllWithPageToken(qb)
			return out
		}

		// Choose epoch such that inverseTS of Run ID has zeros at the end for
		// ease of debugging.
		epoch := testclock.TestRecentTimeUTC.Truncate(time.Millisecond).Add(498490844 * time.Millisecond)

		makeRun := func(project, createdAfter, remainder int) *Run {
			remBytes, err := hex.DecodeString(fmt.Sprintf("%02d", remainder))
			if err != nil {
				panic(err)
			}
			createTime := epoch.Add(time.Duration(createdAfter) * time.Millisecond)
			id := common.MakeRunID(
				fmt.Sprintf("p%02d", project),
				createTime,
				1,
				remBytes,
			)
			return &Run{ID: id, CreateTime: createTime}
		}

		placeRuns := func(runs ...*Run) common.RunIDs {
			ids := make(common.RunIDs, len(runs))
			So(datastore.Put(ctx, runs), ShouldBeNil)
			projects := stringset.New(10)
			for i, r := range runs {
				projects.Add(r.ID.LUCIProject())
				ids[i] = r.ID
			}
			for p := range projects {
				prjcfgtest.Create(ctx, p, &cfgpb.Config{})
			}
			return ids
		}

		Convey("just one project", func() {
			expIDs := placeRuns(
				// project, creationDelay, hash.
				makeRun(1, 90, 11),
				makeRun(1, 80, 12),
				makeRun(1, 70, 13),
				makeRun(1, 70, 14),
				makeRun(1, 70, 15),
				makeRun(1, 60, 11),
				makeRun(1, 60, 12),
			)
			Convey("without paging", func() {
				So(getAll(RecentQueryBuilder{Limit: 128}), ShouldResemble, expIDs)
			})
			Convey("with paging", func() {
				page, next := getAllWithPageToken(RecentQueryBuilder{Limit: 3})
				So(page, ShouldResemble, expIDs[:3])
				So(getAll(RecentQueryBuilder{Limit: 3}.PageToken(next)), ShouldResemble, expIDs[3:6])
			})
		})

		Convey("two projects with overlapping timestaps", func() {
			expIDs := placeRuns(
				// project, creationDelay, hash.
				makeRun(1, 90, 11),
				makeRun(1, 80, 12), // same creationDelay, but smaller project
				makeRun(2, 80, 12),
				makeRun(2, 80, 13), // later hash
				makeRun(2, 70, 13),
				makeRun(1, 60, 12),
			)
			Convey("without paging", func() {
				So(getAll(RecentQueryBuilder{Limit: 128}), ShouldResemble, expIDs)
			})
			Convey("with paging", func() {
				page, next := getAllWithPageToken(RecentQueryBuilder{Limit: 2})
				So(page, ShouldResemble, expIDs[:2])
				So(getAll(RecentQueryBuilder{Limit: 4}.PageToken(next)), ShouldResemble, expIDs[2:6])
			})
		})

		Convey("large scale", func() {
			var runs []*Run
			for p := 50; p < 60; p++ {
				// Distribute # of Runs unevenly across projects.
				for c := p - 49; c > 0; c-- {
					// Create some Runs with the same start timestamp.
					for r := 90; r <= 90+c%3; r++ {
						runs = append(runs, makeRun(p, c, r))
					}
				}
			}
			placeRuns(runs...)
			So(len(runs), ShouldBeLessThan, 128)

			_, _ = Println("without paging")
			all := getAll(RecentQueryBuilder{Limit: 128})
			So(len(all), ShouldEqual, len(runs))

			_, _ = Println("with paging")
			page, next := getAllWithPageToken(RecentQueryBuilder{Limit: 13})
			So(page, ShouldResemble, all[:13])
			So(getAll(RecentQueryBuilder{Limit: 7}.PageToken(next)), ShouldResemble, all[13:20])
		})
	})
}

// TestLoadRunsFromQuery provides additional coverage to loadRunsFromQuery
// which isn't achieved in other tests in this file, where loadRunsFromQuery is
// tested indirectly as part of ...QueryBuilder.LoadRuns().
func TestLoadRunsFromQuery(t *testing.T) {
	t.Parallel()

	Convey("loadRunsFromQuery", t, func() {
		ct := cvtesting.Test{}
		ctx, cancel := ct.SetUp()
		defer cancel()

		makeRun := func(proj string, delay time.Duration, clids ...common.CLID) common.RunID {
			createdAt := ct.Clock.Now().Add(delay)
			runID := common.MakeRunID(proj, createdAt, 1, []byte{0, byte(delay / time.Millisecond)})
			So(datastore.Put(ctx, &Run{ID: runID, CLs: clids}), ShouldBeNil)
			for _, clid := range clids {
				So(datastore.Put(ctx, &RunCL{
					Run:       datastore.MakeKey(ctx, RunKind, string(runID)),
					ID:        clid,
					IndexedID: clid,
				}), ShouldBeNil)
			}
			return runID
		}

		clA, clB, clZ := common.CLID(1), common.CLID(2), common.CLID(3)

		// RunID below are ordered lexicographically.
		bond9 := makeRun("bond", 9*time.Millisecond, clA)
		bond4 := makeRun("bond", 4*time.Millisecond, clA, clB)
		bond2 := makeRun("bond", 2*time.Millisecond, clA) // ignored by aclChecker
		dart5 := makeRun("dart", 5*time.Millisecond, clA)
		dart3 := makeRun("dart", 3*time.Millisecond, clA)      // ignored by aclChecker
		rust8 := makeRun("rust", 8*time.Millisecond, clA, clB) // ignored by aclChecker
		rust1 := makeRun("rust", 1*time.Millisecond, clA, clB)
		xero7 := makeRun("xero", 7*time.Millisecond, clA)

		errNotFound := appstatus.Error(codes.NotFound, "but really, no permission")
		aclChecker := &fakeRunChecker{
			before: map[common.RunID]error{
				bond2: errNotFound,
				rust8: errNotFound,
			},
			after: map[common.RunID]error{
				dart3: errNotFound,
			},
		}

		Convey("If there is no limit, page token must not be returned but ACLs must be obeyed", func() {
			q := CLQueryBuilder{CLID: clA}
			runs, pt, err := loadRunsFromQuery(ctx, q, aclChecker)
			So(err, ShouldBeNil)
			So(idsOf(runs), ShouldResemble, common.RunIDs{bond9, bond4, dart5, rust1, xero7})
			So(pt, ShouldBeNil)
		})

		Convey("Obeys and provides exactly the limit of Runs when available", func() {
			Convey("without ACLs filtering", func() {
				q := CLQueryBuilder{CLID: clA, Limit: 3}
				runs, pt, err := loadRunsFromQuery(ctx, q)
				So(err, ShouldBeNil)
				So(idsOf(runs), ShouldResemble, common.RunIDs{bond9, bond4, bond2})
				So(pt, ShouldNotBeNil)
			})

			Convey("even with ACLs filtering", func() {
				Convey("limit=3", func() {
					q := CLQueryBuilder{CLID: clA, Limit: 3}
					runs, pt, err := loadRunsFromQuery(ctx, q, aclChecker)
					So(err, ShouldBeNil)
					So(idsOf(runs), ShouldResemble, common.RunIDs{bond9, bond4, dart5})
					_, _ = Println("and chooses pagetoken to maximally avoid redundant work")
					// NOTE: the second behind-the-scenes query should have fetched
					// keys for {dart5,dart3,rust8}.
					So(pt.GetRun(), ShouldResemble, string(rust8))

					q = q.PageToken(pt)
					runs, pt, err = loadRunsFromQuery(ctx, q, aclChecker)
					So(err, ShouldBeNil)
					So(idsOf(runs), ShouldResemble, common.RunIDs{rust1, xero7})
					So(pt, ShouldBeNil)
				})
				Convey("limit=4", func() {
					q := CLQueryBuilder{CLID: clA, Limit: 4}
					runs, pt, err := loadRunsFromQuery(ctx, q, aclChecker)
					So(err, ShouldBeNil)
					So(idsOf(runs), ShouldResemble, common.RunIDs{bond9, bond4, dart5, rust1})
					_, _ = Println("and chooses pagetoken to maximally avoid redundant work")
					// NOTE: the second behind-the-scenes query should have fetched
					// keys for {dart3,rust8,rust1,xero7} but xero7 didn't fit into
					// `runs`, thus next time it's sufficient to start with >rust1.
					So(pt.GetRun(), ShouldResemble, string(rust1))

					q = q.PageToken(pt)
					runs, pt, err = loadRunsFromQuery(ctx, q, aclChecker)
					So(err, ShouldBeNil)
					So(idsOf(runs), ShouldResemble, common.RunIDs{xero7})
					So(pt, ShouldBeNil)
				})
				Convey("limit=5", func() {
					q := CLQueryBuilder{CLID: clA, Limit: 5}
					runs, pt, err := loadRunsFromQuery(ctx, q, aclChecker)
					So(err, ShouldBeNil)
					So(idsOf(runs), ShouldResemble, common.RunIDs{bond9, bond4, dart5, rust1, xero7})
					// NOTE: the second behind-the-scenes query should have fetched
					// keys for {rust8,rust1,xero7}, and thus exhausted the search.
					So(pt, ShouldBeNil)
				})
			})
		})

		Convey("If there are exactly `limit` Runs, page token may be returned", func() {
			q := CLQueryBuilder{CLID: clB, Limit: 3}
			runs, pt, err := loadRunsFromQuery(ctx, q, aclChecker)
			So(err, ShouldBeNil)
			So(idsOf(runs), ShouldResemble, common.RunIDs{bond4, rust1})
			if pt != nil {
				// If page token is returned, then next page must be empty.
				q = q.PageToken(pt)
				runs, pt, err = loadRunsFromQuery(ctx, q, aclChecker)
				So(err, ShouldBeNil)
				So(runs, ShouldBeEmpty)
				So(pt, ShouldBeNil)
			}
		})

		Convey("If there are no Runs, then limit is not obeyed", func() {
			q := CLQueryBuilder{CLID: clZ, Limit: 1}
			runs, pt, err := loadRunsFromQuery(ctx, q, aclChecker)
			So(err, ShouldBeNil)
			So(runs, ShouldBeEmpty)
			So(pt, ShouldBeNil)
		})

		Convey("Limits looping when most Runs are filtered out, returning partial page instead", func() {
			// Create batches Runs referencing the same CL with every batch having 1
			// visible Run and many not visible ones.
			// It's important that each batch has a shared project name prefix,
			// s.t. that CLQueryBuilder iterates Runs in the order of batches.
			const batchesN = queryStopAfterIterations + 2
			const invisibleN = 7
			const visibleSuffix = "visible"
			const invisibleSuffix = "zzz-no-access" // must be after visibleSuffix lexicographically
			var visible, invisible common.RunIDs
			visibleSet := stringset.New(batchesN)
			clX := common.CLID(25)
			for i := 1; i < batchesN; i++ {
				creationDelay := time.Duration(i) * time.Millisecond
				id := makeRun(fmt.Sprintf("%d-%s", i, visibleSuffix), creationDelay, clX)
				visible = append(visible, id)
				visibleSet.Add(string(id))
				for j := 1; j <= invisibleN; j++ {
					id := makeRun(fmt.Sprintf("%d-%s-%02d", i, invisibleSuffix, j), creationDelay, clX)
					invisible = append(invisible, id)
				}
			}
			// Simulate extremely slow ACL checks.
			const aclCheckDuration = queryStopAfterDuration / 20
			// Quick check test setup: the queryStopAfterDuration must be hit before
			// queryStopAfterIterations are done.
			So(aclCheckDuration*(invisibleN+1)*queryStopAfterIterations, ShouldBeGreaterThan, queryStopAfterDuration)
			aclChecker.beforeFunc = func(id common.RunID) error {
				ct.Clock.Add(aclCheckDuration)
				if visibleSet.Has(string(id)) {
					return nil
				}
				return errNotFound
			}

			tStart := ct.Clock.Now()
			// Run the query s.t. every iteration it loads an entire batch.
			q := CLQueryBuilder{CLID: clX, Limit: invisibleN + 1}
			runs, pt, err := loadRunsFromQuery(ctx, q, aclChecker)
			took := ct.Clock.Now().Sub(tStart)
			So(err, ShouldBeNil)
			// Shouldn't have terminated prematurely.
			So(took, ShouldBeGreaterThanOrEqualTo, queryStopAfterDuration)
			// It should have loaded `queryStopAfterIterations` of batches, each
			// having 1 visible Run.
			So(idsOf(runs), ShouldResemble, visible[:queryStopAfterIterations])
			// Which must be less than the requested limit.
			So(len(runs), ShouldBeLessThan, q.Limit)
			// Quick check test assumption.
			So(invisibleSuffix, ShouldBeGreaterThan, visibleSuffix)
			// Thus the page token must point to the last Run among invisible ones.
			So(pt.GetRun(), ShouldResemble, string(invisible[(queryStopAfterIterations*invisibleN)-1]))
		})
	})
}

type queryInTest interface {
	runKeysQuery
	LoadRuns(context.Context, ...LoadRunChecker) ([]*Run, *PageToken, error)
}

func execQueryInTest(ctx context.Context, q queryInTest, checkers ...LoadRunChecker) ([]*datastore.Key, []*Run, *PageToken) {
	keys, err := q.GetAllRunKeys(ctx)
	So(err, ShouldBeNil)
	runs, pageToken, err := q.LoadRuns(ctx, checkers...)
	So(err, ShouldBeNil)
	return keys, runs, pageToken
}

func execQueryInTestSameRunsAndKeysWithPageToken(ctx context.Context, q queryInTest, checkers ...LoadRunChecker) (common.RunIDs, *PageToken) {
	keys, runs, pageToken := execQueryInTest(ctx, q, checkers...)
	ids := idsOfKeys(keys)
	So(ids, ShouldResemble, idsOf(runs))
	assertCorrectPageToken(q, keys, pageToken)
	return ids, pageToken
}

func assertCorrectPageToken(q queryInTest, keys []*datastore.Key, pageToken *PageToken) {
	if l := len(keys); q.qLimit() <= 0 || l < int(q.qLimit()) {
		So(pageToken, ShouldBeNil)
	} else {
		So(pageToken.GetRun(), ShouldResemble, keys[l-1].StringID())
	}
}

func execQueryInTestSameRunsAndKeys(ctx context.Context, q queryInTest, checkers ...LoadRunChecker) common.RunIDs {
	ids, _ := execQueryInTestSameRunsAndKeysWithPageToken(ctx, q, checkers...)
	return ids
}

func idsOf(runs []*Run) common.RunIDs {
	if len(runs) == 0 {
		return nil
	}
	out := make(common.RunIDs, len(runs))
	for i, r := range runs {
		out[i] = r.ID
	}
	return out
}

func idsOfKeys(keys []*datastore.Key) common.RunIDs {
	if len(keys) == 0 {
		return nil
	}
	out := make(common.RunIDs, len(keys))
	for i, k := range keys {
		out[i] = common.RunID(k.StringID())
	}
	return out
}
