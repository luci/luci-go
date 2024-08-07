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

	. "github.com/smartystreets/goconvey/convey"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/data/stringset"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestGetProject(t *testing.T) {
	t.Parallel()

	Convey("GetProject works", t, func() {
		ct := cvtesting.Test{}
		ctx := ct.SetUp(t)

		const lProject = "luci"
		a := AdminServer{}

		Convey("without access", func() {
			ctx = auth.WithState(ctx, &authtest.FakeState{
				Identity: "anonymous:anonymous",
			})
			_, err := a.GetProject(ctx, &adminpb.GetProjectRequest{Project: lProject})
			So(err, ShouldBeRPCPermissionDenied)
		})

		Convey("with access", func() {
			ctx = auth.WithState(ctx, &authtest.FakeState{
				Identity:       "user:admin@example.com",
				IdentityGroups: []string{allowGroup},
			})
			Convey("not exists", func() {
				_, err := a.GetProject(ctx, &adminpb.GetProjectRequest{Project: lProject})
				So(err, ShouldBeRPCNotFound)
			})
		})
	})
}

func TestGetProjectLogs(t *testing.T) {
	t.Parallel()

	Convey("GetProjectLogs works", t, func() {
		ct := cvtesting.Test{}
		ctx := ct.SetUp(t)

		const lProject = "luci"
		a := AdminServer{}

		Convey("without access", func() {
			ctx = auth.WithState(ctx, &authtest.FakeState{
				Identity: "anonymous:anonymous",
			})
			_, err := a.GetProjectLogs(ctx, &adminpb.GetProjectLogsRequest{Project: lProject})
			So(err, ShouldBeRPCPermissionDenied)
		})

		Convey("with access", func() {
			ctx = auth.WithState(ctx, &authtest.FakeState{
				Identity:       "user:admin@example.com",
				IdentityGroups: []string{allowGroup},
			})
			Convey("nothing", func() {
				resp, err := a.GetProjectLogs(ctx, &adminpb.GetProjectLogsRequest{Project: lProject})
				So(err, ShouldBeNil)
				So(resp.GetLogs(), ShouldHaveLength, 0)
			})
		})
	})
}

func TestGetRun(t *testing.T) {
	t.Parallel()

	Convey("GetRun works", t, func() {
		ct := cvtesting.Test{}
		ctx := ct.SetUp(t)

		const rid = "proj/123-deadbeef"
		So(datastore.Put(ctx, &run.Run{ID: rid}), ShouldBeNil)

		a := AdminServer{}

		Convey("without access", func() {
			ctx = auth.WithState(ctx, &authtest.FakeState{
				Identity: "anonymous:anonymous",
			})
			_, err := a.GetRun(ctx, &adminpb.GetRunRequest{Run: rid})
			So(err, ShouldBeRPCPermissionDenied)
		})

		Convey("with access", func() {
			ctx = auth.WithState(ctx, &authtest.FakeState{
				Identity:       "user:admin@example.com",
				IdentityGroups: []string{allowGroup},
			})
			Convey("not exists", func() {
				_, err := a.GetRun(ctx, &adminpb.GetRunRequest{Run: rid + "cafe"})
				So(err, ShouldBeRPCNotFound)
			})
			Convey("exists", func() {
				_, err := a.GetRun(ctx, &adminpb.GetRunRequest{Run: rid})
				So(err, ShouldBeRPCOK)
			})
		})
	})
}

func TestGetCL(t *testing.T) {
	t.Parallel()

	Convey("GetCL works", t, func() {
		ct := cvtesting.Test{}
		ctx := ct.SetUp(t)

		a := AdminServer{}

		Convey("without access", func() {
			ctx = auth.WithState(ctx, &authtest.FakeState{
				Identity: "anonymous:anonymous",
			})
			_, err := a.GetCL(ctx, &adminpb.GetCLRequest{Id: 123})
			So(err, ShouldBeRPCPermissionDenied)
		})

		Convey("with access", func() {
			ctx = auth.WithState(ctx, &authtest.FakeState{
				Identity:       "user:admin@example.com",
				IdentityGroups: []string{allowGroup},
			})
			So(datastore.Put(ctx, &changelist.CL{ID: 123, ExternalID: changelist.MustGobID("x-review", 44)}), ShouldBeNil)
			resp, err := a.GetCL(ctx, &adminpb.GetCLRequest{Id: 123})
			So(err, ShouldBeNil)
			So(resp.GetExternalId(), ShouldEqual, "gerrit/x-review/44")
		})
	})
}

