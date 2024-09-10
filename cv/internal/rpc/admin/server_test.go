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

package admin

import (
	"context"
	"encoding/hex"
	"fmt"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"

	cfgpb "go.chromium.org/luci/cv/api/config/v2"
	"go.chromium.org/luci/cv/internal/changelist"
	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/configs/prjcfg/prjcfgtest"
	"go.chromium.org/luci/cv/internal/cvtesting"
	"go.chromium.org/luci/cv/internal/gerrit/poller"
	"go.chromium.org/luci/cv/internal/prjmanager"
	"go.chromium.org/luci/cv/internal/prjmanager/prjpb"
	adminpb "go.chromium.org/luci/cv/internal/rpc/admin/api"
	"go.chromium.org/luci/cv/internal/run"
	"go.chromium.org/luci/cv/internal/run/eventpb"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/data/stringset"
	. "go.chromium.org/luci/common/testing/assertions"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/convey"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestGetProject(t *testing.T) {
	t.Parallel()

	ftt.Run("GetProject works", t, func(t *ftt.Test) {
		ct := cvtesting.Test{}
		ctx := ct.SetUp(t)

		const lProject = "luci"
		a := AdminServer{}

		t.Run("without access", func(t *ftt.Test) {
			ctx = auth.WithState(ctx, &authtest.FakeState{
				Identity: "anonymous:anonymous",
			})
			_, err := a.GetProject(ctx, &adminpb.GetProjectRequest{Project: lProject})
			assert.Loosely(t, err, convey.Adapt(ShouldBeRPCPermissionDenied)())
		})

		t.Run("with access", func(t *ftt.Test) {
			ctx = auth.WithState(ctx, &authtest.FakeState{
				Identity:       "user:admin@example.com",
				IdentityGroups: []string{allowGroup},
			})
			t.Run("not exists", func(t *ftt.Test) {
				_, err := a.GetProject(ctx, &adminpb.GetProjectRequest{Project: lProject})
				assert.Loosely(t, err, convey.Adapt(ShouldBeRPCNotFound)())
			})
		})
	})
}

func TestGetProjectLogs(t *testing.T) {
	t.Parallel()

	ftt.Run("GetProjectLogs works", t, func(t *ftt.Test) {
		ct := cvtesting.Test{}
		ctx := ct.SetUp(t)

		const lProject = "luci"
		a := AdminServer{}

		t.Run("without access", func(t *ftt.Test) {
			ctx = auth.WithState(ctx, &authtest.FakeState{
				Identity: "anonymous:anonymous",
			})
			_, err := a.GetProjectLogs(ctx, &adminpb.GetProjectLogsRequest{Project: lProject})
			assert.Loosely(t, err, convey.Adapt(ShouldBeRPCPermissionDenied)())
		})

		t.Run("with access", func(t *ftt.Test) {
			ctx = auth.WithState(ctx, &authtest.FakeState{
				Identity:       "user:admin@example.com",
				IdentityGroups: []string{allowGroup},
			})
			t.Run("nothing", func(t *ftt.Test) {
				resp, err := a.GetProjectLogs(ctx, &adminpb.GetProjectLogsRequest{Project: lProject})
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, resp.GetLogs(), should.HaveLength(0))
			})
		})
	})
}

func TestGetRun(t *testing.T) {
	t.Parallel()

	ftt.Run("GetRun works", t, func(t *ftt.Test) {
		ct := cvtesting.Test{}
		ctx := ct.SetUp(t)

		const rid = "proj/123-deadbeef"
		assert.Loosely(t, datastore.Put(ctx, &run.Run{ID: rid}), should.BeNil)

		a := AdminServer{}

		t.Run("without access", func(t *ftt.Test) {
			ctx = auth.WithState(ctx, &authtest.FakeState{
				Identity: "anonymous:anonymous",
			})
			_, err := a.GetRun(ctx, &adminpb.GetRunRequest{Run: rid})
			assert.Loosely(t, err, convey.Adapt(ShouldBeRPCPermissionDenied)())
		})

		t.Run("with access", func(t *ftt.Test) {
			ctx = auth.WithState(ctx, &authtest.FakeState{
				Identity:       "user:admin@example.com",
				IdentityGroups: []string{allowGroup},
			})
			t.Run("not exists", func(t *ftt.Test) {
				_, err := a.GetRun(ctx, &adminpb.GetRunRequest{Run: rid + "cafe"})
				assert.Loosely(t, err, convey.Adapt(ShouldBeRPCNotFound)())
			})
			t.Run("exists", func(t *ftt.Test) {
				_, err := a.GetRun(ctx, &adminpb.GetRunRequest{Run: rid})
				assert.Loosely(t, err, convey.Adapt(ShouldBeRPCOK)())
			})
		})
	})
}

