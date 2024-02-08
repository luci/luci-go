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
	"fmt"
	"reflect"

	"golang.org/x/sync/errgroup"

	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/api/gerrit"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/common/sync/parallel"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/grpc/appstatus"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/dsmapper"
	"go.chromium.org/luci/server/quota"
	"go.chromium.org/luci/server/quota/quotapb"
	"go.chromium.org/luci/server/tq"

	"go.chromium.org/luci/cv/internal/changelist"
	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/common/eventbox"
	"go.chromium.org/luci/cv/internal/gerrit/poller"
	"go.chromium.org/luci/cv/internal/prjmanager"
	"go.chromium.org/luci/cv/internal/prjmanager/prjpb"
	adminpb "go.chromium.org/luci/cv/internal/rpc/admin/api"
	"go.chromium.org/luci/cv/internal/rpc/pagination"
	"go.chromium.org/luci/cv/internal/run"
	"go.chromium.org/luci/cv/internal/run/eventpb"
	"go.chromium.org/luci/cv/internal/run/runquery"
)

// allowGroup is a Chrome Infra Auth group, members of which are allowed to call
// admin API. See https://crbug.com/1183616.
const allowGroup = "service-luci-change-verifier-admins"

type AdminServer struct {
	tqDispatcher *tq.Dispatcher
	clUpdater    *changelist.Updater
	pmNotifier   *prjmanager.Notifier
	runNotifier  *run.Notifier

	dsmapper *dsMapper

	adminpb.UnimplementedAdminServer
}

func New(t *tq.Dispatcher, ctrl *dsmapper.Controller, u *changelist.Updater, p *prjmanager.Notifier, r *run.Notifier) *AdminServer {
	return &AdminServer{
		tqDispatcher: t,
		clUpdater:    u,
		pmNotifier:   p,
		runNotifier:  r,
		dsmapper:     newDSMapper(ctrl),
	}
}

func (a *AdminServer) GetProject(ctx context.Context, req *adminpb.GetProjectRequest) (resp *adminpb.GetProjectResponse, err error) {
	defer func() { err = appstatus.GRPCifyAndLog(ctx, err) }()
	if err = checkAllowed(ctx, "GetProject"); err != nil {
		return
	}
	if req.GetProject() == "" {
		return nil, appstatus.Error(codes.InvalidArgument, "project is required")
	}

	eg, ctx := errgroup.WithContext(ctx)

	var p *prjmanager.Project
	eg.Go(func() (err error) {
		p, err = prjmanager.Load(ctx, req.GetProject())
		return
	})

	resp = &adminpb.GetProjectResponse{}
	eg.Go(func() error {
		list, err := eventbox.List(ctx, prjmanager.EventboxRecipient(ctx, req.GetProject()))
		if err != nil {
			return errors.Annotate(err, "failed to fetch Project Events").Err()
		}
		events := make([]*prjpb.Event, len(list))
		for i, item := range list {
			events[i] = &prjpb.Event{}
			if err = proto.Unmarshal(item.Value, events[i]); err != nil {
				return errors.Annotate(err, "failed to unmarshal Event %q", item.ID).Err()
			}
		}
		resp.Events = events
		return nil
	})

	switch err = eg.Wait(); {
	case err != nil:
		return nil, err
	case p == nil:
		return nil, appstatus.Error(codes.NotFound, "project not found")
	default:
		resp.State = p.State
		resp.State.LuciProject = req.GetProject()
		return resp, nil
	}
}

