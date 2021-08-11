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
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/grpc/grpcutil"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"

	commonpb "go.chromium.org/luci/cv/api/common/v1"
	"go.chromium.org/luci/cv/internal/changelist"
	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/cvtesting"
	"go.chromium.org/luci/cv/internal/gerrit/poller"
	"go.chromium.org/luci/cv/internal/gerrit/updater"
	"go.chromium.org/luci/cv/internal/gerrit/updater/updatertest"
	"go.chromium.org/luci/cv/internal/prjmanager"
	"go.chromium.org/luci/cv/internal/prjmanager/prjpb"
	adminpb "go.chromium.org/luci/cv/internal/rpc/admin/api"
	"go.chromium.org/luci/cv/internal/run"
	"go.chromium.org/luci/cv/internal/run/eventpb"

	. "github.com/smartystreets/goconvey/convey"

	. "go.chromium.org/luci/common/testing/assertions"
)

func TestParseGerritURL(t *testing.T) {
	t.Parallel()

	Convey("parseGerritURL works", t, func() {
		eid, err := parseGerritURL("https://crrev.com/i/12/34")
		So(err, ShouldBeNil)
		So(string(eid), ShouldEqual, "gerrit/chrome-internal-review.googlesource.com/12")

		eid, err = parseGerritURL("https://crrev.com/c/12")
		So(err, ShouldBeNil)
		So(string(eid), ShouldEqual, "gerrit/chromium-review.googlesource.com/12")

		eid, err = parseGerritURL("https://chromium-review.googlesource.com/1541677")
		So(err, ShouldBeNil)
		So(string(eid), ShouldEqual, "gerrit/chromium-review.googlesource.com/1541677")

		eid, err = parseGerritURL("https://pdfium-review.googlesource.com/c/33")
		So(err, ShouldBeNil)
		So(string(eid), ShouldEqual, "gerrit/pdfium-review.googlesource.com/33")

		eid, err = parseGerritURL("https://chromium-review.googlesource.com/#/c/infra/luci/luci-go/+/1541677/7")
		So(err, ShouldBeNil)
		So(string(eid), ShouldEqual, "gerrit/chromium-review.googlesource.com/1541677")

		eid, err = parseGerritURL("https://chromium-review.googlesource.com/c/infra/luci/luci-go/+/2652967")
		So(err, ShouldBeNil)
		So(string(eid), ShouldEqual, "gerrit/chromium-review.googlesource.com/2652967")

		eid, err = parseGerritURL("chromium-review.googlesource.com/2652967")
		So(err, ShouldBeNil)
		So(string(eid), ShouldEqual, "gerrit/chromium-review.googlesource.com/2652967")
	})
}

func TestGetProject(t *testing.T) {
	t.Parallel()

	Convey("GetProject works", t, func() {
		ct := cvtesting.Test{}
		ctx, cancel := ct.SetUp()
		defer cancel()

		const lProject = "luci"
		d := AdminServer{}

		Convey("without access", func() {
			ctx = auth.WithState(ctx, &authtest.FakeState{
				Identity: "anonymous:anonymous",
			})
			_, err := d.GetProject(ctx, &adminpb.GetProjectRequest{Project: lProject})
			So(grpcutil.Code(err), ShouldEqual, codes.PermissionDenied)
		})

		Convey("with access", func() {
			ctx = auth.WithState(ctx, &authtest.FakeState{
				Identity:       "user:admin@example.com",
				IdentityGroups: []string{allowGroup},
			})
			Convey("not exists", func() {
				_, err := d.GetProject(ctx, &adminpb.GetProjectRequest{Project: lProject})
				So(grpcutil.Code(err), ShouldEqual, codes.NotFound)
			})
		})
	})
}