func TestGetCL(t *testing.T) {
	t.Parallel()

	ftt.Run("GetCL works", t, func(t *ftt.Test) {
		ct := cvtesting.Test{}
		ctx := ct.SetUp(t)

		a := AdminServer{}

		t.Run("without access", func(t *ftt.Test) {
			ctx = auth.WithState(ctx, &authtest.FakeState{
				Identity: "anonymous:anonymous",
			})
			_, err := a.GetCL(ctx, &adminpb.GetCLRequest{Id: 123})
			assert.Loosely(t, err, convey.Adapt(ShouldBeRPCPermissionDenied)())
		})

		t.Run("with access", func(t *ftt.Test) {
			ctx = auth.WithState(ctx, &authtest.FakeState{
				Identity:       "user:admin@example.com",
				IdentityGroups: []string{allowGroup},
			})
			assert.Loosely(t, datastore.Put(ctx, &changelist.CL{ID: 123, ExternalID: changelist.MustGobID("x-review", 44)}), should.BeNil)
			resp, err := a.GetCL(ctx, &adminpb.GetCLRequest{Id: 123})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, resp.GetExternalId(), should.Equal("gerrit/x-review/44"))
		})
	})
}

func TestGetPoller(t *testing.T) {
	t.Parallel()

	ftt.Run("GetPoller works", t, func(t *ftt.Test) {
		ct := cvtesting.Test{}
		ctx := ct.SetUp(t)

		const lProject = "luci"
		a := AdminServer{}

		t.Run("without access", func(t *ftt.Test) {
			ctx = auth.WithState(ctx, &authtest.FakeState{
				Identity: "anonymous:anonymous",
			})
			_, err := a.GetPoller(ctx, &adminpb.GetPollerRequest{Project: lProject})
			assert.Loosely(t, err, convey.Adapt(ShouldBeRPCPermissionDenied)())
		})

		t.Run("with access", func(t *ftt.Test) {
			ctx = auth.WithState(ctx, &authtest.FakeState{
				Identity:       "user:admin@example.com",
				IdentityGroups: []string{allowGroup},
			})
			now := ct.Clock.Now().UTC().Truncate(time.Second)
			assert.Loosely(t, datastore.Put(ctx, &poller.State{
				LuciProject: lProject,
				UpdateTime:  now,
			}), should.BeNil)
			resp, err := a.GetPoller(ctx, &adminpb.GetPollerRequest{Project: lProject})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, resp.GetUpdateTime().AsTime(), should.Resemble(now))
		})
	})
}