func (a *AdminServer) GetProjectLogs(ctx context.Context, req *adminpb.GetProjectLogsRequest) (resp *adminpb.GetProjectLogsResponse, err error) {
	defer func() { err = appstatus.GRPCifyAndLog(ctx, err) }()
	if err = checkAllowed(ctx, "GetProjectLogs"); err != nil {
		return
	}
	switch {
	case req.GetPageToken() != "":
		return nil, appstatus.Error(codes.Unimplemented, "not implemented yet")
	case req.GetPageSize() < 0:
		return nil, appstatus.Error(codes.InvalidArgument, "negative page size not allowed")
	case req.GetProject() == "":
		return nil, appstatus.Error(codes.InvalidArgument, "project is required")
	case req.GetEversionMin() < 0:
		return nil, appstatus.Error(codes.InvalidArgument, "eversion_min must be non-negative")
	case req.GetEversionMax() < 0:
		return nil, appstatus.Error(codes.InvalidArgument, "eversion_max must be non-negative")
	}

	q := datastore.NewQuery(prjmanager.ProjectLogKind)
	q = q.Ancestor(datastore.MakeKey(ctx, prjmanager.ProjectKind, req.GetProject()))

	if m := req.GetEversionMin(); m > 0 {
		q.Gte("EVersion", m)
	}
	if m := req.GetEversionMax(); m > 0 {
		q.Lte("EVersion", m)
	}
	switch s := req.GetPageSize(); {
	case s > 1024:
		q.Limit(1024)
	case s > 0:
		q.Limit(s)
	default:
		q.Limit(128)
	}

	var out []*prjmanager.ProjectLog
	if err = datastore.GetAll(ctx, q, &out); err != nil {
		return nil, err
	}

	resp = &adminpb.GetProjectLogsResponse{Logs: make([]*adminpb.ProjectLog, len(out))}
	for i, l := range out {
		resp.Logs[i] = &adminpb.ProjectLog{
			Eversion:   l.EVersion,
			State:      l.State,
			UpdateTime: common.Time2PBNillable(l.UpdateTime),
			Reasons:    &prjpb.LogReasons{Reasons: l.Reasons},
		}
	}
	return resp, nil
}

func (a *AdminServer) GetRun(ctx context.Context, req *adminpb.GetRunRequest) (resp *adminpb.GetRunResponse, err error) {
	defer func() { err = appstatus.GRPCifyAndLog(ctx, err) }()
	if err = checkAllowed(ctx, "GetRun"); err != nil {
		return
	}
	if req.GetRun() == "" {
		return nil, appstatus.Error(codes.InvalidArgument, "run ID is required")
	}
	return loadRunAndEvents(ctx, common.RunID(req.GetRun()), nil)
}

func (a *AdminServer) GetCL(ctx context.Context, req *adminpb.GetCLRequest) (resp *adminpb.GetCLResponse, err error) {
	defer func() { err = appstatus.GRPCifyAndLog(ctx, err) }()
	if err = checkAllowed(ctx, "GetCL"); err != nil {
		return
	}

	cl, err := loadCL(ctx, req)
	if err != nil {
		return nil, err
	}
	runs := make([]string, len(cl.IncompleteRuns))
	for i, id := range cl.IncompleteRuns {
		runs[i] = string(id)
	}
	resp = &adminpb.GetCLResponse{
		Id:               int64(cl.ID),
		Eversion:         cl.EVersion,
		ExternalId:       string(cl.ExternalID),
		UpdateTime:       timestamppb.New(cl.UpdateTime),
		Snapshot:         cl.Snapshot,
		ApplicableConfig: cl.ApplicableConfig,
		Access:           cl.Access,
		IncompleteRuns:   runs,
	}
	return resp, nil
}

func (a *AdminServer) GetPoller(ctx context.Context, req *adminpb.GetPollerRequest) (resp *adminpb.GetPollerResponse, err error) {
	defer func() { err = appstatus.GRPCifyAndLog(ctx, err) }()
	if err = checkAllowed(ctx, "GetPoller"); err != nil {
		return
	}
	if req.GetProject() == "" {
		return nil, appstatus.Error(codes.InvalidArgument, "project is required")
	}

	s := poller.State{LuciProject: req.GetProject()}
	switch err := datastore.Get(ctx, &s); {
	case err == datastore.ErrNoSuchEntity:
		return nil, appstatus.Error(codes.NotFound, "poller not found")
	case err != nil:
		return nil, errors.Annotate(err, "failed to fetch Poller state").Tag(transient.Tag).Err()
	}
	resp = &adminpb.GetPollerResponse{
		Project:     s.LuciProject,
		Eversion:    s.EVersion,
		ConfigHash:  s.ConfigHash,
		UpdateTime:  timestamppb.New(s.UpdateTime),
		QueryStates: s.QueryStates,
	}
	return resp, nil
}

