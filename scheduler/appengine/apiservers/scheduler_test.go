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
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"

	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/appengine/gaetesting"
	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	"go.chromium.org/luci/scheduler/api/scheduler/v1"
	"go.chromium.org/luci/scheduler/appengine/catalog"
	"go.chromium.org/luci/scheduler/appengine/engine"
	"go.chromium.org/luci/scheduler/appengine/internal"
	"go.chromium.org/luci/scheduler/appengine/messages"
	"go.chromium.org/luci/scheduler/appengine/task"
	"go.chromium.org/luci/scheduler/appengine/task/urlfetch"
)

func TestGetJobsApi(t *testing.T) {
	t.Parallel()

	ftt.Run("Scheduler GetJobs API works", t, func(t *ftt.Test) {
		ctx := gaetesting.TestingContext()
		fakeEng, catalog := newTestEngine()
		fakeTaskBlob, err := registerURLFetcher(catalog)
		assert.Loosely(t, err, should.BeNil)
		ss := SchedulerServer{Engine: fakeEng, Catalog: catalog}

		t.Run("Empty", func(t *ftt.Test) {
			fakeEng.getVisibleJobs = func() ([]*engine.Job, error) { return []*engine.Job{}, nil }
			reply, err := ss.GetJobs(ctx, nil)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, len(reply.GetJobs()), should.BeZero)
		})

		t.Run("All Projects", func(t *ftt.Test) {
			fakeEng.getVisibleJobs = func() ([]*engine.Job, error) {
				return []*engine.Job{
					{
						JobID:             "bar/foo",
						ProjectID:         "bar",
						Enabled:           true,
						Schedule:          "0 * * * * * *",
						ActiveInvocations: []int64{1},
						Task:              fakeTaskBlob,
					},
					{
						JobID:     "baz/faz",
						ProjectID: "baz",
						Enabled:   true,
						Paused:    true,
						Schedule:  "with 1m interval",
						Task:      fakeTaskBlob,
					},
				}, nil
			}
			reply, err := ss.GetJobs(ctx, nil)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, reply.GetJobs(), should.Resemble([]*scheduler.Job{
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
			}))
		})

		t.Run("One Project", func(t *ftt.Test) {
			fakeEng.getVisibleProjectJobs = func(projectID string) ([]*engine.Job, error) {
				assert.Loosely(t, projectID, should.Equal("bar"))
				return []*engine.Job{
					{
						JobID:             "bar/foo",
						ProjectID:         "bar",
						Enabled:           true,
						Schedule:          "0 * * * * * *",
						ActiveInvocations: []int64{1},
						Task:              fakeTaskBlob,
					},
				}, nil
			}
			reply, err := ss.GetJobs(ctx, &scheduler.JobsRequest{Project: "bar"})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, reply.GetJobs(), should.Resemble([]*scheduler.Job{
				{
					JobRef:   &scheduler.JobRef{Job: "foo", Project: "bar"},
					Schedule: "0 * * * * * *",
					State:    &scheduler.JobState{UiStatus: "RUNNING"},
					Paused:   false,
				},
			}))
		})

		t.Run("Paused but currently running job", func(t *ftt.Test) {
			fakeEng.getVisibleProjectJobs = func(projectID string) ([]*engine.Job, error) {
				assert.Loosely(t, projectID, should.Equal("bar"))
				return []*engine.Job{
					{
						// Job which is paused but its latest invocation still running.
						JobID:             "bar/foo",
						ProjectID:         "bar",
						Enabled:           true,
						Schedule:          "0 * * * * * *",
						ActiveInvocations: []int64{1},
						Paused:            true,
						Task:              fakeTaskBlob,
					},
				}, nil
			}
			reply, err := ss.GetJobs(ctx, &scheduler.JobsRequest{Project: "bar"})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, reply.GetJobs(), should.Resemble([]*scheduler.Job{
				{
					JobRef:   &scheduler.JobRef{Job: "foo", Project: "bar"},
					Schedule: "0 * * * * * *",
					State:    &scheduler.JobState{UiStatus: "RUNNING"},
					Paused:   true,
				},
			}))
		})
	})
}

