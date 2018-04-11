// Copyright 2016 The LUCI Authors.
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
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/golang/protobuf/ptypes/empty"
	"golang.org/x/net/context"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/proto/google"
	"go.chromium.org/luci/server/auth"

	"go.chromium.org/luci/scheduler/api/scheduler/v1"
	"go.chromium.org/luci/scheduler/appengine/catalog"
	"go.chromium.org/luci/scheduler/appengine/engine"
	"go.chromium.org/luci/scheduler/appengine/internal"
	"go.chromium.org/luci/scheduler/appengine/presentation"
)

// SchedulerServer implements scheduler.Scheduler API.
type SchedulerServer struct {
	Engine  engine.Engine
	Catalog catalog.Catalog
}

var _ scheduler.SchedulerServer = (*SchedulerServer)(nil)

// GetJobs fetches all jobs satisfying JobsRequest and visibility ACLs.
func (s *SchedulerServer) GetJobs(ctx context.Context, in *scheduler.JobsRequest) (*scheduler.JobsReply, error) {
	if in.GetCursor() != "" {
		// Paging in GetJobs isn't implemented until we have enough jobs to care.
		// Until then, not empty cursor implies no more jobs to return.
		return &scheduler.JobsReply{Jobs: []*scheduler.Job{}, NextCursor: ""}, nil
	}
	var ejobs []*engine.Job
	var err error
	if in.GetProject() == "" {
		ejobs, err = s.Engine.GetVisibleJobs(ctx)
	} else {
		ejobs, err = s.Engine.GetVisibleProjectJobs(ctx, in.GetProject())
	}
	if err != nil {
		return nil, status.Errorf(codes.Internal, "internal error: %s", err)
	}

	jobs := make([]*scheduler.Job, len(ejobs))
	for i, ej := range ejobs {
		traits, err := presentation.GetJobTraits(ctx, s.Catalog, ej)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "failed to get traits: %s", err)
		}
		jobs[i] = &scheduler.Job{
			JobRef: &scheduler.JobRef{
				Project: ej.ProjectID,
				Job:     ej.JobName(),
			},
			Schedule: ej.Schedule,
			State: &scheduler.JobState{
				UiStatus: string(presentation.GetPublicStateKind(ej, traits)),
			},
			Paused: ej.Paused,
		}
	}
	return &scheduler.JobsReply{Jobs: jobs, NextCursor: ""}, nil
}

func (s *SchedulerServer) GetInvocations(ctx context.Context, in *scheduler.InvocationsRequest) (*scheduler.InvocationsReply, error) {
	job, err := s.getJob(ctx, in.GetJobRef())
	if err != nil {
		return nil, err
	}

	pageSize := 50
	if in.PageSize > 0 && int(in.PageSize) < pageSize {
		pageSize = int(in.PageSize)
	}

	einvs, cursor, err := s.Engine.ListInvocations(ctx, job, engine.ListInvocationsOpts{
		PageSize: pageSize,
		Cursor:   in.Cursor,
	})
	if err != nil {
		return nil, status.Errorf(codes.Internal, "internal error: %s", err)
	}

	invs := make([]*scheduler.Invocation, len(einvs))
	for i, einv := range einvs {
		invs[i] = &scheduler.Invocation{
			InvocationRef: &scheduler.InvocationRef{
				JobRef: &scheduler.JobRef{
					Project: in.GetJobRef().GetProject(),
					Job:     in.GetJobRef().GetJob(),
				},
				InvocationId: einv.ID,
			},
			StartedTs:      einv.Started.UnixNano() / 1000,
			TriggeredBy:    string(einv.TriggeredBy),
			Status:         string(einv.Status),
			Final:          einv.Status.Final(),
			ConfigRevision: einv.Revision,
			ViewUrl:        einv.ViewURL,
		}
		if einv.Status.Final() {
			invs[i].FinishedTs = einv.Finished.UnixNano() / 1000
		}
	}
	return &scheduler.InvocationsReply{Invocations: invs, NextCursor: cursor}, nil
}

//// Actions.

func (s *SchedulerServer) PauseJob(ctx context.Context, in *scheduler.JobRef) (*empty.Empty, error) {
	return s.runAction(ctx, in, func(job *engine.Job) error {
		return s.Engine.PauseJob(ctx, job)
	})
}

func (s *SchedulerServer) ResumeJob(ctx context.Context, in *scheduler.JobRef) (*empty.Empty, error) {
	return s.runAction(ctx, in, func(job *engine.Job) error {
		return s.Engine.ResumeJob(ctx, job)
	})
}

func (s *SchedulerServer) AbortJob(ctx context.Context, in *scheduler.JobRef) (*empty.Empty, error) {
	return s.runAction(ctx, in, func(job *engine.Job) error {
		return s.Engine.AbortJob(ctx, job)
	})
}

func (s *SchedulerServer) AbortInvocation(ctx context.Context, in *scheduler.InvocationRef) (*empty.Empty, error) {
	return s.runAction(ctx, in.GetJobRef(), func(job *engine.Job) error {
		return s.Engine.AbortInvocation(ctx, job, in.GetInvocationId())
	})
}