func TestSearchRuns(t *testing.T) {
	t.Parallel()

	ftt.Run("SearchRuns works", t, func(t *ftt.Test) {
		ct := cvtesting.Test{}
		ctx := ct.SetUp(t)

		const lProject = "proj"
		const earlierID = lProject + "/124-earlier-has-higher-number"
		const laterID = lProject + "/123-later-has-lower-number"
		const diffProjectID = "diff/333-foo-bar"

		idsOf := func(resp *adminpb.RunsResponse) []string {
			out := make([]string, len(resp.GetRuns()))
			for i, r := range resp.GetRuns() {
				out[i] = r.GetId()
			}
			return out
		}

		assertOrdered := func(resp *adminpb.RunsResponse) {
			for i := 1; i < len(resp.GetRuns()); i++ {
				curr := resp.GetRuns()[i]
				prev := resp.GetRuns()[i-1]

				currID := common.RunID(curr.GetId())
				prevID := common.RunID(prev.GetId())
				assert.Loosely(t, prevID, should.NotResemble(currID))

				currTS := curr.GetCreateTime().AsTime()
				prevTS := prev.GetCreateTime().AsTime()
				if !prevTS.Equal(currTS) {
					assert.Loosely(t, prevTS, should.HappenAfter(currTS))
					continue
				}
				// Same TS.

				if prevID.LUCIProject() != currID.LUCIProject() {
					assert.Loosely(t, prevID.LUCIProject(), should.BeLessThan(currID.LUCIProject()))
					continue
				}
				// Same LUCI project.
				assert.Loosely(t, prevID, should.BeLessThan(currID))
			}
		}

		a := AdminServer{}

		fetchAll := func(ctx context.Context, origReq *adminpb.SearchRunsRequest) *adminpb.RunsResponse {
			var out *adminpb.RunsResponse
			// Allow orig request to have page token already.
			nextPageToken := origReq.GetPageToken()
			for out == nil || nextPageToken != "" {
				req := proto.Clone(origReq).(*adminpb.SearchRunsRequest)
				req.PageToken = nextPageToken
				resp, err := a.SearchRuns(ctx, req)
				assert.Loosely(t, err, should.BeNil)
				assertOrdered(resp)
				if out == nil {
					out = resp
				} else {
					out.Runs = append(out.Runs, resp.GetRuns()...)
				}
				nextPageToken = resp.GetNextPageToken()
			}
			return out
		}

		t.Run("without access", func(t *ftt.Test) {
			ctx = auth.WithState(ctx, &authtest.FakeState{
				Identity: "anonymous:anonymous",
			})
			_, err := a.SearchRuns(ctx, &adminpb.SearchRunsRequest{Project: lProject})
			assert.Loosely(t, err, convey.Adapt(ShouldBeRPCPermissionDenied)())
		})

		t.Run("with access", func(t *ftt.Test) {
			ctx = auth.WithState(ctx, &authtest.FakeState{
				Identity:       "user:admin@example.com",
				IdentityGroups: []string{allowGroup},
			})
			t.Run("no runs exist", func(t *ftt.Test) {
				resp, err := a.SearchRuns(ctx, &adminpb.SearchRunsRequest{Project: lProject})
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, resp.GetRuns(), should.HaveLength(0))
			})
			t.Run("two runs of the same project", func(t *ftt.Test) {
				assert.Loosely(t, datastore.Put(ctx, &run.Run{ID: earlierID}, &run.Run{ID: laterID}), should.BeNil)
				resp, err := a.SearchRuns(ctx, &adminpb.SearchRunsRequest{Project: lProject})
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, idsOf(resp), should.Resemble([]string{laterID, earlierID}))

				t.Run("with page size exactly 2", func(t *ftt.Test) {
					req := &adminpb.SearchRunsRequest{Project: lProject, PageSize: 2}
					resp1, err := a.SearchRuns(ctx, req)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, idsOf(resp1), should.Resemble([]string{laterID, earlierID}))

					req.PageToken = resp1.GetNextPageToken()
					resp2, err := a.SearchRuns(ctx, req)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, idsOf(resp2), should.BeEmpty)
				})

				t.Run("with page size of 1", func(t *ftt.Test) {
					req := &adminpb.SearchRunsRequest{Project: lProject, PageSize: 1}
					resp1, err := a.SearchRuns(ctx, req)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, idsOf(resp1), should.Resemble([]string{laterID}))

					req.PageToken = resp1.GetNextPageToken()
					resp2, err := a.SearchRuns(ctx, req)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, idsOf(resp2), should.Resemble([]string{earlierID}))

					req.PageToken = resp2.GetNextPageToken()
					resp3, err := a.SearchRuns(ctx, req)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, idsOf(resp3), should.BeEmpty)
					assert.Loosely(t, resp3.GetNextPageToken(), should.BeEmpty)
				})
			})

			t.Run("filtering", func(t *ftt.Test) {
				const gHost = "r-review.example.com"
				cl1 := changelist.MustGobID(gHost, 1).MustCreateIfNotExists(ctx)
				cl2 := changelist.MustGobID(gHost, 2).MustCreateIfNotExists(ctx)

				assert.Loosely(t, datastore.Put(ctx,
					&run.Run{
						ID:     earlierID,
						Status: run.Status_CANCELLED,
						CLs:    common.MakeCLIDs(1, 2),
					},
					&run.RunCL{Run: datastore.MakeKey(ctx, common.RunKind, earlierID), ID: cl1.ID, IndexedID: cl1.ID},
					&run.RunCL{Run: datastore.MakeKey(ctx, common.RunKind, earlierID), ID: cl2.ID, IndexedID: cl2.ID},

					&run.Run{
						ID:     laterID,
						Status: run.Status_RUNNING,
						CLs:    common.MakeCLIDs(1),
					},
					&run.RunCL{Run: datastore.MakeKey(ctx, common.RunKind, laterID), ID: cl1.ID, IndexedID: cl1.ID},
				), should.BeNil)

				t.Run("exact", func(t *ftt.Test) {
					resp, err := a.SearchRuns(ctx, &adminpb.SearchRunsRequest{
						Project: lProject,
						Status:  run.Status_CANCELLED,
					})
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, idsOf(resp), should.Resemble([]string{earlierID}))
				})

				t.Run("ended", func(t *ftt.Test) {
					resp, err := a.SearchRuns(ctx, &adminpb.SearchRunsRequest{
						Project: lProject,
						Status:  run.Status_ENDED_MASK,
					})
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, idsOf(resp), should.Resemble([]string{earlierID}))
				})

				t.Run("with CL", func(t *ftt.Test) {
					resp, err := a.SearchRuns(ctx, &adminpb.SearchRunsRequest{
						Cl: &adminpb.GetCLRequest{ExternalId: string(cl2.ExternalID)},
					})
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, idsOf(resp), should.Resemble([]string{earlierID}))
				})

				t.Run("with CL and run status", func(t *ftt.Test) {
					resp, err := a.SearchRuns(ctx, &adminpb.SearchRunsRequest{
						Cl:     &adminpb.GetCLRequest{ExternalId: string(cl1.ExternalID)},
						Status: run.Status_ENDED_MASK,
					})
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, idsOf(resp), should.Resemble([]string{earlierID}))
				})

				t.Run("with CL + paging", func(t *ftt.Test) {
					req := &adminpb.SearchRunsRequest{
						Cl:       &adminpb.GetCLRequest{ExternalId: string(cl1.ExternalID)},
						PageSize: 1,
					}
					total := fetchAll(ctx, req)
					assert.Loosely(t, idsOf(total), should.Resemble([]string{laterID, earlierID}))
				})

				t.Run("with CL across projects and paging", func(t *ftt.Test) {
					// Make CL1 included in 3 runs: diffProjectID, laterID, earlierID.
					assert.Loosely(t, datastore.Put(ctx,
						&run.Run{
							ID:     diffProjectID,
							Status: run.Status_RUNNING,
							CLs:    common.MakeCLIDs(1),
						},
						&run.RunCL{Run: datastore.MakeKey(ctx, common.RunKind, diffProjectID), ID: cl1.ID, IndexedID: cl1.ID},
					), should.BeNil)

					req := &adminpb.SearchRunsRequest{
						Cl:       &adminpb.GetCLRequest{ExternalId: string(cl1.ExternalID)},
						PageSize: 2,
					}
					total := fetchAll(ctx, req)
					assert.Loosely(t, idsOf(total), should.Resemble([]string{diffProjectID, laterID, earlierID}))
				})

				t.Run("with CL and project and paging", func(t *ftt.Test) {
					// Make CL1 included in 3 runs: diffProjectID, laterID, earlierID.
					assert.Loosely(t, datastore.Put(ctx,
						&run.Run{
							ID:     diffProjectID,
							Status: run.Status_RUNNING,
							CLs:    common.MakeCLIDs(1),
						},
						&run.RunCL{Run: datastore.MakeKey(ctx, common.RunKind, diffProjectID), ID: cl1.ID, IndexedID: cl1.ID},
					), should.BeNil)

					req := &adminpb.SearchRunsRequest{
						Cl:       &adminpb.GetCLRequest{ExternalId: string(cl1.ExternalID)},
						Project:  lProject,
						PageSize: 1,
					}
					total := fetchAll(ctx, req)
					assert.Loosely(t, idsOf(total), should.Resemble([]string{laterID, earlierID}))
				})
			})

			t.Run("runs aross all projects", func(t *ftt.Test) {
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

				placeRuns := func(runs ...*run.Run) []string {
					ids := make([]string, len(runs))
					assert.Loosely(t, datastore.Put(ctx, runs), should.BeNil)
					projects := stringset.New(10)
					for i, r := range runs {
						projects.Add(r.ID.LUCIProject())
						ids[i] = string(r.ID)
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
						resp, err := a.SearchRuns(ctx, &adminpb.SearchRunsRequest{PageSize: 128})
						assert.Loosely(t, err, should.BeNil)
						assertOrdered(resp)
						assert.Loosely(t, idsOf(resp), should.Resemble(expIDs))
					})
					t.Run("with paging", func(t *ftt.Test) {
						total := fetchAll(ctx, &adminpb.SearchRunsRequest{PageSize: 2})
						assertOrdered(total)
						assert.Loosely(t, idsOf(total), should.Resemble(expIDs))
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
						resp, err := a.SearchRuns(ctx, &adminpb.SearchRunsRequest{PageSize: 128})
						assert.Loosely(t, err, should.BeNil)
						assertOrdered(resp)
						assert.Loosely(t, idsOf(resp), should.Resemble(expIDs))
					})
					t.Run("with paging", func(t *ftt.Test) {
						total := fetchAll(ctx, &adminpb.SearchRunsRequest{PageSize: 1})
						assertOrdered(total)
						assert.Loosely(t, idsOf(total), should.Resemble(expIDs))
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

					t.Run("without paging", func(t *ftt.Test) {
						assert.Loosely(t, len(runs), should.BeLessThan(128))
						resp, err := a.SearchRuns(ctx, &adminpb.SearchRunsRequest{PageSize: 128})
						assert.Loosely(t, err, should.BeNil)
						assertOrdered(resp)
						assert.Loosely(t, resp.GetRuns(), should.HaveLength(len(runs)))
					})
					t.Run("with paging", func(t *ftt.Test) {
						total := fetchAll(ctx, &adminpb.SearchRunsRequest{PageSize: 8})
						assertOrdered(total)
						assert.Loosely(t, total.GetRuns(), should.HaveLength(len(runs)))
					})
				})
			})
		})
	})
}

