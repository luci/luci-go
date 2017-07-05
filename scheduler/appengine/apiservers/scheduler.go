// Copyright 2016 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package apiservers

import (
	"github.com/luci/luci-go/scheduler/api/scheduler/v1"
	"github.com/luci/luci-go/scheduler/appengine/catalog"
	"github.com/luci/luci-go/scheduler/appengine/engine"
	"github.com/luci/luci-go/scheduler/appengine/presentation"
	"github.com/luci/luci-go/server/auth"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"

	"github.com/golang/protobuf/ptypes/empty"
)

// SchedulerServer implements scheduler.Scheduler API.
type SchedulerServer struct {
	Engine  engine.Engine
	Catalog catalog.Catalog
}

var _ scheduler.SchedulerServer = (*SchedulerServer)(nil)

// GetJobs fetches all jobs satisfying JobsRequest and visibility ACLs.
func (s SchedulerServer) GetJobs(ctx context.Context, in *scheduler.JobsRequest) (*scheduler.JobsReply, error) {
	if in.GetCursor() != "" {
		// Paging in GetJobs isn't implemented until we have enough jobs to care.
		// Until then, not empty cursor implies no more jobs to return.
		return &scheduler.JobsReply{Jobs: []*scheduler.Job{}, NextCursor: ""}, nil
	}
	var ejobs []*engine.Job
	var err error
	if in.GetProject() == "" {
		ejobs, err = s.Engine.GetAllJobs(ctx)
	} else {
		ejobs, err = s.Engine.GetProjectJobs(ctx, in.GetProject())
	}
	if err != nil {
		return nil, grpc.Errorf(codes.Internal, "datastore error: %s", err)
	}

	jobs := make([]*scheduler.Job, len(ejobs))
	for i, ej := range ejobs {
		traits, err := presentation.GetJobTraits(ctx, s.Catalog, ej)
		if err != nil {
			return nil, grpc.Errorf(codes.Internal, "failed to get traits: %s", err)
		}
		jobs[i] = &scheduler.Job{
			JobRef: &scheduler.JobRef{
				Project: ej.ProjectID,
				Job:     ej.GetJobName(),
			},
			Schedule: ej.Schedule,
			State: &scheduler.JobState{
				UiStatus: string(presentation.GetPublicStateKind(ej, traits)),
			},
		}
	}
	return &scheduler.JobsReply{Jobs: jobs, NextCursor: ""}, nil
}

func (s SchedulerServer) GetInvocations(ctx context.Context, in *scheduler.InvocationsRequest) (*scheduler.InvocationsReply, error) {
	ejob, err := s.Engine.GetJob(ctx, getJobId(in.GetJobRef()))
	if err != nil {
		return nil, grpc.Errorf(codes.Internal, "datastore error: %s", err)
	}
	if ejob == nil {
		return nil, grpc.Errorf(codes.NotFound, "Job does not exist or you have no access")
	}

	pageSize := 50
	if in.PageSize > 0 && int(in.PageSize) < pageSize {
		pageSize = int(in.PageSize)
	}

	einvs, cursor, err := s.Engine.ListInvocations(ctx, ejob.JobID, pageSize, in.GetCursor())
	if err != nil {
		return nil, grpc.Errorf(codes.Internal, "datastore error: %s", err)
	}
	invs := make([]*scheduler.Invocation, len(einvs))
	for i, einv := range einvs {
		invs[i] = &scheduler.Invocation{
			InvocationRef: &scheduler.InvocationRef{
				JobRef: &scheduler.JobRef{
					Project: ejob.ProjectID,
					Job:     ejob.GetJobName(),
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

func (s SchedulerServer) PauseJob(ctx context.Context, in *scheduler.JobRef) (*empty.Empty, error) {
	return runAction(ctx, in, func() error {
		return s.Engine.PauseJob(ctx, getJobId(in), auth.CurrentIdentity(ctx))
	})
}

func (s SchedulerServer) ResumeJob(ctx context.Context, in *scheduler.JobRef) (*empty.Empty, error) {
	return runAction(ctx, in, func() error {
		return s.Engine.ResumeJob(ctx, getJobId(in), auth.CurrentIdentity(ctx))
	})
}

func (s SchedulerServer) AbortJob(ctx context.Context, in *scheduler.JobRef) (*empty.Empty, error) {
	return runAction(ctx, in, func() error {
		return s.Engine.AbortJob(ctx, getJobId(in), auth.CurrentIdentity(ctx))
	})
}

func (s SchedulerServer) AbortInvocation(ctx context.Context, in *scheduler.InvocationRef) (
	*empty.Empty, error) {
	return runAction(ctx, in.GetJobRef(), func() error {
		return s.Engine.AbortInvocation(ctx, getJobId(in.GetJobRef()), in.GetInvocationId(), auth.CurrentIdentity(ctx))
	})
}

//// Private helpers.

func runAction(ctx context.Context, jobRef *scheduler.JobRef, action func() error) (*empty.Empty, error) {
	if !presentation.IsJobOwner(ctx, jobRef.GetProject(), jobRef.GetJob()) {
		return nil, grpc.Errorf(codes.PermissionDenied, "No permission to execute action")
	}
	switch err := action(); {
	case err == nil:
		return &empty.Empty{}, nil
	case err == engine.ErrNoSuchJob:
		return nil, grpc.Errorf(codes.NotFound, "no such job")
	case err == engine.ErrNoSuchInvocation:
		return nil, grpc.Errorf(codes.NotFound, "no such invocation")
	default:
		return nil, grpc.Errorf(codes.Internal, "internal error: %s", err)
	}
}

func getJobId(jobRef *scheduler.JobRef) string {
	return jobRef.GetProject() + "/" + jobRef.GetJob()
}