func (s *SchedulerServer) EmitTriggers(ctx context.Context, in *scheduler.EmitTriggersRequest) (*empty.Empty, error) {
	caller := auth.CurrentIdentity(ctx)

	// Optionally use client-provided time if it is within reasonable margins.
	// This is needed to make EmitTriggers idempotent (when it emits a batch).
	now := clock.Now(ctx)
	if in.Timestamp != 0 {
		if in.Timestamp < 0 || in.Timestamp > (1<<53) {
			return nil, status.Errorf(codes.InvalidArgument,
				"the provided timestamp doesn't look like a valid number of microseconds since epoch")
		}
		ts := time.Unix(0, in.Timestamp*1000)
		if ts.After(now.Add(15 * time.Minute)) {
			return nil, status.Errorf(codes.InvalidArgument,
				"the provided timestamp (%s) is more than 15 min in the future based on the server clock value %s",
				ts, now)
		}
		if ts.Before(now.Add(-15 * time.Minute)) {
			return nil, status.Errorf(codes.InvalidArgument,
				"the provided timestamp (%s) is more than 15 min in the past based on the server clock value %s",
				ts, now)
		}
		now = ts
	}

	// Build a mapping "jobID => list of triggers", convert public representation
	// of a trigger into internal one, validating them.
	triggersPerJobID := map[string][]*internal.Trigger{}
	for index, batch := range in.Batches {
		tr, err := internalTrigger(batch.Trigger, now, caller, index)
		if err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "bad trigger #%d (%q) - %s", index, batch.Trigger.Id, err)
		}
		for _, jobRef := range batch.Jobs {
			jobId := jobRef.GetProject() + "/" + jobRef.GetJob()
			triggersPerJobID[jobId] = append(triggersPerJobID[jobId], tr)
		}
	}

	// Check jobs presence and READER ACLs.
	jobIDs := make([]string, 0, len(triggersPerJobID))
	for id := range triggersPerJobID {
		jobIDs = append(jobIDs, id)
	}
	visible, err := s.Engine.GetVisibleJobBatch(ctx, jobIDs)
	switch {
	case err != nil:
		return nil, status.Errorf(codes.Internal, "internal error: %s", err)
	case len(visible) != len(jobIDs):
		missing := make([]string, 0, len(jobIDs)-len(visible))
		for _, j := range jobIDs {
			if visible[j] == nil {
				missing = append(missing, j)
			}
		}
		return nil, status.Errorf(codes.NotFound,
			"no such job or no READ permission: %s", strings.Join(missing, ", "))
	}

	// Submit the request to the Engine.
	triggersPerJob := make(map[*engine.Job][]*internal.Trigger, len(visible))
	for id, job := range visible {
		triggersPerJob[job] = triggersPerJobID[id]
	}
	switch err = s.Engine.EmitTriggers(ctx, triggersPerJob); {
	case err == engine.ErrNoPermission:
		return nil, status.Errorf(codes.PermissionDenied, "no permission to execute the action")
	case err != nil:
		return nil, status.Errorf(codes.Internal, "internal error: %s", err)
	}

	return &empty.Empty{}, nil
}

//// Private helpers.

// getJob fetches a job, checking READER ACL.
//
// Returns grpc errors that can be returned as is.
func (s *SchedulerServer) getJob(ctx context.Context, ref *scheduler.JobRef) (*engine.Job, error) {
	jobID := ref.GetProject() + "/" + ref.GetJob()
	switch job, err := s.Engine.GetVisibleJob(ctx, jobID); {
	case err == nil:
		return job, nil
	case err == engine.ErrNoSuchJob:
		return nil, status.Errorf(codes.NotFound, "no such job or no READ permission")
	default:
		return nil, status.Errorf(codes.Internal, "internal error when fetching job: %s", err)
	}
}

func (s *SchedulerServer) runAction(ctx context.Context, ref *scheduler.JobRef, action func(*engine.Job) error) (*empty.Empty, error) {
	job, err := s.getJob(ctx, ref)
	if err != nil {
		return nil, err
	}
	switch err := action(job); {
	case err == nil:
		return &empty.Empty{}, nil
	case err == engine.ErrNoSuchJob:
		return nil, status.Errorf(codes.NotFound, "no such job or no READ permission")
	case err == engine.ErrNoPermission:
		return nil, status.Errorf(codes.PermissionDenied, "no permission to execute the action")
	case err == engine.ErrNoSuchInvocation:
		return nil, status.Errorf(codes.NotFound, "no such invocation")
	default:
		return nil, status.Errorf(codes.Internal, "internal error: %s", err)
	}
}

func internalTrigger(t *scheduler.Trigger, now time.Time, who identity.Identity, index int) (*internal.Trigger, error) {
	if t.Id == "" {
		return nil, fmt.Errorf("trigger id is required")
	}
	out := &internal.Trigger{
		Id:            t.Id,
		Created:       google.NewTimestamp(now),
		OrderInBatch:  int64(index),
		Title:         t.Title,
		Url:           t.Url,
		EmittedByUser: string(who),
	}
	if t.Payload != nil {
		// Ugh...
		switch v := t.Payload.(type) {
		case *scheduler.Trigger_Cron:
			return nil, errors.New("emitting cron triggers through API is not allowed")
		case *scheduler.Trigger_Noop:
			out.Payload = &internal.Trigger_Noop{Noop: v.Noop}
		case *scheduler.Trigger_Gitiles:
			out.Payload = &internal.Trigger_Gitiles{Gitiles: v.Gitiles}
		case *scheduler.Trigger_Buildbucket:
			out.Payload = &internal.Trigger_Buildbucket{Buildbucket: v.Buildbucket}
		default:
			return nil, errors.New("unrecognized trigger payload")
		}
	}
	return out, nil
}
