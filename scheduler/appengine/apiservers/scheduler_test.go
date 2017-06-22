// Copyright 2017 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package apiservers

import (
	"fmt"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"golang.org/x/net/context"

	"github.com/golang/protobuf/proto"
	"github.com/luci/luci-go/appengine/gaetesting"
	"github.com/luci/luci-go/server/auth/identity"

	scheduler "github.com/luci/luci-go/scheduler/api/scheduler/v1"
	"github.com/luci/luci-go/scheduler/appengine/catalog"
	"github.com/luci/luci-go/scheduler/appengine/engine"
	"github.com/luci/luci-go/scheduler/appengine/messages"
	"github.com/luci/luci-go/scheduler/appengine/task"
	"github.com/luci/luci-go/scheduler/appengine/task/urlfetch"

	. "github.com/smartystreets/goconvey/convey"
)

func TestGetJobsApi(t *testing.T) {
	t.Parallel()

	Convey("Scheduler GetJobs API works", t, func() {
		ctx := gaetesting.TestingContext()
		fakeEng, catalog := newTestEngine()
		fakeTaskBlob, err := registerUrlFetcher(catalog)
		So(err, ShouldBeNil)
		ss := SchedulerServer{fakeEng, catalog}

		Convey("Empty", func() {
			fakeEng.getAllJobs = func() ([]*engine.Job, error) { return []*engine.Job{}, nil }
			reply, err := ss.GetJobs(ctx, nil)
			So(err, ShouldBeNil)
			So(len(reply.GetJobs()), ShouldEqual, 0)
		})

		Convey("All Projects", func() {
			fakeEng.getAllJobs = func() ([]*engine.Job, error) {
				return []*engine.Job{
					{
						JobID:     "bar/foo",
						ProjectID: "bar",
						Schedule:  "0 * * * * * *",
						State:     engine.JobState{State: engine.JobStateRunning},
						Task:      fakeTaskBlob,
					},
					{
						JobID:     "baz/faz",
						Paused:    true,
						ProjectID: "baz",
						Schedule:  "with 1m interval",
						State:     engine.JobState{State: engine.JobStateSuspended},
						Task:      fakeTaskBlob,
					},
				}, nil
			}
			reply, err := ss.GetJobs(ctx, nil)
			So(err, ShouldBeNil)
			So(reply.GetJobs(), ShouldResemble, []*scheduler.Job{
				{
					Name:     "foo",
					Project:  "bar",
					Schedule: "0 * * * * * *",
					State:    &scheduler.JobState{UiStatus: "RUNNING"},
				},
				{
					Name:     "faz",
					Project:  "baz",
					Schedule: "with 1m interval",
					State:    &scheduler.JobState{UiStatus: "PAUSED"},
				},
			})
		})

		Convey("One Project", func() {
			fakeEng.getProjectJobs = func(projectID string) ([]*engine.Job, error) {
				So(projectID, ShouldEqual, "bar")
				return []*engine.Job{
					{
						JobID:     "bar/foo",
						ProjectID: "bar",
						Schedule:  "0 * * * * * *",
						State:     engine.JobState{State: engine.JobStateRunning},
						Task:      fakeTaskBlob,
					},
				}, nil
			}
			reply, err := ss.GetJobs(ctx, &scheduler.JobsRequest{Project: "bar"})
			So(err, ShouldBeNil)
			So(reply.GetJobs(), ShouldResemble, []*scheduler.Job{
				{
					Name:     "foo",
					Project:  "bar",
					Schedule: "0 * * * * * *",
					State:    &scheduler.JobState{UiStatus: "RUNNING"},
				},
			})
		})
	})
}

