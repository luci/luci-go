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
	"strings"

	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/types/known/emptypb"

	"go.chromium.org/luci/common/tsmon/field"
	"go.chromium.org/luci/common/tsmon/metric"
	"go.chromium.org/luci/grpc/appstatus"
	"go.chromium.org/luci/server/dsmapper"

	adminpb "go.chromium.org/luci/cv/internal/rpc/admin/api"
)

var (
	metricUpgraded = metric.NewCounter(
		"cv/internal/dsmapper/upgraded",
		"Number of Datastore entities upgraded with dsmapper",
		nil,
		field.String("name"), // dsmapper.ID
		field.Int("id"),      // dsmapper.JobID
		field.String("kind"), // entity Kind
	)
)

type dsMapper struct {
	ctrl  *dsmapper.Controller
	cfgs  map[dsmapper.ID]*dsmapper.JobConfig
	final bool
}

func newDSMapper(ctrl *dsmapper.Controller) *dsMapper {
	s := &dsMapper{ctrl: ctrl}
	// Add your dsmapper factory here,
	// e.g. s.register(&yourConfig, yourFactory)

	// TODO(crbug/1260615): remove descriptions once CQDaemon is gone.
	s.register(&removeCLDescriptionsCOnfig, removeCLDescriptionsFactory)
	s.register(&multiCLAnalysisConfig, multiCLAnalysisMapperFactory)
	s.register(&backfillRetentionKey, backfillRetentionKeyFactory)
	s.register(&deleteEntitiesKey, deleteEntitiesFactory)

	s.final = true
	return s
}

func (d *dsMapper) register(cfg *dsmapper.JobConfig, f dsmapper.Factory) {
	if d.final {
		panic("register must be called in New()")
	}
	if err := cfg.Validate(); err != nil {
		panic(err)
	}
	if f == nil {
		panic("dsmapper.Factory must not be nil")
	}
	d.ctrl.RegisterFactory(cfg.Mapper, f) // panics if id not unique
	if d.cfgs == nil {
		d.cfgs = make(map[dsmapper.ID]*dsmapper.JobConfig, 1)
	}
	d.cfgs[cfg.Mapper] = cfg
}

func (a *AdminServer) DSMLaunchJob(ctx context.Context, req *adminpb.DSMLaunchJobRequest) (resp *adminpb.DSMJobID, err error) {
	defer func() { err = appstatus.GRPCifyAndLog(ctx, err) }()
	if err = checkAllowed(ctx, "DSMLaunchJob"); err != nil {
		return
	}

	cfg, exists := a.dsmapper.cfgs[dsmapper.ID(req.GetName())]
	if !exists {
		var sb strings.Builder
		_, _ = fmt.Fprintf(&sb, "Name %q is not pregistered; %d known names are: ", req.GetName(), len(a.dsmapper.cfgs))
		for name := range a.dsmapper.cfgs {
			sb.WriteString(string(name))
			sb.WriteRune(',')
		}
		return nil, appstatus.Error(codes.NotFound, strings.TrimRight(sb.String(), ","))
	}

	jobID, err := a.dsmapper.ctrl.LaunchJob(ctx, cfg)
	if err != nil {
		return nil, err
	}
	return &adminpb.DSMJobID{Id: int64(jobID)}, nil
}

func (a *AdminServer) DSMGetJob(ctx context.Context, req *adminpb.DSMJobID) (_ *adminpb.DSMJob, err error) {
	defer func() { err = appstatus.GRPCifyAndLog(ctx, err) }()
	if err = checkAllowed(ctx, "DSMGetJob"); err != nil {
		return
	}

	id := dsmapper.JobID(req.GetId())
	job, err := a.dsmapper.ctrl.GetJob(ctx, id)
	switch {
	case err == dsmapper.ErrNoSuchJob:
		return nil, appstatus.Error(codes.NotFound, "not found")
	case err != nil:
		return nil, err
	}

	info, err := job.FetchInfo(ctx)
	if err != nil {
		return nil, err
	}

	return &adminpb.DSMJob{
		Name: string(job.Config.Mapper),
		Info: info,
	}, nil
}

func (a *AdminServer) DSMAbortJob(ctx context.Context, req *adminpb.DSMJobID) (_ *emptypb.Empty, err error) {
	defer func() { err = appstatus.GRPCifyAndLog(ctx, err) }()
	if err = checkAllowed(ctx, "DSMAbortJob"); err != nil {
		return
	}

	id := dsmapper.JobID(req.GetId())
	switch _, err := a.dsmapper.ctrl.AbortJob(ctx, id); {
	case err == dsmapper.ErrNoSuchJob:
		return nil, appstatus.Error(codes.NotFound, "not found")
	case err != nil:
		return nil, err
	default:
		return &emptypb.Empty{}, nil
	}
}
