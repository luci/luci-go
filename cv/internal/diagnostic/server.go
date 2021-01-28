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
	"net/url"
	"regexp"
	"strconv"

	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/grpc/grpcutil"
	"go.chromium.org/luci/server/auth"

	diagnosticpb "go.chromium.org/luci/cv/api/diagnostic"
	"go.chromium.org/luci/cv/internal/changelist"
	"go.chromium.org/luci/cv/internal/common"
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
	eg.Go(func() (err error) {
		p, err = prjmanager.Load(ctx, req.GetProject())
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
		resp.State = p.State
		resp.State.LuciProject = req.GetProject()
		return resp, nil
	}
}

func (d *DiagnosticServer) GetCL(ctx context.Context, req *diagnosticpb.GetCLRequest) (resp *diagnosticpb.GetCLResponse, err error) {
	var cl *changelist.CL
	var eid changelist.ExternalID
	switch {
	case req.GetId() != 0:
		cl.ID = common.CLID(req.GetId())
		err = datastore.Get(ctx, cl)
	case req.GetExternalId() != "":
		eid = changelist.ExternalID(req.GetExternalId())
		cl, err = eid.Get(ctx)
	case req.GetGerritUrl() != "":
		eid, err = parseGerritURL(req.GetExternalId())
		if err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "invalid Gerrit URL %q: %s", req.GetGerritUrl(), err)
		}
		cl, err = eid.Get(ctx)
	default:
		return nil, status.Errorf(codes.InvalidArgument, "id or external_id or gerrit_url is required")
	}

	switch {
	case err == datastore.ErrNoSuchEntity:
		if req.GetId() == 0 {
			return nil, status.Errorf(codes.NotFound, "CL %d not found", req.GetId())
		}
		return nil, status.Errorf(codes.NotFound, "CL %s not found", eid)
	case err != nil:
		return nil, err
	}
	runs := make([]string, len(cl.IncompleteRuns))
	for i, id := range cl.IncompleteRuns {
		runs[i] = string(id)
	}
	resp = &diagnosticpb.GetCLResponse{
		Id:               int64(cl.ID),
		ExternalId:       string(cl.ExternalID),
		Snapshot:         cl.Snapshot,
		ApplicableConfig: cl.ApplicableConfig,
		DependentMeta:    cl.DependentMeta,
		IncompleteRuns:   runs,
	}
	return resp, nil
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

var regexCrRevPath = regexp.MustCompile(`/([ci])/(\d+)(/(\d+))?`)
var regexGoB = regexp.MustCompile(`((\w+-)+review\.googlesource\.com)/(#/)?(c/)?(([^\+]+)/\+/)?(\d+)(/(\d+)?)?`)

func parseGerritURL(s string) (changelist.ExternalID, error) {
	u, err := url.Parse(s)
	if err != nil {
		return "", err
	}
	var host string
	var change int64
	if u.Host == "crrev.com" {
		m := regexCrRevPath.FindStringSubmatch(u.Path)
		if m == nil {
			return "", errors.New("invalid crrev.com URL")
		}
		switch m[1] {
		case "c":
			host = "chromium-review.googlesource.com"
		case "i":
			host = "chrome-internal-review.googlesource.com"
		default:
			panic("impossible")
		}
		if change, err = strconv.ParseInt(m[2], 10, 64); err != nil {
			return "", errors.Reason("invalid crrev.com URL change number /%s/", m[2]).Err()
		}
	} else {
		m := regexGoB.FindStringSubmatch(s)
		if m == nil {
			return "", errors.Reason("Gerrit URL didn't match regexp %q", regexGoB.String()).Err()
		}
		if host = m[1]; host == "" {
			return "", errors.New("invalid Gerrit host")
		}
		if change, err = strconv.ParseInt(m[7], 10, 64); err != nil {
			return "", errors.Reason("invalid Gerrit URL change number /%s/", m[7]).Err()
		}
	}
	return changelist.GobID(host, change)
}