func TestGetPoller(t *testing.T) {
	t.Parallel()

	Convey("GetPoller works", t, func() {
		ct := cvtesting.Test{}
		ctx := ct.SetUp(t)

		const lProject = "luci"
		a := AdminServer{}

		Convey("without access", func() {
			ctx = auth.WithState(ctx, &authtest.FakeState{
				Identity: "anonymous:anonymous",
			})
			_, err := a.GetPoller(ctx, &adminpb.GetPollerRequest{Project: lProject})
			So(err, ShouldBeRPCPermissionDenied)
		})

		Convey("with access", func() {
			ctx = auth.WithState(ctx, &authtest.FakeState{
				Identity:       "user:admin@example.com",
				IdentityGroups: []string{allowGroup},
			})
			now := ct.Clock.Now().UTC().Truncate(time.Second)
			So(datastore.Put(ctx, &poller.State{
				LuciProject: lProject,
				UpdateTime:  now,
			}), ShouldBeNil)
			resp, err := a.GetPoller(ctx, &adminpb.GetPollerRequest{Project: lProject})
			So(err, ShouldBeNil)
			So(resp.GetUpdateTime().AsTime(), ShouldResemble, now)
		})
	})
}

func TestSearchRuns(t *testing.T) {
	t.Parallel()

	Convey("SearchRuns works", t, func() {
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
				So(prevID, ShouldNotResemble, currID)

				currTS := curr.GetCreateTime().AsTime()
				prevTS := prev.GetCreateTime().AsTime()
				if !prevTS.Equal(currTS) {
					So(prevTS, ShouldHappenAfter, currTS)
					continue
				}
				// Same TS.

				if prevID.LUCIProject() != currID.LUCIProject() {
					So(prevID.LUCIProject(), ShouldBeLessThan, currID.LUCIProject())
					continue
				}
				// Same LUCI project.
				So(prevID, ShouldBeLessThan, currID)
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
				So(err, ShouldBeNil)
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

		Convey("without access", func() {
			ctx = auth.WithState(ctx, &authtest.FakeState{
				Identity: "anonymous:anonymous",
			})
			_, err := a.SearchRuns(ctx, &adminpb.SearchRunsRequest{Project: lProject})
			So(err, ShouldBeRPCPermissionDenied)
		})

		Convey("with access", func() {
			ctx = auth.WithState(ctx, &authtest.FakeState{
				Identity:       "user:admin@example.com",
				IdentityGroups: []string{allowGroup},
			})
			Convey("no runs exist", func() {
				resp, err := a.SearchRuns(ctx, &adminpb.SearchRunsRequest{Project: lProject})
				So(err, ShouldBeNil)
				So(resp.GetRuns(), ShouldHaveLength, 0)
			})
			Convey("two runs of the same project", func() {
				So(datastore.Put(ctx, &run.Run{ID: earlierID}, &run.Run{ID: laterID}), ShouldBeNil)
				resp, err := a.SearchRuns(ctx, &adminpb.SearchRunsRequest{Project: lProject})
				So(err, ShouldBeNil)
				So(idsOf(resp), ShouldResemble, []string{laterID, earlierID})

				Convey("with page size exactly 2", func() {
					req := &adminpb.SearchRunsRequest{Project: lProject, PageSize: 2}
					resp1, err := a.SearchRuns(ctx, req)
					So(err, ShouldBeNil)
					So(idsOf(resp1), ShouldResemble, []string{laterID, earlierID})

					req.PageToken = resp1.GetNextPageToken()
					resp2, err := a.SearchRuns(ctx, req)
					So(err, ShouldBeNil)
					So(idsOf(resp2), ShouldBeEmpty)
				})

				Convey("with page size of 1", func() {
					req := &adminpb.SearchRunsRequest{Project: lProject, PageSize: 1}
					resp1, err := a.SearchRuns(ctx, req)
					So(err, ShouldBeNil)
					So(idsOf(resp1), ShouldResemble, []string{laterID})

					req.PageToken = resp1.GetNextPageToken()
					resp2, err := a.SearchRuns(ctx, req)
					So(err, ShouldBeNil)
					So(idsOf(resp2), ShouldResemble, []string{earlierID})

					req.PageToken = resp2.GetNextPageToken()
					resp3, err := a.SearchRuns(ctx, req)
					So(err, ShouldBeNil)
					So(idsOf(resp3), ShouldBeEmpty)
					So(resp3.GetNextPageToken(), ShouldBeEmpty)
				})
			})

			Convey("filtering", func() {
				const gHost = "r-review.example.com"
				cl1 := changelist.MustGobID(gHost, 1).MustCreateIfNotExists(ctx)
				cl2 := changelist.MustGobID(gHost, 2).MustCreateIfNotExists(ctx)

				So(datastore.Put(ctx,
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
				), ShouldBeNil)

				Convey("exact", func() {
					resp, err := a.SearchRuns(ctx, &adminpb.SearchRunsRequest{
						Project: lProject,
						Status:  run.Status_CANCELLED,
					})
					So(err, ShouldBeNil)
					So(idsOf(resp), ShouldResemble, []string{earlierID})
				})

				Convey("ended", func() {
					resp, err := a.SearchRuns(ctx, &adminpb.SearchRunsRequest{
						Project: lProject,
						Status:  run.Status_ENDED_MASK,
					})
					So(err, ShouldBeNil)
					So(idsOf(resp), ShouldResemble, []string{earlierID})
				})

				Convey("with CL", func() {
					resp, err := a.SearchRuns(ctx, &adminpb.SearchRunsRequest{
						Cl: &adminpb.GetCLRequest{ExternalId: string(cl2.ExternalID)},
					})
					So(err, ShouldBeNil)
					So(idsOf(resp), ShouldResemble, []string{earlierID})
				})

				Convey("with CL and run status", func() {
					resp, err := a.SearchRuns(ctx, &adminpb.SearchRunsRequest{
						Cl:     &adminpb.GetCLRequest{ExternalId: string(cl1.ExternalID)},
						Status: run.Status_ENDED_MASK,
					})
					So(err, ShouldBeNil)
					So(idsOf(resp), ShouldResemble, []string{earlierID})
				})

				Convey("with CL + paging", func() {
					req := &adminpb.SearchRunsRequest{
						Cl:       &adminpb.GetCLRequest{ExternalId: string(cl1.ExternalID)},
						PageSize: 1,
					}
					total := fetchAll(ctx, req)
					So(idsOf(total), ShouldResemble, []string{laterID, earlierID})
				})

				Convey("with CL across projects and paging", func() {
					// Make CL1 included in 3 runs: diffProjectID, laterID, earlierID.
					So(datastore.Put(ctx,
						&run.Run{
							ID:     diffProjectID,
							Status: run.Status_RUNNING,
							CLs:    common.MakeCLIDs(1),
						},
						&run.RunCL{Run: datastore.MakeKey(ctx, common.RunKind, diffProjectID), ID: cl1.ID, IndexedID: cl1.ID},
					), ShouldBeNil)

					req := &adminpb.SearchRunsRequest{
						Cl:       &adminpb.GetCLRequest{ExternalId: string(cl1.ExternalID)},
						PageSize: 2,
					}
					total := fetchAll(ctx, req)
					So(idsOf(total), ShouldResemble, []string{diffProjectID, laterID, earlierID})
				})

				Convey("with CL and project and paging", func() {
					// Make CL1 included in 3 runs: diffProjectID, laterID, earlierID.
					So(datastore.Put(ctx,
						&run.Run{
							ID:     diffProjectID,
							Status: run.Status_RUNNING,
							CLs:    common.MakeCLIDs(1),
						},
						&run.RunCL{Run: datastore.MakeKey(ctx, common.RunKind, diffProjectID), ID: cl1.ID, IndexedID: cl1.ID},
					), ShouldBeNil)

					req := &adminpb.SearchRunsRequest{
						Cl:       &adminpb.GetCLRequest{ExternalId: string(cl1.ExternalID)},
						Project:  lProject,
						PageSize: 1,
					}
					total := fetchAll(ctx, req)
					So(idsOf(total), ShouldResemble, []string{laterID, earlierID})
				})
			})

			Convey("runs aross all projects", func() {
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
					So(datastore.Put(ctx, runs), ShouldBeNil)
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
						resp, err := a.SearchRuns(ctx, &adminpb.SearchRunsRequest{PageSize: 128})
						So(err, ShouldBeNil)
						assertOrdered(resp)
						So(idsOf(resp), ShouldResemble, expIDs)
					})
					Convey("with paging", func() {
						total := fetchAll(ctx, &adminpb.SearchRunsRequest{PageSize: 2})
						assertOrdered(total)
						So(idsOf(total), ShouldResemble, expIDs)
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
						resp, err := a.SearchRuns(ctx, &adminpb.SearchRunsRequest{PageSize: 128})
						So(err, ShouldBeNil)
						assertOrdered(resp)
						So(idsOf(resp), ShouldResemble, expIDs)
					})
					Convey("with paging", func() {
						total := fetchAll(ctx, &adminpb.SearchRunsRequest{PageSize: 1})
						assertOrdered(total)
						So(idsOf(total), ShouldResemble, expIDs)
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

					Convey("without paging", func() {
						So(len(runs), ShouldBeLessThan, 128)
						resp, err := a.SearchRuns(ctx, &adminpb.SearchRunsRequest{PageSize: 128})
						So(err, ShouldBeNil)
						assertOrdered(resp)
						So(resp.GetRuns(), ShouldHaveLength, len(runs))
					})
					Convey("with paging", func() {
						total := fetchAll(ctx, &adminpb.SearchRunsRequest{PageSize: 8})
						assertOrdered(total)
						So(total.GetRuns(), ShouldHaveLength, len(runs))
					})
				})
			})
		})
	})
}