func (a *AdminServer) SearchRuns(ctx context.Context, req *adminpb.SearchRunsRequest) (resp *adminpb.RunsResponse, err error) {
	defer func() { err = appstatus.GRPCifyAndLog(ctx, err) }()
	if err = checkAllowed(ctx, "SearchRuns"); err != nil {
		return
	}
	limit, err := pagination.ValidatePageSize(req, 16, 128)
	if err != nil {
		return nil, err
	}
	var pt *runquery.PageToken
	if s := req.GetPageToken(); s != "" {
		pt = &runquery.PageToken{}
		if err := pagination.DecryptPageToken(ctx, req.GetPageToken(), pt); err != nil {
			return nil, err
		}
	}

	// Compute potentially interesting run keys using the most efficient query.
	var runKeys []*datastore.Key
	var cl *changelist.CL
	switch {
	case req.GetCl() != nil:
		cl, runKeys, err = searchRunsByCL(ctx, req, pt, limit)
	case req.GetProject() != "":
		runKeys, err = searchRunsByProject(ctx, req, pt, limit)
	default:
		runKeys, err = searchRecentRunsSlow(ctx, req, pt, limit)
	}
	if err != nil {
		return nil, err
	}

	// Fetch individual runs in parallel and apply final filtering.
	shouldSkip := func(r *run.Run) bool {
		if req.GetProject() != "" && req.GetProject() != r.ID.LUCIProject() {
			return true
		}
		if req.GetCl() != nil && !r.CLs.Contains(cl.ID) {
			return true
		}
		switch s := req.GetStatus(); s {
		case run.Status_STATUS_UNSPECIFIED:
		case run.Status_ENDED_MASK:
			if !run.IsEnded(r.Status) {
				return true
			}
		default:
			if s != r.Status {
				return true
			}
		}
		if m := req.GetMode(); m != "" && run.Mode(m) != r.Mode {
			return true
		}
		return false
	}
	runs := make([]*adminpb.GetRunResponse, len(runKeys))
	errs := parallel.WorkPool(min(len(runKeys), 16), func(work chan<- func() error) {
		for i, key := range runKeys {
			i, key := i, key
			work <- func() (err error) {
				runs[i], err = loadRunAndEvents(ctx, common.RunID(key.StringID()), shouldSkip)
				return
			}
		}
	})
	if errs != nil {
		return nil, common.MostSevereError(errs)
	}

	resp = &adminpb.RunsResponse{}

	// Remove nil runs, which were skipped above.
	resp.Runs = runs[:0]
	for _, r := range runs {
		if r != nil {
			resp.Runs = append(resp.Runs, r)
		}
	}

	if l := len(runKeys); int32(l) == limit {
		resp.NextPageToken, err = pagination.EncryptPageToken(ctx, &runquery.PageToken{Run: runKeys[l-1].StringID()})
		if err != nil {
			return nil, err
		}
	}
	return resp, nil
}

// searchRunsByCL returns CL & Run IDs as Datastore keys, using CL to limit
// results.
func searchRunsByCL(ctx context.Context, req *adminpb.SearchRunsRequest, pt *runquery.PageToken, limit int32) (*changelist.CL, []*datastore.Key, error) {
	cl, err := loadCL(ctx, req.GetCl())
	if err != nil {
		return nil, nil, err
	}

	qb := runquery.CLQueryBuilder{
		CLID:    cl.ID,
		Limit:   limit,
		Project: req.GetProject(), // optional
	}.PageToken(pt)
	runKeys, err := qb.GetAllRunKeys(ctx)
	return cl, runKeys, err
}

// searchRunsByProject returns Run IDs as Datastore keys, using LUCI Project to
// limit results.
func searchRunsByProject(ctx context.Context, req *adminpb.SearchRunsRequest, pt *runquery.PageToken, limit int32) ([]*datastore.Key, error) {
	qb := runquery.ProjectQueryBuilder{
		Project: req.GetProject(),
		Limit:   limit,
		Status:  req.GetStatus(), // optional
	}.PageToken(pt)
	return qb.GetAllRunKeys(ctx)
}