func TestGetInvocationsApi(t *testing.T) {
	t.Parallel()

	ftt.Run("Scheduler GetInvocations API works", t, func(t *ftt.Test) {
		ctx := gaetesting.TestingContext()
		fakeEng, catalog := newTestEngine()
		_, err := registerURLFetcher(catalog)
		assert.Loosely(t, err, should.BeNil)
		ss := SchedulerServer{Engine: fakeEng, Catalog: catalog}

		t.Run("Job not found", func(t *ftt.Test) {
			fakeEng.mockNoJob()
			_, err := ss.GetInvocations(ctx, &scheduler.InvocationsRequest{
				JobRef: &scheduler.JobRef{Project: "not", Job: "exists"},
			})
			s, ok := status.FromError(err)
			assert.Loosely(t, ok, should.BeTrue)
			assert.Loosely(t, s.Code(), should.Equal(codes.NotFound))
		})

		t.Run("DS error", func(t *ftt.Test) {
			fakeEng.mockJob("proj/job")
			fakeEng.listInvocations = func(opts engine.ListInvocationsOpts) ([]*engine.Invocation, string, error) {
				return nil, "", fmt.Errorf("ds error")
			}
			_, err := ss.GetInvocations(ctx, &scheduler.InvocationsRequest{
				JobRef: &scheduler.JobRef{Project: "proj", Job: "job"},
			})
			s, ok := status.FromError(err)
			assert.Loosely(t, ok, should.BeTrue)
			assert.Loosely(t, s.Code(), should.Equal(codes.Internal))
		})

		t.Run("Empty with huge pagesize", func(t *ftt.Test) {
			fakeEng.mockJob("proj/job")
			fakeEng.listInvocations = func(opts engine.ListInvocationsOpts) ([]*engine.Invocation, string, error) {
				assert.Loosely(t, opts, should.Resemble(engine.ListInvocationsOpts{
					PageSize: 50,
				}))
				return nil, "", nil
			}
			r, err := ss.GetInvocations(ctx, &scheduler.InvocationsRequest{
				JobRef:   &scheduler.JobRef{Project: "proj", Job: "job"},
				PageSize: 1e9,
			})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, r.GetNextCursor(), should.BeEmpty)
			assert.Loosely(t, r.GetInvocations(), should.BeEmpty)
		})

		t.Run("Some with custom pagesize and cursor", func(t *ftt.Test) {
			started := time.Unix(123123123, 0).UTC()
			finished := time.Unix(321321321, 0).UTC()
			fakeEng.mockJob("proj/job")
			fakeEng.listInvocations = func(opts engine.ListInvocationsOpts) ([]*engine.Invocation, string, error) {
				assert.Loosely(t, opts, should.Resemble(engine.ListInvocationsOpts{
					PageSize: 5,
					Cursor:   "cursor",
				}))
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
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, r.GetNextCursor(), should.Equal("next"))
			assert.Loosely(t, r.GetInvocations(), should.Resemble([]*scheduler.Invocation{
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
			}))
		})
	})
}

func TestGetInvocationApi(t *testing.T) {
	t.Parallel()

	ftt.Run("Works", t, func(t *ftt.Test) {
		ctx := gaetesting.TestingContext()
		fakeEng, catalog := newTestEngine()
		ss := SchedulerServer{Engine: fakeEng, Catalog: catalog}

		t.Run("OK", func(t *ftt.Test) {
			fakeEng.mockJob("proj/job")
			fakeEng.getInvocation = func(jobID string, invID int64) (*engine.Invocation, error) {
				assert.Loosely(t, jobID, should.Equal("proj/job"))
				assert.Loosely(t, invID, should.Equal(12))
				return &engine.Invocation{
					JobID:    jobID,
					ID:       12,
					Revision: "deadbeef",
					Status:   task.StatusRunning,
					Started:  time.Unix(123123123, 0).UTC(),
				}, nil
			}
			inv, err := ss.GetInvocation(ctx, &scheduler.InvocationRef{
				JobRef:       &scheduler.JobRef{Project: "proj", Job: "job"},
				InvocationId: 12,
			})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, inv, should.Resemble(&scheduler.Invocation{
				InvocationRef: &scheduler.InvocationRef{
					JobRef:       &scheduler.JobRef{Project: "proj", Job: "job"},
					InvocationId: 12,
				},
				ConfigRevision: "deadbeef",
				Status:         "RUNNING",
				StartedTs:      123123123000000,
			}))
		})

		t.Run("No job", func(t *ftt.Test) {
			fakeEng.mockNoJob()
			_, err := ss.GetInvocation(ctx, &scheduler.InvocationRef{
				JobRef:       &scheduler.JobRef{Project: "proj", Job: "job"},
				InvocationId: 12,
			})
			s, ok := status.FromError(err)
			assert.Loosely(t, ok, should.BeTrue)
			assert.Loosely(t, s.Code(), should.Equal(codes.NotFound))
		})

		t.Run("No invocation", func(t *ftt.Test) {
			fakeEng.mockJob("proj/job")
			fakeEng.getInvocation = func(jobID string, invID int64) (*engine.Invocation, error) {
				return nil, engine.ErrNoSuchInvocation
			}
			_, err := ss.GetInvocation(ctx, &scheduler.InvocationRef{
				JobRef:       &scheduler.JobRef{Project: "proj", Job: "job"},
				InvocationId: 12,
			})
			s, ok := status.FromError(err)
			assert.Loosely(t, ok, should.BeTrue)
			assert.Loosely(t, s.Code(), should.Equal(codes.NotFound))
		})
	})
}