func TestDeleteProjectEvents(t *testing.T) {
	t.Parallel()

	Convey("DeleteProjectEvents works", t, func() {
		ct := cvtesting.Test{}
		ctx := ct.SetUp(t)

		const lProject = "luci"
		a := AdminServer{}

		Convey("without access", func() {
			ctx = auth.WithState(ctx, &authtest.FakeState{
				Identity: "anonymous:anonymous",
			})
			_, err := a.DeleteProjectEvents(ctx, &adminpb.DeleteProjectEventsRequest{Project: lProject, Limit: 10})
			So(err, ShouldBeRPCPermissionDenied)
		})

		Convey("with access", func() {
			ctx = auth.WithState(ctx, &authtest.FakeState{
				Identity:       "user:admin@example.com",
				IdentityGroups: []string{allowGroup},
			})
			pm := prjmanager.NewNotifier(ct.TQDispatcher)

			So(pm.Poke(ctx, lProject), ShouldBeNil)
			So(pm.UpdateConfig(ctx, lProject), ShouldBeNil)
			So(pm.Poke(ctx, lProject), ShouldBeNil)

			Convey("All", func() {
				resp, err := a.DeleteProjectEvents(ctx, &adminpb.DeleteProjectEventsRequest{Project: lProject, Limit: 10})
				So(err, ShouldBeNil)
				So(resp.GetEvents(), ShouldResemble, map[string]int64{"*prjpb.Event_Poke": 2, "*prjpb.Event_NewConfig": 1})
			})

			Convey("Limited", func() {
				resp, err := a.DeleteProjectEvents(ctx, &adminpb.DeleteProjectEventsRequest{Project: lProject, Limit: 2})
				So(err, ShouldBeNil)
				sum := int64(0)
				for _, v := range resp.GetEvents() {
					sum += v
				}
				So(sum, ShouldEqual, 2)
			})
		})
	})
}