// searchRecentRunsSlow returns Run IDs as Datastore keys for the most recent
// Runs.
func searchRecentRunsSlow(ctx context.Context, req *adminpb.SearchRunsRequest, pt *runquery.PageToken, limit int32) ([]*datastore.Key, error) {
	return runquery.RecentQueryBuilder{
		Status: req.GetStatus(), // optional
		Limit:  limit,
	}.PageToken(pt).GetAllRunKeys(ctx)
}

// Copy from dsset.
type itemEntity struct {
	_kind string `gae:"$kind,dsset.Item"`

	ID     string         `gae:"$id"`
	Parent *datastore.Key `gae:"$parent"`
	Value  []byte         `gae:",noindex"`
}

func (a *AdminServer) DeleteProjectEvents(ctx context.Context, req *adminpb.DeleteProjectEventsRequest) (resp *adminpb.DeleteProjectEventsResponse, err error) {
	defer func() { err = appstatus.GRPCifyAndLog(ctx, err) }()
	if err = checkAllowed(ctx, "DeleteProjectEvents"); err != nil {
		return
	}

	switch {
	case req.GetProject() == "":
		return nil, appstatus.Error(codes.InvalidArgument, "project is required")
	case req.GetLimit() <= 0:
		return nil, appstatus.Error(codes.InvalidArgument, "limit must be >0")
	}

	parent := datastore.MakeKey(ctx, prjmanager.ProjectKind, req.GetProject())
	q := datastore.NewQuery("dsset.Item").Ancestor(parent).Limit(req.GetLimit())
	var entities []*itemEntity
	if err := datastore.GetAll(ctx, q, &entities); err != nil {
		return nil, errors.Annotate(err, "failed to fetch up to %d events", req.GetLimit()).Tag(transient.Tag).Err()
	}

	stats := make(map[string]int64, 10)
	for _, e := range entities {
		pb := &prjpb.Event{}
		if err := proto.Unmarshal(e.Value, pb); err != nil {
			stats["<unknown>"]++
		} else {
			stats[fmt.Sprintf("%T", pb.GetEvent())]++
		}
	}
	if err := datastore.Delete(ctx, entities); err != nil {
		return nil, errors.Annotate(err, "failed to delete %d events", len(entities)).Tag(transient.Tag).Err()
	}
	return &adminpb.DeleteProjectEventsResponse{Events: stats}, nil
}

func (a *AdminServer) RefreshProjectCLs(ctx context.Context, req *adminpb.RefreshProjectCLsRequest) (resp *adminpb.RefreshProjectCLsResponse, err error) {
	defer func() { err = appstatus.GRPCifyAndLog(ctx, err) }()
	if err = checkAllowed(ctx, "RefreshProjectCLs"); err != nil {
		return
	}
	if req.GetProject() == "" {
		return nil, appstatus.Error(codes.InvalidArgument, "project is required")
	}

	p, err := prjmanager.Load(ctx, req.GetProject())
	if err != nil {
		return nil, errors.Annotate(err, "failed to fetch Project %q", req.GetProject()).Tag(transient.Tag).Err()
	}

	cls := make([]*changelist.CL, len(p.State.GetPcls()))
	errs := parallel.WorkPool(20, func(work chan<- func() error) {
		for i, pcl := range p.State.GetPcls() {
			i := i
			id := pcl.GetClid()
			work <- func() error {
				// Load individual CL to avoid OOMs.
				cl := changelist.CL{ID: common.CLID(id)}
				if err := datastore.Get(ctx, &cl); err != nil {
					return errors.Annotate(err, "failed to fetch CL %d", id).Tag(transient.Tag).Err()
				}
				cls[i] = &changelist.CL{ID: cl.ID, EVersion: cl.EVersion}
				payload := &changelist.UpdateCLTask{
					LuciProject: req.GetProject(),
					ExternalId:  string(cl.ExternalID),
					Id:          int64(cl.ID),
					Requester:   changelist.UpdateCLTask_RPC_ADMIN,
				}
				return a.clUpdater.Schedule(ctx, payload)
			}
		}
	})
	if err := common.MostSevereError(errs); err != nil {
		return nil, err
	}

	if err := a.pmNotifier.NotifyCLsUpdated(ctx, req.GetProject(), changelist.ToUpdatedEvents(cls...)); err != nil {
		return nil, err
	}

	clvs := make(map[int64]int64, len(p.State.GetPcls()))
	for _, cl := range cls {
		clvs[int64(cl.ID)] = cl.EVersion
	}
	return &adminpb.RefreshProjectCLsResponse{ClVersions: clvs}, nil
}

