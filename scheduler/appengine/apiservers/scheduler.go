// Copyright 2016 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package apiservers

import (
	"github.com/luci/luci-go/scheduler/api/scheduler/v1"
	"github.com/luci/luci-go/scheduler/appengine/catalog"
	"github.com/luci/luci-go/scheduler/appengine/engine"
	"github.com/luci/luci-go/scheduler/appengine/presentation"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
)

// SchedulerServer implements scheduler.Scheduler API.
type SchedulerServer struct {
	Engine  engine.Engine
	Catalog catalog.Catalog
}

var _ scheduler.SchedulerServer = (*SchedulerServer)(nil)

// GetJobs fetches all jobs satisfying JobsRequest and visibility ACLs.
func (s SchedulerServer) GetJobs(ctx context.Context, in *scheduler.JobsRequest) (*scheduler.JobsReply, error) {
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
			Name:     ej.GetJobName(),
			Project:  ej.ProjectID,
			Schedule: ej.Schedule,
			State: &scheduler.JobState{
				UiStatus: string(presentation.GetPublicStateKind(ej, traits)),
			},
		}
	}
	return &scheduler.JobsReply{Jobs: jobs}, nil
}

func (s SchedulerServer) GetInvocations(ctx context.Context, in *scheduler.InvocationsRequest) (*scheduler.InvocationsReply, error) {
	ejob, err := s.Engine.GetJob(ctx, in.GetProject()+"/"+in.GetJob())
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
			Id:             einv.ID,
			Job:            ejob.GetJobName(),
			Project:        ejob.ProjectID,
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