func TestDeleteProjectEvents(t *testing.T) {
	t.Parallel()

	ftt.Run("DeleteProjectEvents works", t, func(t *ftt.Test) {
		ct := cvtesting.Test{}
		ctx := ct.SetUp(t)

		const lProject = "luci"
		a := AdminServer{}

		t.Run("without access", func(t *ftt.Test) {
			ctx = auth.WithState(ctx, &authtest.FakeState{
				Identity: "anonymous:anonymous",
			})
			_, err := a.DeleteProjectEvents(ctx, &adminpb.DeleteProjectEventsRequest{Project: lProject, Limit: 10})
			assert.Loosely(t, err, convey.Adapt(ShouldBeRPCPermissionDenied)())
		})

		t.Run("with access", func(t *ftt.Test) {
			ctx = auth.WithState(ctx, &authtest.FakeState{
				Identity:       "user:admin@example.com",
				IdentityGroups: []string{allowGroup},
			})
			pm := prjmanager.NewNotifier(ct.TQDispatcher)

			assert.Loosely(t, pm.Poke(ctx, lProject), should.BeNil)
			assert.Loosely(t, pm.UpdateConfig(ctx, lProject), should.BeNil)
			assert.Loosely(t, pm.Poke(ctx, lProject), should.BeNil)

			t.Run("All", func(t *ftt.Test) {
				resp, err := a.DeleteProjectEvents(ctx, &adminpb.DeleteProjectEventsRequest{Project: lProject, Limit: 10})
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, resp.GetEvents(), should.Resemble(map[string]int64{"*prjpb.Event_Poke": 2, "*prjpb.Event_NewConfig": 1}))
			})

			t.Run("Limited", func(t *ftt.Test) {
				resp, err := a.DeleteProjectEvents(ctx, &adminpb.DeleteProjectEventsRequest{Project: lProject, Limit: 2})
				assert.Loosely(t, err, should.BeNil)
				sum := int64(0)
				for _, v := range resp.GetEvents() {
					sum += v
				}
				assert.Loosely(t, sum, should.Equal(2))
			})
		})
	})
}