func TestRefreshProjectCLs(t *testing.T) {
	t.Parallel()

	Convey("RefreshProjectCLs works", t, func() {
		ct := cvtesting.Test{}
		ctx := ct.SetUp(t)

		const lProject = "luci"
		a := AdminServer{
			clUpdater:  changelist.NewUpdater(ct.TQDispatcher, nil),
			pmNotifier: prjmanager.NewNotifier(ct.TQDispatcher),
		}

		Convey("without access", func() {
			ctx = auth.WithState(ctx, &authtest.FakeState{
				Identity: "anonymous:anonymous",
			})
			_, err := a.RefreshProjectCLs(ctx, &adminpb.RefreshProjectCLsRequest{Project: lProject})
			So(err, ShouldBeRPCPermissionDenied)
		})

		Convey("with access", func() {
			ctx = auth.WithState(ctx, &authtest.FakeState{
				Identity:       "user:admin@example.com",
				IdentityGroups: []string{allowGroup},
			})

			So(datastore.Put(ctx, &prjmanager.Project{
				ID: lProject,
				State: &prjpb.PState{
					Pcls: []*prjpb.PCL{
						{Clid: 1},
					},
				},
			}), ShouldBeNil)
			cl := changelist.CL{
				ID:         1,
				EVersion:   4,
				ExternalID: changelist.MustGobID("x-review.example.com", 55),
			}
			So(datastore.Put(ctx, &cl), ShouldBeNil)

			resp, err := a.RefreshProjectCLs(ctx, &adminpb.RefreshProjectCLsRequest{Project: lProject})
			So(err, ShouldBeNil)
			So(resp.GetClVersions(), ShouldResemble, map[int64]int64{1: 4})
			scheduledIDs := stringset.New(1)
			for _, p := range ct.TQ.Tasks().Payloads() {
				if t, ok := p.(*changelist.UpdateCLTask); ok {
					scheduledIDs.Add(t.GetExternalId())
				}
			}
			So(scheduledIDs.ToSortedSlice(), ShouldResemble, []string{string(cl.ExternalID)})
		})
	})
}

