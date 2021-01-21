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

package diagnostic

import (
	"context"

	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/grpc/grpcutil"
	"go.chromium.org/luci/server/auth"

	diagnosticpb "go.chromium.org/luci/cv/api/diagnostic"
	"go.chromium.org/luci/cv/internal/eventbox"
	"go.chromium.org/luci/cv/internal/prjmanager"
	"go.chromium.org/luci/cv/internal/prjmanager/prjpb"
)

// allowGroup is a Chrome Infra Auth group, members of which are allowed to call
// diagnostic API.
const allowGroup = "administrators"

type DiagnosticServer struct {
	diagnosticpb.UnimplementedDiagnosticServer
}

func (d *DiagnosticServer) GetProject(ctx context.Context, req *diagnosticpb.GetProjectRequest) (resp *diagnosticpb.GetProjectResponse, err error) {
	defer func() { err = grpcutil.GRPCifyAndLogErr(ctx, err) }()
	if err = d.checkAllowed(ctx); err != nil {
		return
	}
	if req.GetProject() == "" {
		return nil, status.Errorf(codes.InvalidArgument, "project is required")
	}

	eg, ctx := errgroup.WithContext(ctx)

	var p *prjmanager.Project
	var po *prjmanager.ProjectStateOffload
	eg.Go(func() (err error) {
		p, po, err = prjmanager.Load(ctx, req.GetProject())
		return
	})

	resp = &diagnosticpb.GetProjectResponse{}
	eg.Go(func() error {
		list, err := eventbox.List(ctx, datastore.MakeKey(ctx, prjmanager.ProjectKind, req.GetProject()))
		if err != nil {
			return err
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
		return nil, status.Errorf(codes.NotFound, "project not found")
	default:
		resp.Status = po.Status
		resp.ConfigHash = po.ConfigHash
		resp.Pstate = p.State
		resp.Pstate.LuciProject = req.GetProject()
		return resp, nil
	}
}

func (_ *DiagnosticServer) checkAllowed(ctx context.Context) error {
	switch yes, err := auth.IsMember(ctx, allowGroup); {
	case err != nil:
		return status.Errorf(codes.Internal, "failed to check ACL")
	case !yes:
		return status.Errorf(codes.PermissionDenied, "not a member of %s", allowGroup)
	default:
		return nil
	}
}