func TestGetInvocationsApi(t *testing.T) {
	t.Parallel()

	Convey("Scheduler GetInvocations API works", t, func() {
		ctx := gaetesting.TestingContext()
		fakeEng, catalog := newTestEngine()
		_, err := registerUrlFetcher(catalog)
		So(err, ShouldBeNil)
		ss := SchedulerServer{fakeEng, catalog}

		Convey("Job not found", func() {
			fakeEng.getJob = func(JobID string) (*engine.Job, error) { return nil, nil }
			_, err := ss.GetInvocations(ctx, &scheduler.InvocationsRequest{Project: "not", Job: "exists"})
			s, ok := status.FromError(err)
			So(ok, ShouldBeTrue)
			So(s.Code(), ShouldEqual, codes.NotFound)
		})

		Convey("DS error", func() {
			fakeEng.getJob = func(JobID string) (*engine.Job, error) { return nil, fmt.Errorf("ds error") }
			_, err := ss.GetInvocations(ctx, &scheduler.InvocationsRequest{Project: "proj", Job: "job"})
			s, ok := status.FromError(err)
			So(ok, ShouldBeTrue)
			So(s.Code(), ShouldEqual, codes.Internal)
		})

		fakeEng.getJob = func(JobID string) (*engine.Job, error) {
			return &engine.Job{JobID: "proj/job", ProjectID: "proj"}, nil
		}

		Convey("Emtpy with huge pagesize", func() {
			fakeEng.listInvocations = func(pageSize int, cursor string) ([]*engine.Invocation, string, error) {
				So(pageSize, ShouldEqual, 50)
				So(cursor, ShouldEqual, "")
				return nil, "", nil
			}
			r, err := ss.GetInvocations(ctx, &scheduler.InvocationsRequest{Project: "proj", Job: "job", PageSize: 1e9})
			So(err, ShouldBeNil)
			So(r.GetNextCursor(), ShouldEqual, "")
			So(r.GetInvocations(), ShouldBeEmpty)
		})

		Convey("Some with custom pagesize and cursor", func() {
			started := time.Unix(123123123, 0).UTC()
			finished := time.Unix(321321321, 0).UTC()
			fakeEng.listInvocations = func(pageSize int, cursor string) ([]*engine.Invocation, string, error) {
				So(pageSize, ShouldEqual, 5)
				So(cursor, ShouldEqual, "cursor")
				return []*engine.Invocation{
					{ID: 12, Revision: "deadbeef", Status: task.StatusRunning, Started: started,
						TriggeredBy: identity.Identity("user:bot@example.com")},
					{ID: 13, Revision: "deadbeef", Status: task.StatusAborted, Started: started, Finished: finished,
						ViewURL: "https://example.com/13"},
				}, "next", nil
			}
			r, err := ss.GetInvocations(ctx, &scheduler.InvocationsRequest{
				Project: "proj", Job: "job", PageSize: 5, Cursor: "cursor"})
			So(err, ShouldBeNil)
			So(r.GetNextCursor(), ShouldEqual, "next")
			So(r.GetInvocations(), ShouldResemble, []*scheduler.Invocation{
				{
					Project: "proj", Job: "job", ConfigRevision: "deadbeef",
					Id: 12, Final: false, Status: "RUNNING",
					StartedTs:   started.UnixNano() / 1000,
					TriggeredBy: "user:bot@example.com",
				},
				{
					Project: "proj", Job: "job", ConfigRevision: "deadbeef",
					Id: 13, Final: true, Status: "ABORTED",
					StartedTs: started.UnixNano() / 1000, FinishedTs: finished.UnixNano() / 1000,
					ViewUrl: "https://example.com/13",
				},
			})
		})

	})
}

////

func registerUrlFetcher(cat catalog.Catalog) ([]byte, error) {
	if err := cat.RegisterTaskManager(&urlfetch.TaskManager{}); err != nil {
		return nil, err
	}
	return proto.Marshal(&messages.TaskDefWrapper{
		UrlFetch: &messages.UrlFetchTask{Url: "http://example.com/path"},
	})
}

func newTestEngine() (*fakeEngine, catalog.Catalog) {
	cat := catalog.New("scheduler.cfg")
	return &fakeEngine{}, cat
}

type fakeEngine struct {
	getAllJobs      func() ([]*engine.Job, error)
	getProjectJobs  func(projectID string) ([]*engine.Job, error)
	getJob          func(jobID string) (*engine.Job, error)
	listInvocations func(pageSize int, cursor string) ([]*engine.Invocation, string, error)
}

func (f *fakeEngine) GetAllProjects(c context.Context) ([]string, error) {
	panic("not implemented")
}

func (f *fakeEngine) GetAllJobs(c context.Context) ([]*engine.Job, error) {
	return f.getAllJobs()
}

func (f *fakeEngine) GetProjectJobs(c context.Context, projectID string) ([]*engine.Job, error) {
	return f.getProjectJobs(projectID)
}

func (f *fakeEngine) GetJob(c context.Context, jobID string) (*engine.Job, error) {
	return f.getJob(jobID)
}

func (f *fakeEngine) ListInvocations(c context.Context, jobID string, pageSize int, cursor string) ([]*engine.Invocation, string, error) {
	return f.listInvocations(pageSize, cursor)
}

func (f *fakeEngine) GetInvocation(c context.Context, jobID string, invID int64) (*engine.Invocation, error) {
	panic("not implemented")
}

func (f *fakeEngine) GetInvocationsByNonce(c context.Context, invNonce int64) ([]*engine.Invocation, error) {
	panic("not implemented")
}

func (f *fakeEngine) UpdateProjectJobs(c context.Context, projectID string, defs []catalog.Definition) error {
	panic("not implemented")
}

func (f *fakeEngine) ResetAllJobsOnDevServer(c context.Context) error {
	panic("not implemented")
}

func (f *fakeEngine) ExecuteSerializedAction(c context.Context, body []byte, retryCount int) error {
	panic("not implemented")
}

func (f *fakeEngine) ProcessPubSubPush(c context.Context, body []byte) error {
	panic("not implemented")
}

func (f *fakeEngine) PullPubSubOnDevServer(c context.Context, taskManagerName string, publisher string) error {
	panic("not implemented")
}

func (f *fakeEngine) TriggerInvocation(c context.Context, jobID string, triggeredBy identity.Identity) (int64, error) {
	panic("not implemented")
}

func (f *fakeEngine) PauseJob(c context.Context, jobID string, who identity.Identity) error {
	panic("not implemented")
}

func (f *fakeEngine) ResumeJob(c context.Context, jobID string, who identity.Identity) error {
	panic("not implemented")
}

func (f *fakeEngine) AbortInvocation(c context.Context, jobID string, invID int64, who identity.Identity) error {
	panic("not implemented")
}

func (f *fakeEngine) AbortJob(c context.Context, jobID string, who identity.Identity) error {
	panic("not implemented")
}
