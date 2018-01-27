// Copyright 2017 The LUCI Authors.
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

package apiservers

import (
	"fmt"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"golang.org/x/net/context"

	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes/empty"
	"go.chromium.org/luci/appengine/gaetesting"
	"go.chromium.org/luci/common/auth/identity"

	"go.chromium.org/luci/scheduler/api/scheduler/v1"
	"go.chromium.org/luci/scheduler/appengine/catalog"
	"go.chromium.org/luci/scheduler/appengine/engine"
	"go.chromium.org/luci/scheduler/appengine/internal"
	"go.chromium.org/luci/scheduler/appengine/messages"
	"go.chromium.org/luci/scheduler/appengine/task"
	"go.chromium.org/luci/scheduler/appengine/task/urlfetch"

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
			fakeEng.getVisibleJobs = func() ([]*engine.Job, error) { return []*engine.Job{}, nil }
			reply, err := ss.GetJobs(ctx, nil)
			So(err, ShouldBeNil)
			So(len(reply.GetJobs()), ShouldEqual, 0)
		})

		Convey("All Projects", func() {
			fakeEng.getVisibleJobs = func() ([]*engine.Job, error) {
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
					JobRef:   &scheduler.JobRef{Job: "foo", Project: "bar"},
					Schedule: "0 * * * * * *",
					State:    &scheduler.JobState{UiStatus: "RUNNING"},
					Paused:   false,
				},
				{
					JobRef:   &scheduler.JobRef{Job: "faz", Project: "baz"},
					Schedule: "with 1m interval",
					State:    &scheduler.JobState{UiStatus: "PAUSED"},
					Paused:   true,
				},
			})
		})

		Convey("One Project", func() {
			fakeEng.getVisibleProjectJobs = func(projectID string) ([]*engine.Job, error) {
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
					JobRef:   &scheduler.JobRef{Job: "foo", Project: "bar"},
					Schedule: "0 * * * * * *",
					State:    &scheduler.JobState{UiStatus: "RUNNING"},
					Paused:   false,
				},
			})
		})

		Convey("Paused but currently running job", func() {
			fakeEng.getVisibleProjectJobs = func(projectID string) ([]*engine.Job, error) {
				So(projectID, ShouldEqual, "bar")
				return []*engine.Job{
					{
						// Job which is paused but its latest invocation still running.
						JobID:     "bar/foo",
						ProjectID: "bar",
						Schedule:  "0 * * * * * *",
						State:     engine.JobState{State: engine.JobStateRunning},
						Paused:    true,
						Task:      fakeTaskBlob,
					},
				}, nil
			}
			reply, err := ss.GetJobs(ctx, &scheduler.JobsRequest{Project: "bar"})
			So(err, ShouldBeNil)
			So(reply.GetJobs(), ShouldResemble, []*scheduler.Job{
				{
					JobRef:   &scheduler.JobRef{Job: "foo", Project: "bar"},
					Schedule: "0 * * * * * *",
					State:    &scheduler.JobState{UiStatus: "RUNNING"},
					Paused:   true,
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
			fakeEng.listVisibleInvocations = func(int, string) ([]*engine.Invocation, string, error) {
				return nil, "", engine.ErrNoSuchJob
			}
			_, err := ss.GetInvocations(ctx, &scheduler.InvocationsRequest{
				JobRef: &scheduler.JobRef{Project: "not", Job: "exists"},
			})
			s, ok := status.FromError(err)
			So(ok, ShouldBeTrue)
			So(s.Code(), ShouldEqual, codes.NotFound)
		})

		Convey("DS error", func() {
			fakeEng.listVisibleInvocations = func(int, string) ([]*engine.Invocation, string, error) {
				return nil, "", fmt.Errorf("ds error")
			}
			_, err := ss.GetInvocations(ctx, &scheduler.InvocationsRequest{
				JobRef: &scheduler.JobRef{Project: "proj", Job: "job"},
			})
			s, ok := status.FromError(err)
			So(ok, ShouldBeTrue)
			So(s.Code(), ShouldEqual, codes.Internal)
		})

		fakeEng.getVisibleJob = func(JobID string) (*engine.Job, error) {
			return &engine.Job{JobID: "proj/job", ProjectID: "proj"}, nil
		}

		Convey("Emtpy with huge pagesize", func() {
			fakeEng.listVisibleInvocations = func(pageSize int, cursor string) ([]*engine.Invocation, string, error) {
				So(pageSize, ShouldEqual, 50)
				So(cursor, ShouldEqual, "")
				return nil, "", nil
			}
			r, err := ss.GetInvocations(ctx, &scheduler.InvocationsRequest{
				JobRef:   &scheduler.JobRef{Project: "proj", Job: "job"},
				PageSize: 1e9,
			})
			So(err, ShouldBeNil)
			So(r.GetNextCursor(), ShouldEqual, "")
			So(r.GetInvocations(), ShouldBeEmpty)
		})

		Convey("Some with custom pagesize and cursor", func() {
			started := time.Unix(123123123, 0).UTC()
			finished := time.Unix(321321321, 0).UTC()
			fakeEng.listVisibleInvocations = func(pageSize int, cursor string) ([]*engine.Invocation, string, error) {
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
				JobRef:   &scheduler.JobRef{Project: "proj", Job: "job"},
				PageSize: 5,
				Cursor:   "cursor",
			})
			So(err, ShouldBeNil)
			So(r.GetNextCursor(), ShouldEqual, "next")
			So(r.GetInvocations(), ShouldResemble, []*scheduler.Invocation{
				{
					InvocationRef: &scheduler.InvocationRef{
						JobRef:       &scheduler.JobRef{Project: "proj", Job: "job"},
						InvocationId: 12,
					},
					ConfigRevision: "deadbeef",
					Final:          false,
					Status:         "RUNNING",
					StartedTs:      started.UnixNano() / 1000,
					TriggeredBy:    "user:bot@example.com",
				},
				{
					InvocationRef: &scheduler.InvocationRef{
						JobRef:       &scheduler.JobRef{Project: "proj", Job: "job"},
						InvocationId: 13,
					},
					ConfigRevision: "deadbeef",
					Final:          true,
					Status:         "ABORTED",
					StartedTs:      started.UnixNano() / 1000,
					FinishedTs:     finished.UnixNano() / 1000,
					ViewUrl:        "https://example.com/13",
				},
			})
		})
	})
}