func (a *AdminServer) SendProjectEvent(ctx context.Context, req *adminpb.SendProjectEventRequest) (_ *emptypb.Empty, err error) {
	defer func() { err = appstatus.GRPCifyAndLog(ctx, err) }()
	if err = checkAllowed(ctx, "SendProjectEvent"); err != nil {
		return
	}
	switch {
	case req.GetProject() == "":
		return nil, appstatus.Error(codes.InvalidArgument, "project is required")
	case req.GetEvent().GetEvent() == nil:
		return nil, appstatus.Error(codes.InvalidArgument, "event with a specific inner event is required")
	}

	switch p, err := prjmanager.Load(ctx, req.GetProject()); {
	case err != nil:
		return nil, errors.Annotate(err, "failed to fetch Project").Err()
	case p == nil:
		return nil, appstatus.Error(codes.NotFound, "project not found")
	}

	if err := a.pmNotifier.SendNow(ctx, req.GetProject(), req.GetEvent()); err != nil {
		return nil, errors.Annotate(err, "failed to send event").Err()
	}
	return &emptypb.Empty{}, nil
}

func (a *AdminServer) SendRunEvent(ctx context.Context, req *adminpb.SendRunEventRequest) (_ *emptypb.Empty, err error) {
	defer func() { err = appstatus.GRPCifyAndLog(ctx, err) }()
	if err = checkAllowed(ctx, "SendRunEvent"); err != nil {
		return
	}
	switch {
	case req.GetRun() == "":
		return nil, appstatus.Error(codes.InvalidArgument, "Run is required")
	case req.GetEvent().GetEvent() == nil:
		return nil, appstatus.Error(codes.InvalidArgument, "event with a specific inner event is required")
	}

	switch r, err := run.LoadRun(ctx, common.RunID(req.GetRun())); {
	case err != nil:
		return nil, errors.Annotate(err, "failed to fetch Run").Tag(transient.Tag).Err()
	case r == nil:
		return nil, appstatus.Error(codes.NotFound, "Run not found")
	}

	if err := a.runNotifier.SendNow(ctx, common.RunID(req.GetRun()), req.GetEvent()); err != nil {
		return nil, errors.Annotate(err, "failed to send event").Err()
	}
	return &emptypb.Empty{}, nil
}

