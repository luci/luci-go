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
	"context"
	"encoding/hex"
	"fmt"
	"testing"
	"time"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/gae/service/datastore"

	cfgpb "go.chromium.org/luci/cv/api/config/v2"
	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/configs/prjcfg/prjcfgtest"
	"go.chromium.org/luci/cv/internal/cvtesting"
	"go.chromium.org/luci/cv/internal/run"

	. "github.com/smartystreets/goconvey/convey"
)

func TestRecentQueryBuilder(t *testing.T) {
	t.Parallel()

	Convey("RecentQueryBuilder works", t, func() {
		ct := cvtesting.Test{}
		ctx := ct.SetUp(t)

		// checkOrder verifies this order:
		//  * DESC Created (== ASC InverseTS, or latest first)
		//  * ASC  Project
		//  * ASC  RunID (the remaining part of RunID)
		checkOrder := func(runs []*run.Run) {
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

		makeRun := func(project, createdAfter, remainder int) *run.Run {
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
			return &run.Run{ID: id, CreateTime: createTime}
		}

		placeRuns := func(runs ...*run.Run) common.RunIDs {
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
			Convey("without read access to project", func() {
				runs, pageToken, err := RecentQueryBuilder{
					CheckProjectAccess: func(ctx context.Context, proj string) (bool, error) {
						if proj == expIDs[0].LUCIProject() {
							return false, nil
						}
						return true, nil

					},
				}.LoadRuns(ctx)
				So(err, ShouldBeNil)
				So(pageToken, ShouldBeNil)
				So(runs, ShouldBeEmpty)
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
			Convey("without read access to one project", func() {
				prj1 := expIDs[0].LUCIProject()
				prj2 := expIDs[2].LUCIProject()
				runs, pageToken, err := RecentQueryBuilder{
					CheckProjectAccess: func(ctx context.Context, proj string) (bool, error) {
						if proj == prj1 {
							return false, nil
						}
						return true, nil
					},
				}.LoadRuns(ctx)
				So(err, ShouldBeNil)
				So(pageToken, ShouldBeNil)
				checkOrder(runs)
				for _, r := range runs {
					So(r.ID.LUCIProject(), ShouldResemble, prj2)
				}
				So(idsOf(runs), ShouldResemble, expIDs[2:5])
			})
			Convey("without read access to any project", func() {
				prj1 := expIDs[0].LUCIProject()
				prj2 := expIDs[2].LUCIProject()
				runs, pageToken, err := RecentQueryBuilder{
					CheckProjectAccess: func(ctx context.Context, proj string) (bool, error) {
						switch proj {
						case prj1, prj2:
							return false, nil
						default:
							return true, nil
						}
					},
				}.LoadRuns(ctx)
				So(err, ShouldBeNil)
				So(pageToken, ShouldBeNil)
				So(runs, ShouldBeEmpty)
			})
		})

		Convey("large scale", func() {
			var runs []*run.Run
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