func TestRefreshProjectCLs(t *testing.T) {
	t.Parallel()

	ftt.Run("RefreshProjectCLs works", t, func(t *ftt.Test) {
		ct := cvtesting.Test{}
		ctx := ct.SetUp(t)

		const lProject = "luci"
		a := AdminServer{
			clUpdater:  changelist.NewUpdater(ct.TQDispatcher, nil),
			pmNotifier: prjmanager.NewNotifier(ct.TQDispatcher),
		}

		t.Run("without access", func(t *ftt.Test) {
			ctx = auth.WithState(ctx, &authtest.FakeState{
				Identity: "anonymous:anonymous",
			})
			_, err := a.RefreshProjectCLs(ctx, &adminpb.RefreshProjectCLsRequest{Project: lProject})
			assert.Loosely(t, err, convey.Adapt(ShouldBeRPCPermissionDenied)())
		})

		t.Run("with access", func(t *ftt.Test) {
			ctx = auth.WithState(ctx, &authtest.FakeState{
				Identity:       "user:admin@example.com",
				IdentityGroups: []string{allowGroup},
			})

			assert.Loosely(t, datastore.Put(ctx, &prjmanager.Project{
				ID: lProject,
				State: &prjpb.PState{
					Pcls: []*prjpb.PCL{
						{Clid: 1},
					},
				},
			}), should.BeNil)
			cl := changelist.CL{
				ID:         1,
				EVersion:   4,
				ExternalID: changelist.MustGobID("x-review.example.com", 55),
			}
			assert.Loosely(t, datastore.Put(ctx, &cl), should.BeNil)

			resp, err := a.RefreshProjectCLs(ctx, &adminpb.RefreshProjectCLsRequest{Project: lProject})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, resp.GetClVersions(), should.Resemble(map[int64]int64{1: 4}))
			scheduledIDs := stringset.New(1)
			for _, p := range ct.TQ.Tasks().Payloads() {
				if tsk, ok := p.(*changelist.UpdateCLTask); ok {
					scheduledIDs.Add(tsk.GetExternalId())
				}
			}
			assert.Loosely(t, scheduledIDs.ToSortedSlice(), should.Resemble([]string{string(cl.ExternalID)}))
		})
	})
}