func (a *AdminServer) ScheduleTask(ctx context.Context, req *adminpb.ScheduleTaskRequest) (_ *emptypb.Empty, err error) {
	defer func() { err = appstatus.GRPCifyAndLog(ctx, err) }()
	if err = checkAllowed(ctx, "ScheduleTask"); err != nil {
		return
	}

	const trans = true
	var possiblePayloads = []struct {
		inTransaction bool
		payload       proto.Message
	}{
		{trans, req.GetBatchUpdateCl()},
		{trans, req.GetBatchOnClUpdated()},
		{false, req.GetExportRunToBq()},
		{trans, req.GetKickManageProject()},
		{trans, req.GetKickManageRun()},
		{false, req.GetManageProject()},
		{false, req.GetManageRun()},
		{false, req.GetPollGerrit()},
		{trans, req.GetPurgeCl()},
		{false, req.GetUpdateCl()},
		{false, req.GetRefreshProjectConfig()},
		{trans, req.GetManageRunLongOp()},
	}

	chosen := possiblePayloads[0]
	for _, another := range possiblePayloads[1:] {
		switch {
		case reflect.ValueOf(another.payload).IsNil():
		case !reflect.ValueOf(chosen.payload).IsNil():
			return nil, appstatus.Error(codes.InvalidArgument, "exactly one task payload required, but 2+ given")
		default:
			chosen = another
		}
	}

	if reflect.ValueOf(chosen.payload).IsNil() {
		return nil, appstatus.Error(codes.InvalidArgument, "exactly one task payload required, but none given")
	}
	kind := chosen.payload.ProtoReflect().Type().Descriptor().Name()
	if chosen.inTransaction && req.GetDeduplicationKey() != "" {
		return nil, appstatus.Errorf(codes.InvalidArgument, "task %q is transactional, so the deduplication_key is not allowed", kind)
	}

	t := &tq.Task{
		Payload:          chosen.payload,
		DeduplicationKey: req.GetDeduplicationKey(),
		Title:            fmt.Sprintf("admin/%s/%s/%s", auth.CurrentIdentity(ctx), kind, req.GetDeduplicationKey()),
	}

	if chosen.inTransaction {
		err = datastore.RunInTransaction(ctx, func(ctx context.Context) error {
			return a.tqDispatcher.AddTask(ctx, t)
		}, nil)
	} else {
		err = a.tqDispatcher.AddTask(ctx, t)
	}

	if err != nil {
		return nil, errors.Annotate(err, "failed to schedule task").Err()
	}
	return &emptypb.Empty{}, nil
}

func (a *AdminServer) GetQuotaAccounts(ctx context.Context, req *quotapb.GetAccountsRequest) (resp *quotapb.GetAccountsResponse, err error) {
	defer func() { err = appstatus.GRPCifyAndLog(ctx, err) }()
	if err := checkAllowed(ctx, "GetQuotaAccounts"); err != nil {
		return nil, err
	}

	if err := req.Validate(); err != nil {
		return nil, appstatus.Errorf(codes.InvalidArgument, "request validation error: %s", err)
	}

	return quota.GetAccounts(ctx, req.Account)
}

func (a *AdminServer) ApplyQuotaOps(ctx context.Context, req *quotapb.ApplyOpsRequest) (resp *quotapb.ApplyOpsResponse, err error) {
	defer func() { err = appstatus.GRPCifyAndLog(ctx, err) }()
	if err := checkAllowed(ctx, "ApplyQuotaOps"); err != nil {
		return nil, err
	}

	if err := req.Validate(); err != nil {
		return nil, appstatus.Errorf(codes.InvalidArgument, "request validation error: %s", err)
	}

	return quota.ApplyOps(ctx, req.RequestId, req.RequestIdTtl, req.Ops)
}

func checkAllowed(ctx context.Context, name string) error {
	switch yes, err := auth.IsMember(ctx, allowGroup); {
	case err != nil:
		return errors.Annotate(err, "failed to check ACL").Err()
	case !yes:
		return appstatus.Errorf(codes.PermissionDenied, "not a member of %s", allowGroup)
	default:
		logging.Warningf(ctx, "%s is calling admin.%s", auth.CurrentIdentity(ctx), name)
		return nil
	}
}