func TestJobActionsApi(t *testing.T) {
	t.Parallel()

	Convey("works", t, func() {
		ctx := gaetesting.TestingContext()
		fakeEng, catalog := newTestEngine()
		ss := SchedulerServer{fakeEng, catalog}

		Convey("PermissionDenied", func() {
			onAction := func(jobID string) error {
				return engine.ErrNoPermission
			}
			Convey("Pause", func() {
				fakeEng.pauseJob = onAction
				_, err := ss.PauseJob(ctx, &scheduler.JobRef{Project: "proj", Job: "job"})
				s, ok := status.FromError(err)
				So(ok, ShouldBeTrue)
				So(s.Code(), ShouldEqual, codes.PermissionDenied)
			})

			Convey("Abort", func() {
				fakeEng.abortJob = onAction
				_, err := ss.AbortJob(ctx, &scheduler.JobRef{Project: "proj", Job: "job"})
				s, ok := status.FromError(err)
				So(ok, ShouldBeTrue)
				So(s.Code(), ShouldEqual, codes.PermissionDenied)
			})
		})

		Convey("OK", func() {
			onAction := func(jobID string) error {
				So(jobID, ShouldEqual, "proj/job")
				return nil
			}

			Convey("Pause", func() {
				fakeEng.pauseJob = onAction
				r, err := ss.PauseJob(ctx, &scheduler.JobRef{Project: "proj", Job: "job"})
				So(err, ShouldBeNil)
				So(r, ShouldResemble, &empty.Empty{})
			})

			Convey("Resume", func() {
				fakeEng.resumeJob = onAction
				r, err := ss.ResumeJob(ctx, &scheduler.JobRef{Project: "proj", Job: "job"})
				So(err, ShouldBeNil)
				So(r, ShouldResemble, &empty.Empty{})
			})

			Convey("Abort", func() {
				fakeEng.abortJob = onAction
				r, err := ss.AbortJob(ctx, &scheduler.JobRef{Project: "proj", Job: "job"})
				So(err, ShouldBeNil)
				So(r, ShouldResemble, &empty.Empty{})
			})
		})

		Convey("NotFound", func() {
			fakeEng.pauseJob = func(jobID string) error {
				return engine.ErrNoSuchJob
			}
			_, err := ss.PauseJob(ctx, &scheduler.JobRef{Project: "proj", Job: "job"})
			s, ok := status.FromError(err)
			So(ok, ShouldBeTrue)
			So(s.Code(), ShouldEqual, codes.NotFound)
		})
	})
}