func TestJobActionsApi(t *testing.T) {
	t.Parallel()

	// Note: PauseJob/ResumeJob/AbortJob are implemented identically, so test only
	// PauseJob.

	ftt.Run("works", t, func(t *ftt.Test) {
		ctx := gaetesting.TestingContext()
		fakeEng, catalog := newTestEngine()
		ss := SchedulerServer{Engine: fakeEng, Catalog: catalog}

		t.Run("PermissionDenied", func(t *ftt.Test) {
			fakeEng.mockJob("proj/job")
			fakeEng.pauseJob = func(jobID string) error {
				return engine.ErrNoPermission
			}
			_, err := ss.PauseJob(ctx, &scheduler.JobRef{Project: "proj", Job: "job"})
			s, ok := status.FromError(err)
			assert.Loosely(t, ok, should.BeTrue)
			assert.Loosely(t, s.Code(), should.Equal(codes.PermissionDenied))
		})

		t.Run("OK", func(t *ftt.Test) {
			fakeEng.mockJob("proj/job")
			fakeEng.pauseJob = func(jobID string) error {
				assert.Loosely(t, jobID, should.Equal("proj/job"))
				return nil
			}
			r, err := ss.PauseJob(ctx, &scheduler.JobRef{Project: "proj", Job: "job"})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, r, should.Resemble(&emptypb.Empty{}))
		})

		t.Run("NotFound", func(t *ftt.Test) {
			fakeEng.mockNoJob()
			_, err := ss.PauseJob(ctx, &scheduler.JobRef{Project: "proj", Job: "job"})
			s, ok := status.FromError(err)
			assert.Loosely(t, ok, should.BeTrue)
			assert.Loosely(t, s.Code(), should.Equal(codes.NotFound))
		})
	})
}

func TestAbortInvocationApi(t *testing.T) {
	t.Parallel()

	ftt.Run("works", t, func(t *ftt.Test) {
		ctx := gaetesting.TestingContext()
		fakeEng, catalog := newTestEngine()
		ss := SchedulerServer{Engine: fakeEng, Catalog: catalog}

		t.Run("PermissionDenied", func(t *ftt.Test) {
			fakeEng.mockJob("proj/job")
			fakeEng.abortInvocation = func(jobID string, invID int64) error {
				return engine.ErrNoPermission
			}
			_, err := ss.AbortInvocation(ctx, &scheduler.InvocationRef{
				JobRef:       &scheduler.JobRef{Project: "proj", Job: "job"},
				InvocationId: 12,
			})
			s, ok := status.FromError(err)
			assert.Loosely(t, ok, should.BeTrue)
			assert.Loosely(t, s.Code(), should.Equal(codes.PermissionDenied))
		})

		t.Run("OK", func(t *ftt.Test) {
			fakeEng.mockJob("proj/job")
			fakeEng.abortInvocation = func(jobID string, invID int64) error {
				assert.Loosely(t, jobID, should.Equal("proj/job"))
				assert.Loosely(t, invID, should.Equal(12))
				return nil
			}
			r, err := ss.AbortInvocation(ctx, &scheduler.InvocationRef{
				JobRef:       &scheduler.JobRef{Project: "proj", Job: "job"},
				InvocationId: 12,
			})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, r, should.Resemble(&emptypb.Empty{}))
		})

		t.Run("No job", func(t *ftt.Test) {
			fakeEng.mockNoJob()
			_, err := ss.AbortInvocation(ctx, &scheduler.InvocationRef{
				JobRef:       &scheduler.JobRef{Project: "proj", Job: "job"},
				InvocationId: 12,
			})
			s, ok := status.FromError(err)
			assert.Loosely(t, ok, should.BeTrue)
			assert.Loosely(t, s.Code(), should.Equal(codes.NotFound))
		})

		t.Run("No invocation", func(t *ftt.Test) {
			fakeEng.mockJob("proj/job")
			fakeEng.abortInvocation = func(jobID string, invID int64) error {
				return engine.ErrNoSuchInvocation
			}
			_, err := ss.AbortInvocation(ctx, &scheduler.InvocationRef{
				JobRef:       &scheduler.JobRef{Project: "proj", Job: "job"},
				InvocationId: 12,
			})
			s, ok := status.FromError(err)
			assert.Loosely(t, ok, should.BeTrue)
			assert.Loosely(t, s.Code(), should.Equal(codes.NotFound))
		})
	})
}

