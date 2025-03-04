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
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/service/datastore"

	cfgpb "go.chromium.org/luci/cv/api/config/v2"
	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/configs/prjcfg/prjcfgtest"
	"go.chromium.org/luci/cv/internal/cvtesting"
	"go.chromium.org/luci/cv/internal/run"
)

func TestRecentQueryBuilder(t *testing.T) {
	t.Parallel()

	ftt.Run("RecentQueryBuilder works", t, func(t *ftt.Test) {
		ct := cvtesting.Test{}
		ctx := ct.SetUp(t)

		// checkOrder verifies this order:
		//  * DESC Created (== ASC InverseTS, or latest first)
		//  * ASC  Project
		//  * ASC  RunID (the remaining part of RunID)
		checkOrder := func(t testing.TB, runs []*run.Run) {
			t.Helper()
			for i := range runs {
				if i == 0 {
					continue
				}
				switch l, r := runs[i-1], runs[i]; {
				case !l.CreateTime.Equal(r.CreateTime):
					assert.Loosely(t, l.CreateTime, should.HappenAfter(r.CreateTime), truth.LineContext())
				case l.ID.LUCIProject() != r.ID.LUCIProject():
					assert.Loosely(t, l.ID.LUCIProject(), should.BeLessThan(r.ID.LUCIProject()), truth.LineContext())
				default:
					// Same CreateTime and Project.
					assert.Loosely(t, l.ID, should.BeLessThan(r.ID), truth.LineContext())
				}
			}
		}

		getAllWithPageToken := func(t testing.TB, qb RecentQueryBuilder) (common.RunIDs, *PageToken) {
			t.Helper()
			keys, runs, pt := execQueryInTest(ctx, t, qb)
			checkOrder(t, runs)
			// Check that loading Runs returns the same values in the same order.
			assert.That(t, idsOf(runs), should.Match(idsOfKeys(keys)), truth.LineContext())
			assertCorrectPageToken(t, qb, keys, pt)
			return idsOfKeys(keys), pt
		}

		getAll := func(t testing.TB, qb RecentQueryBuilder) common.RunIDs {
			t.Helper()
			out, _ := getAllWithPageToken(t, qb)
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
			assert.NoErr(t, datastore.Put(ctx, runs))
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

		t.Run("just one project", func(t *ftt.Test) {
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
			t.Run("without paging", func(t *ftt.Test) {
				assert.That(t, getAll(t, RecentQueryBuilder{Limit: 128}), should.Match(expIDs))
			})
			t.Run("with paging", func(t *ftt.Test) {
				page, next := getAllWithPageToken(t, RecentQueryBuilder{Limit: 3})
				assert.That(t, page, should.Match(expIDs[:3]))
				assert.That(t, getAll(t, RecentQueryBuilder{Limit: 3}.PageToken(next)), should.Match(expIDs[3:6]))
			})
			t.Run("without read access to project", func(t *ftt.Test) {
				runs, pageToken, err := RecentQueryBuilder{
					CheckProjectAccess: func(ctx context.Context, proj string) (bool, error) {
						if proj == expIDs[0].LUCIProject() {
							return false, nil
						}
						return true, nil

					},
				}.LoadRuns(ctx)
				assert.NoErr(t, err)
				assert.Loosely(t, pageToken, should.BeNil)
				assert.Loosely(t, runs, should.BeEmpty)
			})
		})

		t.Run("two projects with overlapping timestaps", func(t *ftt.Test) {
			expIDs := placeRuns(
				// project, creationDelay, hash.
				makeRun(1, 90, 11),
				makeRun(1, 80, 12), // same creationDelay, but smaller project
				makeRun(2, 80, 12),
				makeRun(2, 80, 13), // later hash
				makeRun(2, 70, 13),
				makeRun(1, 60, 12),
			)
			t.Run("without paging", func(t *ftt.Test) {
				assert.That(t, getAll(t, RecentQueryBuilder{Limit: 128}), should.Match(expIDs))
			})
			t.Run("with paging", func(t *ftt.Test) {
				page, next := getAllWithPageToken(t, RecentQueryBuilder{Limit: 2})
				assert.That(t, page, should.Match(expIDs[:2]))
				assert.That(t, getAll(t, RecentQueryBuilder{Limit: 4}.PageToken(next)), should.Match(expIDs[2:6]))
			})
			t.Run("without read access to one project", func(t *ftt.Test) {
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
				assert.NoErr(t, err)
				assert.Loosely(t, pageToken, should.BeNil)
				checkOrder(t, runs)
				for _, r := range runs {
					assert.That(t, r.ID.LUCIProject(), should.Match(prj2))
				}
				assert.That(t, idsOf(runs), should.Match(expIDs[2:5]))
			})
			t.Run("without read access to any project", func(t *ftt.Test) {
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
				assert.NoErr(t, err)
				assert.Loosely(t, pageToken, should.BeNil)
				assert.Loosely(t, runs, should.BeEmpty)
			})
		})

		t.Run("large scale", func(t *ftt.Test) {
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
			assert.Loosely(t, len(runs), should.BeLessThan(128))

			t.Log("without paging")
			all := getAll(t, RecentQueryBuilder{Limit: 128})
			assert.Loosely(t, len(all), should.Equal(len(runs)))

			t.Log("with paging")
			page, next := getAllWithPageToken(t, RecentQueryBuilder{Limit: 13})
			assert.That(t, page, should.Match(all[:13]))
			assert.That(t, getAll(t, RecentQueryBuilder{Limit: 7}.PageToken(next)), should.Match(all[13:20]))
		})
	})
}