func TestSendProjectEvent(t *testing.T) {
	t.Parallel()

	ftt.Run("SendProjectEvent works", t, func(t *ftt.Test) {
		ct := cvtesting.Test{}
		ctx := ct.SetUp(t)

		const lProject = "luci"
		a := AdminServer{
			pmNotifier: prjmanager.NewNotifier(ct.TQDispatcher),
		}

		t.Run("without access", func(t *ftt.Test) {
			ctx = auth.WithState(ctx, &authtest.FakeState{
				Identity: "anonymous:anonymous",
			})
			_, err := a.SendProjectEvent(ctx, &adminpb.SendProjectEventRequest{Project: lProject})
			assert.Loosely(t, err, convey.Adapt(ShouldBeRPCPermissionDenied)())
		})

		t.Run("with access", func(t *ftt.Test) {
			ctx = auth.WithState(ctx, &authtest.FakeState{
				Identity:       "user:admin@example.com",
				IdentityGroups: []string{allowGroup},
			})
			t.Run("not exists", func(t *ftt.Test) {
				_, err := a.SendProjectEvent(ctx, &adminpb.SendProjectEventRequest{
					Project: lProject,
					Event:   &prjpb.Event{Event: &prjpb.Event_Poke{Poke: &prjpb.Poke{}}},
				})
				assert.Loosely(t, err, convey.Adapt(ShouldBeRPCNotFound)())
			})
		})
	})
}