func TestSendProjectEvent(t *testing.T) {
	t.Parallel()

	Convey("SendProjectEvent works", t, func() {
		ct := cvtesting.Test{}
		ctx := ct.SetUp(t)

		const lProject = "luci"
		a := AdminServer{
			pmNotifier: prjmanager.NewNotifier(ct.TQDispatcher),
		}

		Convey("without access", func() {
			ctx = auth.WithState(ctx, &authtest.FakeState{
				Identity: "anonymous:anonymous",
			})
			_, err := a.SendProjectEvent(ctx, &adminpb.SendProjectEventRequest{Project: lProject})
			So(err, ShouldBeRPCPermissionDenied)
		})

		Convey("with access", func() {
			ctx = auth.WithState(ctx, &authtest.FakeState{
				Identity:       "user:admin@example.com",
				IdentityGroups: []string{allowGroup},
			})
			Convey("not exists", func() {
				_, err := a.SendProjectEvent(ctx, &adminpb.SendProjectEventRequest{
					Project: lProject,
					Event:   &prjpb.Event{Event: &prjpb.Event_Poke{Poke: &prjpb.Poke{}}},
				})
				So(err, ShouldBeRPCNotFound)
			})
		})
	})
}

func TestSendRunEvent(t *testing.T) {
	t.Parallel()

	Convey("SendRunEvent works", t, func() {
		ct := cvtesting.Test{}
		ctx := ct.SetUp(t)

		const rid = "proj/123-deadbeef"
		a := AdminServer{
			runNotifier: run.NewNotifier(ct.TQDispatcher),
		}

		Convey("without access", func() {
			ctx = auth.WithState(ctx, &authtest.FakeState{
				Identity: "anonymous:anonymous",
			})
			_, err := a.SendRunEvent(ctx, &adminpb.SendRunEventRequest{Run: rid})
			So(err, ShouldBeRPCPermissionDenied)
		})

		Convey("with access", func() {
			ctx = auth.WithState(ctx, &authtest.FakeState{
				Identity:       "user:admin@example.com",
				IdentityGroups: []string{allowGroup},
			})
			Convey("not exists", func() {
				_, err := a.SendRunEvent(ctx, &adminpb.SendRunEventRequest{
					Run:   rid,
					Event: &eventpb.Event{Event: &eventpb.Event_Poke{Poke: &eventpb.Poke{}}},
				})
				So(err, ShouldBeRPCNotFound)
			})
		})
	})
}

func TestScheduleTask(t *testing.T) {
	t.Parallel()

	Convey("ScheduleTask works", t, func() {
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

		Convey("without access", func() {
			ctx = auth.WithState(ctx, &authtest.FakeState{
				Identity: "anonymous:anonymous",
			})
			_, err := a.ScheduleTask(ctx, req)
			So(err, ShouldBeRPCPermissionDenied)
		})

		Convey("with access", func() {
			ctx = auth.WithState(ctx, &authtest.FakeState{
				Identity:       "user:admin@example.com",
				IdentityGroups: []string{allowGroup},
			})
			Convey("OK", func() {
				Convey("Non-Transactional", func() {
					_, err := a.ScheduleTask(ctx, req)
					So(err, ShouldBeNil)
					So(ct.TQ.Tasks().Payloads(), ShouldResembleProto, []proto.Message{
						req.GetManageProject(),
					})
				})
				Convey("Transactional", func() {
					_, err := a.ScheduleTask(ctx, reqTrans)
					So(err, ShouldBeNil)
					So(ct.TQ.Tasks().Payloads(), ShouldResembleProto, []proto.Message{
						reqTrans.GetKickManageProject(),
					})
				})
			})
			Convey("InvalidArgument", func() {
				Convey("Missing payload", func() {
					req.ManageProject = nil
					_, err := a.ScheduleTask(ctx, req)
					So(err, ShouldBeRPCInvalidArgument, "none given")
				})
				Convey("Two payloads", func() {
					req.KickManageProject = reqTrans.GetKickManageProject()
					_, err := a.ScheduleTask(ctx, req)
					So(err, ShouldBeRPCInvalidArgument, "but 2+ given")
				})
				Convey("Trans + DeduplicationKey is not allwoed", func() {
					reqTrans.DeduplicationKey = "beef"
					_, err := a.ScheduleTask(ctx, reqTrans)
					So(err, ShouldBeRPCInvalidArgument, `"KickManageProjectTask" is transactional`)
				})
			})
		})
	})
}
