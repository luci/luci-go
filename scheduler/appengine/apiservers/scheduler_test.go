// Copyright 2017 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package apiservers

import (
	"testing"

	"golang.org/x/net/context"

	"github.com/golang/protobuf/proto"
	"github.com/luci/luci-go/appengine/gaetesting"
	"github.com/luci/luci-go/server/auth/identity"

	scheduler "github.com/luci/luci-go/scheduler/api/scheduler/v1"
	"github.com/luci/luci-go/scheduler/appengine/catalog"
	"github.com/luci/luci-go/scheduler/appengine/engine"
	"github.com/luci/luci-go/scheduler/appengine/messages"
	"github.com/luci/luci-go/scheduler/appengine/task/urlfetch"

	. "github.com/smartystreets/goconvey/convey"
)

func TestGetJobsApi(t *testing.T) {
	t.Parallel()

	Convey("Scheduler GetJobs API works", t, func() {
		ctx := gaetesting.TestingContext()
		fakeEng, catalog := newTestEngine()
		So(catalog.RegisterTaskManager(&urlfetch.TaskManager{}), ShouldBeNil)

		ss := SchedulerServer{fakeEng, catalog}
		taskBlob, err := proto.Marshal(&messages.TaskDefWrapper{
			UrlFetch: &messages.UrlFetchTask{Url: "http://example.com/path"},
		})
		So(err, ShouldBeNil)

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
						Task:      taskBlob,
					},
					{
						JobID:     "baz/faz",
						Paused:    true,
						ProjectID: "baz",
						Schedule:  "with 1m interval",
						State:     engine.JobState{State: engine.JobStateSuspended},
						Task:      taskBlob,
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
						Task:      taskBlob,
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

////

func newTestEngine() (*fakeEngine, catalog.Catalog) {
	cat := catalog.New("scheduler.cfg")
	return &fakeEngine{}, cat
}

type fakeEngine struct {
	getAllJobs     func() ([]*engine.Job, error)
	getProjectJobs func(projectID string) ([]*engine.Job, error)
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
	panic("not implemented")
}

func (f *fakeEngine) ListInvocations(c context.Context, jobID string, pageSize int, cursor string) ([]*engine.Invocation, string, error) {
	panic("not implemented")
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