func TestGetProjectLogs(t *testing.T) {
	t.Parallel()

	Convey("GetProjectLogs works", t, func() {
		ct := cvtesting.Test{}
		ctx, cancel := ct.SetUp()
		defer cancel()

		const lProject = "luci"
		d := AdminServer{}

		Convey("without access", func() {
			ctx = auth.WithState(ctx, &authtest.FakeState{
				Identity: "anonymous:anonymous",
			})
			_, err := d.GetProjectLogs(ctx, &adminpb.GetProjectLogsRequest{Project: lProject})
			So(grpcutil.Code(err), ShouldEqual, codes.PermissionDenied)
		})

		Convey("with access", func() {
			ctx = auth.WithState(ctx, &authtest.FakeState{
				Identity:       "user:admin@example.com",
				IdentityGroups: []string{allowGroup},
			})
			Convey("nothing", func() {
				resp, err := d.GetProjectLogs(ctx, &adminpb.GetProjectLogsRequest{Project: lProject})
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
		ctx, cancel := ct.SetUp()
		defer cancel()

		const rid = "proj/123-deadbeef"
		d := AdminServer{}

		Convey("without access", func() {
			ctx = auth.WithState(ctx, &authtest.FakeState{
				Identity: "anonymous:anonymous",
			})
			_, err := d.GetRun(ctx, &adminpb.GetRunRequest{Run: rid})
			So(grpcutil.Code(err), ShouldEqual, codes.PermissionDenied)
		})

		Convey("with access", func() {
			ctx = auth.WithState(ctx, &authtest.FakeState{
				Identity:       "user:admin@example.com",
				IdentityGroups: []string{allowGroup},
			})
			Convey("not exists", func() {
				_, err := d.GetRun(ctx, &adminpb.GetRunRequest{Run: rid})
				So(grpcutil.Code(err), ShouldEqual, codes.NotFound)
			})
		})
	})
}

func TestGetCL(t *testing.T) {
	t.Parallel()

	Convey("GetCL works", t, func() {
		ct := cvtesting.Test{}
		ctx, cancel := ct.SetUp()
		defer cancel()

		d := AdminServer{}

		Convey("without access", func() {
			ctx = auth.WithState(ctx, &authtest.FakeState{
				Identity: "anonymous:anonymous",
			})
			_, err := d.GetCL(ctx, &adminpb.GetCLRequest{Id: 123})
			So(grpcutil.Code(err), ShouldEqual, codes.PermissionDenied)
		})

		Convey("with access", func() {
			ctx = auth.WithState(ctx, &authtest.FakeState{
				Identity:       "user:admin@example.com",
				IdentityGroups: []string{allowGroup},
			})
			So(datastore.Put(ctx, &changelist.CL{ID: 123, ExternalID: changelist.MustGobID("x-review", 44)}), ShouldBeNil)
			resp, err := d.GetCL(ctx, &adminpb.GetCLRequest{Id: 123})
			So(err, ShouldBeNil)
			So(resp.GetExternalId(), ShouldEqual, "gerrit/x-review/44")
		})
	})
}

func TestGetPoller(t *testing.T) {
	t.Parallel()

	Convey("GetPoller works", t, func() {
		ct := cvtesting.Test{}
		ctx, cancel := ct.SetUp()
		defer cancel()

		const lProject = "luci"
		d := AdminServer{}

		Convey("without access", func() {
			ctx = auth.WithState(ctx, &authtest.FakeState{
				Identity: "anonymous:anonymous",
			})
			_, err := d.GetPoller(ctx, &adminpb.GetPollerRequest{Project: lProject})
			So(grpcutil.Code(err), ShouldEqual, codes.PermissionDenied)
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
			resp, err := d.GetPoller(ctx, &adminpb.GetPollerRequest{Project: lProject})
			So(err, ShouldBeNil)
			So(resp.GetUpdateTime().AsTime(), ShouldResemble, now)
		})
	})
}

func TestSearchRuns(t *testing.T) {
	t.Parallel()

	Convey("SearchRuns works", t, func() {
		ct := cvtesting.Test{}
		ctx, cancel := ct.SetUp()
		defer cancel()

		const lProject = "proj"
		const earlierID = lProject + "/124-earlier-has-higher-number"
		const laterID = lProject + "/123-later-has-lower-number"
		d := AdminServer{}

		Convey("without access", func() {
			ctx = auth.WithState(ctx, &authtest.FakeState{
				Identity: "anonymous:anonymous",
			})
			_, err := d.SearchRuns(ctx, &adminpb.SearchRunsRequest{Project: lProject})
			So(grpcutil.Code(err), ShouldEqual, codes.PermissionDenied)
		})

		Convey("with access", func() {
			ctx = auth.WithState(ctx, &authtest.FakeState{
				Identity:       "user:admin@example.com",
				IdentityGroups: []string{allowGroup},
			})
			Convey("no runs exist", func() {
				resp, err := d.SearchRuns(ctx, &adminpb.SearchRunsRequest{Project: lProject})
				So(err, ShouldBeNil)
				So(resp.GetRuns(), ShouldHaveLength, 0)
			})
			Convey("one run", func() {
				So(datastore.Put(ctx, &run.Run{ID: earlierID}, &run.Run{ID: laterID}), ShouldBeNil)
				resp, err := d.SearchRuns(ctx, &adminpb.SearchRunsRequest{Project: lProject})
				So(err, ShouldBeNil)
				So(resp.GetRuns(), ShouldHaveLength, 2)
				So(resp.GetRuns()[0].GetId(), ShouldResemble, laterID)
				So(resp.GetRuns()[1].GetId(), ShouldResemble, earlierID)
			})
			Convey("filtering", func() {
				const gHost = "r-review.example.com"
				cl1 := changelist.MustGobID(gHost, 1).MustCreateIfNotExists(ctx)
				cl2 := changelist.MustGobID(gHost, 2).MustCreateIfNotExists(ctx)

				So(datastore.Put(ctx,
					&run.Run{
						ID:     earlierID,
						Status: commonpb.Run_CANCELLED,
						CLs:    common.MakeCLIDs(1, 2),
					},
					&run.RunCL{Run: datastore.MakeKey(ctx, run.RunKind, earlierID), ID: cl1.ID, IndexedID: cl1.ID},
					&run.RunCL{Run: datastore.MakeKey(ctx, run.RunKind, earlierID), ID: cl2.ID, IndexedID: cl2.ID},

					&run.Run{
						ID:     laterID,
						Status: commonpb.Run_RUNNING,
						CLs:    common.MakeCLIDs(1),
					},
					&run.RunCL{Run: datastore.MakeKey(ctx, run.RunKind, laterID), ID: cl1.ID, IndexedID: cl1.ID},
				), ShouldBeNil)

				Convey("exact", func() {
					resp, err := d.SearchRuns(ctx, &adminpb.SearchRunsRequest{
						Project: lProject,
						Status:  commonpb.Run_CANCELLED,
					})
					So(err, ShouldBeNil)
					So(resp.GetRuns(), ShouldHaveLength, 1)
					So(resp.GetRuns()[0].GetId(), ShouldResemble, earlierID)
				})

				Convey("ended", func() {
					resp, err := d.SearchRuns(ctx, &adminpb.SearchRunsRequest{
						Project: lProject,
						Status:  commonpb.Run_ENDED_MASK,
					})
					So(err, ShouldBeNil)
					So(resp.GetRuns(), ShouldHaveLength, 1)
					So(resp.GetRuns()[0].GetId(), ShouldResemble, earlierID)
				})

				Convey("with CL", func() {
					resp, err := d.SearchRuns(ctx, &adminpb.SearchRunsRequest{
						Cl: &adminpb.GetCLRequest{ExternalId: string(cl2.ExternalID)},
					})
					So(err, ShouldBeNil)
					So(resp.GetRuns(), ShouldHaveLength, 1)
					So(resp.GetRuns()[0].GetId(), ShouldResemble, earlierID)
				})

				Convey("with CL and run status", func() {
					resp, err := d.SearchRuns(ctx, &adminpb.SearchRunsRequest{
						Cl:     &adminpb.GetCLRequest{ExternalId: string(cl1.ExternalID)},
						Status: commonpb.Run_ENDED_MASK,
					})
					So(err, ShouldBeNil)
					So(resp.GetRuns(), ShouldHaveLength, 1)
					So(resp.GetRuns()[0].GetId(), ShouldResemble, earlierID)
				})
			})
		})
	})
}

func TestDeleteProjectEvents(t *testing.T) {
	t.Parallel()

	Convey("DeleteProjectEvents works", t, func() {
		ct := cvtesting.Test{}
		ctx, cancel := ct.SetUp()
		defer cancel()

		const lProject = "luci"
		d := AdminServer{}

		Convey("without access", func() {
			ctx = auth.WithState(ctx, &authtest.FakeState{
				Identity: "anonymous:anonymous",
			})
			_, err := d.DeleteProjectEvents(ctx, &adminpb.DeleteProjectEventsRequest{Project: lProject, Limit: 10})
			So(grpcutil.Code(err), ShouldEqual, codes.PermissionDenied)
		})

		Convey("with access", func() {
			ctx = auth.WithState(ctx, &authtest.FakeState{
				Identity:       "user:admin@example.com",
				IdentityGroups: []string{allowGroup},
			})
			pm := prjmanager.NewNotifier(ct.TQDispatcher)

			So(pm.NotifyCLUpdated(ctx, lProject, common.CLID(1), 1), ShouldBeNil)
			So(pm.NotifyCLUpdated(ctx, lProject, common.CLID(2), 1), ShouldBeNil)
			So(pm.UpdateConfig(ctx, lProject), ShouldBeNil)

			Convey("All", func() {
				resp, err := d.DeleteProjectEvents(ctx, &adminpb.DeleteProjectEventsRequest{Project: lProject, Limit: 10})
				So(err, ShouldBeNil)
				So(resp.GetEvents(), ShouldResemble, map[string]int64{"*prjpb.Event_ClUpdated": 2, "*prjpb.Event_NewConfig": 1})
			})

			Convey("Limited", func() {
				resp, err := d.DeleteProjectEvents(ctx, &adminpb.DeleteProjectEventsRequest{Project: lProject, Limit: 2})
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
		ctx, cancel := ct.SetUp()
		defer cancel()

		const lProject = "luci"
		d := AdminServer{
			GerritUpdater: updater.New(ct.TQDispatcher, ct.GFactory(), nil),
			PMNotifier:    prjmanager.NewNotifier(ct.TQDispatcher),
		}

		Convey("without access", func() {
			ctx = auth.WithState(ctx, &authtest.FakeState{
				Identity: "anonymous:anonymous",
			})
			_, err := d.RefreshProjectCLs(ctx, &adminpb.RefreshProjectCLsRequest{Project: lProject})
			So(grpcutil.Code(err), ShouldEqual, codes.PermissionDenied)
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
			So(datastore.Put(ctx, &changelist.CL{
				ID:         1,
				EVersion:   4,
				ExternalID: changelist.MustGobID("x-review.example.com", 55),
			}), ShouldBeNil)

			resp, err := d.RefreshProjectCLs(ctx, &adminpb.RefreshProjectCLsRequest{Project: lProject})
			So(err, ShouldBeNil)
			So(resp.GetClVersions(), ShouldResemble, map[int64]int64{1: 4})
			So(updatertest.ChangeNumbers(ct.TQ.Tasks()), ShouldResemble, []int64{55})
		})
	})
}

func TestSendProjectEvent(t *testing.T) {
	t.Parallel()

	Convey("SendProjectEvent works", t, func() {
		ct := cvtesting.Test{}
		ctx, cancel := ct.SetUp()
		defer cancel()

		const lProject = "luci"
		d := AdminServer{
			PMNotifier: prjmanager.NewNotifier(ct.TQDispatcher),
		}

		Convey("without access", func() {
			ctx = auth.WithState(ctx, &authtest.FakeState{
				Identity: "anonymous:anonymous",
			})
			_, err := d.SendProjectEvent(ctx, &adminpb.SendProjectEventRequest{Project: lProject})
			So(grpcutil.Code(err), ShouldEqual, codes.PermissionDenied)
		})

		Convey("with access", func() {
			ctx = auth.WithState(ctx, &authtest.FakeState{
				Identity:       "user:admin@example.com",
				IdentityGroups: []string{allowGroup},
			})
			Convey("not exists", func() {
				_, err := d.SendProjectEvent(ctx, &adminpb.SendProjectEventRequest{
					Project: lProject,
					Event:   &prjpb.Event{Event: &prjpb.Event_Poke{Poke: &prjpb.Poke{}}},
				})
				So(grpcutil.Code(err), ShouldEqual, codes.NotFound)
			})
		})
	})
}

func TestSendRunEvent(t *testing.T) {
	t.Parallel()

	Convey("SendRunEvent works", t, func() {
		ct := cvtesting.Test{}
		ctx, cancel := ct.SetUp()
		defer cancel()

		const rid = "proj/123-deadbeef"
		d := AdminServer{
			RunNotifier: run.NewNotifier(ct.TQDispatcher),
		}

		Convey("without access", func() {
			ctx = auth.WithState(ctx, &authtest.FakeState{
				Identity: "anonymous:anonymous",
			})
			_, err := d.SendRunEvent(ctx, &adminpb.SendRunEventRequest{Run: rid})
			So(grpcutil.Code(err), ShouldEqual, codes.PermissionDenied)
		})

		Convey("with access", func() {
			ctx = auth.WithState(ctx, &authtest.FakeState{
				Identity:       "user:admin@example.com",
				IdentityGroups: []string{allowGroup},
			})
			Convey("not exists", func() {
				_, err := d.SendRunEvent(ctx, &adminpb.SendRunEventRequest{
					Run:   rid,
					Event: &eventpb.Event{Event: &eventpb.Event_Poke{Poke: &eventpb.Poke{}}},
				})
				So(grpcutil.Code(err), ShouldEqual, codes.NotFound)
			})
		})
	})
}

func TestScheduleTask(t *testing.T) {
	t.Parallel()

	Convey("ScheduleTask works", t, func() {
		ct := cvtesting.Test{}
		ctx, cancel := ct.SetUp()
		defer cancel()

		const lProject = "infra"
		d := AdminServer{
			TQDispatcher: ct.TQDispatcher,
			PMNotifier:   prjmanager.NewNotifier(ct.TQDispatcher),
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
			_, err := d.ScheduleTask(ctx, req)
			So(grpcutil.Code(err), ShouldEqual, codes.PermissionDenied)
		})

		Convey("with access", func() {
			ctx = auth.WithState(ctx, &authtest.FakeState{
				Identity:       "user:admin@example.com",
				IdentityGroups: []string{allowGroup},
			})
			Convey("OK", func() {
				Convey("Non-Transactional", func() {
					_, err := d.ScheduleTask(ctx, req)
					So(err, ShouldBeNil)
					So(ct.TQ.Tasks().Payloads(), ShouldResembleProto, []proto.Message{
						req.GetManageProject(),
					})
				})
				Convey("Transactional", func() {
					_, err := d.ScheduleTask(ctx, reqTrans)
					So(err, ShouldBeNil)
					So(ct.TQ.Tasks().Payloads(), ShouldResembleProto, []proto.Message{
						reqTrans.GetKickManageProject(),
					})
				})
			})
			Convey("InvalidArgument", func() {
				Convey("Missing payload", func() {
					req.ManageProject = nil
					_, err := d.ScheduleTask(ctx, req)
					So(grpcutil.Code(err), ShouldEqual, codes.InvalidArgument)
					So(err, ShouldErrLike, "none given")
				})
				Convey("Two payloads", func() {
					req.KickManageProject = reqTrans.GetKickManageProject()
					_, err := d.ScheduleTask(ctx, req)
					So(grpcutil.Code(err), ShouldEqual, codes.InvalidArgument)
					So(err, ShouldErrLike, "but 2+ given")
				})
				Convey("Trans + DeduplicationKey is not allwoed", func() {
					reqTrans.DeduplicationKey = "beef"
					_, err := d.ScheduleTask(ctx, reqTrans)
					So(grpcutil.Code(err), ShouldEqual, codes.InvalidArgument)
					So(err, ShouldErrLike, `"KickManageProjectTask" is transactional`)
				})
			})
		})
	})
}