func TestAbortInvocationApi(t *testing.T) {
	t.Parallel()

	Convey("works", t, func() {
		ctx := gaetesting.TestingContext()
		fakeEng, catalog := newTestEngine()
		ss := SchedulerServer{fakeEng, catalog}

		Convey("PermissionDenied", func() {
			fakeEng.abortInvocation = func(jobID string, invID int64) error {
				return engine.ErrNoPermission
			}
			_, err := ss.AbortInvocation(ctx, &scheduler.InvocationRef{
				JobRef:       &scheduler.JobRef{Project: "proj", Job: "job"},
				InvocationId: 12,
			})
			s, ok := status.FromError(err)
			So(ok, ShouldBeTrue)
			So(s.Code(), ShouldEqual, codes.PermissionDenied)
		})

		Convey("OK", func() {
			fakeEng.abortInvocation = func(jobID string, invID int64) error {
				So(jobID, ShouldEqual, "proj/job")
				So(invID, ShouldEqual, 12)
				return nil
			}
			r, err := ss.AbortInvocation(ctx, &scheduler.InvocationRef{
				JobRef:       &scheduler.JobRef{Project: "proj", Job: "job"},
				InvocationId: 12,
			})
			So(err, ShouldBeNil)
			So(r, ShouldResemble, &empty.Empty{})
		})

		Convey("Error", func() {
			fakeEng.abortInvocation = func(jobID string, invID int64) error {
				return engine.ErrNoSuchInvocation
			}
			_, err := ss.AbortInvocation(ctx, &scheduler.InvocationRef{
				JobRef:       &scheduler.JobRef{Project: "proj", Job: "job"},
				InvocationId: 12,
			})
			s, ok := status.FromError(err)
			So(ok, ShouldBeTrue)
			So(s.Code(), ShouldEqual, codes.NotFound)
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
	getVisibleJobs         func() ([]*engine.Job, error)
	getVisibleProjectJobs  func(projectID string) ([]*engine.Job, error)
	getVisibleJob          func(jobID string) (*engine.Job, error)
	listVisibleInvocations func(pageSize int, cursor string) ([]*engine.Invocation, string, error)

	pauseJob        func(jobID string) error
	resumeJob       func(jobID string) error
	abortJob        func(jobID string) error
	abortInvocation func(jobID string, invID int64) error
}

func (f *fakeEngine) GetVisibleJobs(c context.Context) ([]*engine.Job, error) {
	return f.getVisibleJobs()
}

func (f *fakeEngine) GetVisibleProjectJobs(c context.Context, projectID string) ([]*engine.Job, error) {
	return f.getVisibleProjectJobs(projectID)
}

func (f *fakeEngine) GetVisibleJob(c context.Context, jobID string) (*engine.Job, error) {
	return f.getVisibleJob(jobID)
}

func (f *fakeEngine) ListVisibleInvocations(c context.Context, jobID string, pageSize int, cursor string) ([]*engine.Invocation, string, error) {
	return f.listVisibleInvocations(pageSize, cursor)
}

func (f *fakeEngine) PauseJob(c context.Context, jobID string) error {
	return f.pauseJob(jobID)
}

func (f *fakeEngine) ResumeJob(c context.Context, jobID string) error {
	return f.resumeJob(jobID)
}

func (f *fakeEngine) AbortInvocation(c context.Context, jobID string, invID int64) error {
	return f.abortInvocation(jobID, invID)
}

func (f *fakeEngine) AbortJob(c context.Context, jobID string) error {
	return f.abortJob(jobID)
}

func (f *fakeEngine) EmitTriggers(c context.Context, perJob map[string][]*internal.Trigger) error {
	return nil
}

func (f *fakeEngine) GetVisibleInvocation(c context.Context, jobID string, invID int64) (*engine.Invocation, error) {
	panic("not implemented")
}

func (f *fakeEngine) ForceInvocation(c context.Context, jobID string) (engine.FutureInvocation, error) {
	panic("not implemented")
}

func (f *fakeEngine) InternalAPI() engine.EngineInternal {
	panic("not implemented")
}