////

func registerURLFetcher(cat catalog.Catalog) ([]byte, error) {
	if err := cat.RegisterTaskManager(&urlfetch.TaskManager{}); err != nil {
		return nil, err
	}
	return proto.Marshal(&messages.TaskDefWrapper{
		UrlFetch: &messages.UrlFetchTask{Url: "http://example.com/path"},
	})
}

func newTestEngine() (*fakeEngine, catalog.Catalog) {
	cat := catalog.New()
	return &fakeEngine{}, cat
}

type fakeEngine struct {
	getVisibleJobs        func() ([]*engine.Job, error)
	getVisibleProjectJobs func(projectID string) ([]*engine.Job, error)
	getVisibleJob         func(jobID string) (*engine.Job, error)
	listInvocations       func(opts engine.ListInvocationsOpts) ([]*engine.Invocation, string, error)
	getInvocation         func(jobID string, invID int64) (*engine.Invocation, error)

	pauseJob        func(jobID string) error
	resumeJob       func(jobID string) error
	abortJob        func(jobID string) error
	abortInvocation func(jobID string, invID int64) error
}

func (f *fakeEngine) mockJob(jobID string) *engine.Job {
	j := &engine.Job{
		JobID:     jobID,
		ProjectID: strings.Split(jobID, "/")[0],
		Enabled:   true,
	}
	f.getVisibleJob = func(jobID string) (*engine.Job, error) {
		if jobID == j.JobID {
			return j, nil
		}
		return nil, engine.ErrNoSuchJob
	}
	return j
}

func (f *fakeEngine) mockNoJob() {
	f.getVisibleJob = func(string) (*engine.Job, error) {
		return nil, engine.ErrNoSuchJob
	}
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

func (f *fakeEngine) GetVisibleJobBatch(c context.Context, jobIDs []string) (map[string]*engine.Job, error) {
	out := map[string]*engine.Job{}
	for _, id := range jobIDs {
		switch job, err := f.GetVisibleJob(c, id); {
		case err == nil:
			out[id] = job
		case err != engine.ErrNoSuchJob:
			return nil, err
		}
	}
	return out, nil
}

func (f *fakeEngine) ListInvocations(c context.Context, job *engine.Job, opts engine.ListInvocationsOpts) ([]*engine.Invocation, string, error) {
	return f.listInvocations(opts)
}

func (f *fakeEngine) PauseJob(c context.Context, job *engine.Job, reason string) error {
	return f.pauseJob(job.JobID)
}

func (f *fakeEngine) ResumeJob(c context.Context, job *engine.Job, reason string) error {
	return f.resumeJob(job.JobID)
}

func (f *fakeEngine) AbortInvocation(c context.Context, job *engine.Job, invID int64) error {
	return f.abortInvocation(job.JobID, invID)
}

func (f *fakeEngine) AbortJob(c context.Context, job *engine.Job) error {
	return f.abortJob(job.JobID)
}

func (f *fakeEngine) EmitTriggers(c context.Context, perJob map[*engine.Job][]*internal.Trigger) error {
	return nil
}

func (f *fakeEngine) ListTriggers(c context.Context, job *engine.Job) ([]*internal.Trigger, error) {
	panic("not implemented")
}

func (f *fakeEngine) GetInvocation(c context.Context, job *engine.Job, invID int64) (*engine.Invocation, error) {
	return f.getInvocation(job.JobID, invID)
}

func (f *fakeEngine) InternalAPI() engine.EngineInternal {
	panic("not implemented")
}

func (f *fakeEngine) GetJobTriageLog(c context.Context, job *engine.Job) (*engine.JobTriageLog, error) {
	panic("not implemented")
}