func TestSendRunEvent(t *testing.T) {
	t.Parallel()

	ftt.Run("SendRunEvent works", t, func(t *ftt.Test) {
		ct := cvtesting.Test{}
		ctx := ct.SetUp(t)

		const rid = "proj/123-deadbeef"
		a := AdminServer{
			runNotifier: run.NewNotifier(ct.TQDispatcher),
		}

		t.Run("without access", func(t *ftt.Test) {
			ctx = auth.WithState(ctx, &authtest.FakeState{
				Identity: "anonymous:anonymous",
			})
			_, err := a.SendRunEvent(ctx, &adminpb.SendRunEventRequest{Run: rid})
			assert.Loosely(t, err, convey.Adapt(ShouldBeRPCPermissionDenied)())
		})

		t.Run("with access", func(t *ftt.Test) {
			ctx = auth.WithState(ctx, &authtest.FakeState{
				Identity:       "user:admin@example.com",
				IdentityGroups: []string{allowGroup},
			})
			t.Run("not exists", func(t *ftt.Test) {
				_, err := a.SendRunEvent(ctx, &adminpb.SendRunEventRequest{
					Run:   rid,
					Event: &eventpb.Event{Event: &eventpb.Event_Poke{Poke: &eventpb.Poke{}}},
				})
				assert.Loosely(t, err, convey.Adapt(ShouldBeRPCNotFound)())
			})
		})
	})
}

func TestScheduleTask(t *testing.T) {
	t.Parallel()

	ftt.Run("ScheduleTask works", t, func(t *ftt.Test) {
		ct := cvtesting.Test{}
		ctx := ct.SetUp(t)

		const lProject = "infra"
		a := AdminServer{
			tqDispatcher: ct.TQDispatcher,
			pmNotifier:   prjmanager.NewNotifier(ct.TQDispatcher),
		}
		req := &adminpb.ScheduleTaskRequest{
			DeduplicationKey: "key",
			ManageProject: &prjpb.ManageProjectTask{
				Eta:         timestamppb.New(ct.Clock.Now()),
				LuciProject: lProject,
			},
		}
		reqTrans := &adminpb.ScheduleTaskRequest{
			KickManageProject: &prjpb.KickManageProjectTask{
				Eta:         timestamppb.New(ct.Clock.Now()),
				LuciProject: lProject,
			},
		}

		t.Run("without access", func(t *ftt.Test) {
			ctx = auth.WithState(ctx, &authtest.FakeState{
				Identity: "anonymous:anonymous",
			})
			_, err := a.ScheduleTask(ctx, req)
			assert.Loosely(t, err, convey.Adapt(ShouldBeRPCPermissionDenied)())
		})

		t.Run("with access", func(t *ftt.Test) {
			ctx = auth.WithState(ctx, &authtest.FakeState{
				Identity:       "user:admin@example.com",
				IdentityGroups: []string{allowGroup},
			})
			t.Run("OK", func(t *ftt.Test) {
				t.Run("Non-Transactional", func(t *ftt.Test) {
					_, err := a.ScheduleTask(ctx, req)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, ct.TQ.Tasks().Payloads(), should.Resemble([]proto.Message{
						req.GetManageProject(),
					}))
				})
				t.Run("Transactional", func(t *ftt.Test) {
					_, err := a.ScheduleTask(ctx, reqTrans)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, ct.TQ.Tasks().Payloads(), should.Resemble([]proto.Message{
						reqTrans.GetKickManageProject(),
					}))
				})
			})
			t.Run("InvalidArgument", func(t *ftt.Test) {
				t.Run("Missing payload", func(t *ftt.Test) {
					req.ManageProject = nil
					_, err := a.ScheduleTask(ctx, req)
					assert.Loosely(t, err, convey.Adapt(ShouldBeRPCInvalidArgument)("none given"))
				})
				t.Run("Two payloads", func(t *ftt.Test) {
					req.KickManageProject = reqTrans.GetKickManageProject()
					_, err := a.ScheduleTask(ctx, req)
					assert.Loosely(t, err, convey.Adapt(ShouldBeRPCInvalidArgument)("but 2+ given"))
				})
				t.Run("Trans + DeduplicationKey is not allwoed", func(t *ftt.Test) {
					reqTrans.DeduplicationKey = "beef"
					_, err := a.ScheduleTask(ctx, reqTrans)
					assert.Loosely(t, err, convey.Adapt(ShouldBeRPCInvalidArgument)(`"KickManageProjectTask" is transactional`))
				})
			})
		})
	})
}