func loadRunAndEvents(ctx context.Context, rid common.RunID, shouldSkip func(r *run.Run) bool) (*adminpb.GetRunResponse, error) {
	r, err := run.LoadRun(ctx, rid)
	switch {
	case err != nil:
		return nil, err
	case r == nil:
		return nil, appstatus.Error(codes.NotFound, "Run not found")
	case shouldSkip != nil && shouldSkip(r):
		return nil, nil
	}

	eg, ctx := errgroup.WithContext(ctx)
	var cls []*adminpb.GetRunResponse_CL
	eg.Go(func() error {
		switch rcls, err := run.LoadRunCLs(ctx, r.ID, r.CLs); {
		case err != nil:
			return errors.Annotate(err, "failed to fetch RunCLs").Err()
		default:
			cls = make([]*adminpb.GetRunResponse_CL, len(rcls))
			for i, rcl := range rcls {
				cls[i] = &adminpb.GetRunResponse_CL{
					Id:         int64(rcl.ID),
					ExternalId: string(rcl.ExternalID),
					Detail:     rcl.Detail,
					Trigger:    rcl.Trigger,
				}
			}
			return nil
		}
	})

	var logEntries []*run.LogEntry
	eg.Go(func() error {
		var err error
		logEntries, err = run.LoadRunLogEntries(ctx, r.ID)
		if err != nil {
			return errors.Annotate(err, "failed to fetch RunCLs").Err()
		}
		return nil
	})

	var events []*eventpb.Event
	eg.Go(func() error {
		list, err := eventbox.List(ctx, run.EventboxRecipient(ctx, rid))
		if err != nil {
			return errors.Annotate(err, "failed to fetch Run Events").Err()
		}
		events = make([]*eventpb.Event, len(list))
		for i, item := range list {
			events[i] = &eventpb.Event{}
			if err = proto.Unmarshal(item.Value, events[i]); err != nil {
				return errors.Annotate(err, "failed to unmarshal Event %q", item.ID).Err()
			}
		}
		return nil
	})

	if err := eg.Wait(); err != nil {
		return nil, err
	}

	return &adminpb.GetRunResponse{
		Id:                  string(rid),
		Eversion:            r.EVersion,
		Mode:                string(r.Mode),
		Status:              r.Status,
		CreateTime:          common.Time2PBNillable(r.CreateTime),
		StartTime:           common.Time2PBNillable(r.StartTime),
		UpdateTime:          common.Time2PBNillable(r.UpdateTime),
		EndTime:             common.Time2PBNillable(r.EndTime),
		Owner:               string(r.Owner),
		CreatedBy:           string(r.CreatedBy),
		BilledTo:            string(r.BilledTo),
		ConfigGroupId:       string(r.ConfigGroupID),
		Cls:                 cls,
		Options:             r.Options,
		CancellationReasons: r.CancellationReasons,
		Tryjobs:             r.Tryjobs,
		OngoingLongOps:      r.OngoingLongOps,
		Submission:          r.Submission,
		LatestClsRefresh:    common.Time2PBNillable(r.LatestCLsRefresh),

		LogEntries: logEntries,
		Events:     events,
	}, nil
}

func loadCL(ctx context.Context, req *adminpb.GetCLRequest) (*changelist.CL, error) {
	var err error
	var cl *changelist.CL
	var eid changelist.ExternalID
	switch {
	case req.GetId() != 0:
		cl = &changelist.CL{ID: common.CLID(req.GetId())}
		err = datastore.Get(ctx, cl)
		switch {
		case err == datastore.ErrNoSuchEntity:
			cl, err = nil, nil
		case err != nil:
			err = errors.Annotate(err, "failed to fetch CL by InternalID %d", req.GetId()).Tag(transient.Tag).Err()
		}
	case req.GetExternalId() != "":
		eid = changelist.ExternalID(req.GetExternalId())
		cl, err = eid.Load(ctx)
	case req.GetGerritUrl() != "":
		var host string
		var change int64
		host, change, err = gerrit.FuzzyParseURL(req.GetGerritUrl())
		if err != nil {
			return nil, appstatus.Errorf(codes.InvalidArgument, "invalid Gerrit URL %q: %s", req.GetGerritUrl(), err)
		}
		eid, err = changelist.GobID(host, change)
		if err != nil {
			return nil, appstatus.Errorf(codes.InvalidArgument, "invalid Gerrit URL %q: %s", req.GetGerritUrl(), err)
		}
		cl, err = eid.Load(ctx)
	default:
		return nil, appstatus.Error(codes.InvalidArgument, "id or external_id or gerrit_url is required")
	}

	switch {
	case err != nil:
		return nil, err
	case cl == nil:
		if req.GetId() == 0 {
			return nil, appstatus.Errorf(codes.NotFound, "CL %d not found", req.GetId())
		}
		return nil, appstatus.Errorf(codes.NotFound, "CL %s not found", eid)
	}
	return cl, nil
}
