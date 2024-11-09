// Copyright 2019 The LUCI Authors.
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

package rpc

import (
	"context"

	"github.com/golang/protobuf/proto"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/proto/paged"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/auth"

	"go.chromium.org/luci/gce/api/projects/v1"
	"go.chromium.org/luci/gce/appengine/model"
)

// Projects implements projects.ProjectsServer.
type Projects struct {
}

// Ensure Projects implements projects.ProjectsServer.
var _ projects.ProjectsServer = &Projects{}

// Delete handles a request to delete a project.
func (*Projects) Delete(c context.Context, req *projects.DeleteRequest) (*emptypb.Empty, error) {
	if req.GetId() == "" {
		return nil, status.Errorf(codes.InvalidArgument, "ID is required")
	}
	if err := datastore.Delete(c, &model.Project{ID: req.Id}); err != nil {
		return nil, errors.Annotate(err, "failed to delete project").Err()
	}
	return &emptypb.Empty{}, nil
}

// Ensure handles a request to create or update a project.
func (*Projects) Ensure(c context.Context, req *projects.EnsureRequest) (*projects.Config, error) {
	if req.GetId() == "" {
		return nil, status.Errorf(codes.InvalidArgument, "ID is required")
	}
	p := &model.Project{
		ID:     req.Id,
		Config: req.Project,
	}
	if err := datastore.Put(c, p); err != nil {
		return nil, errors.Annotate(err, "failed to store project").Err()
	}
	return p.Config, nil
}

// Get handles a request to get a project.
func (*Projects) Get(c context.Context, req *projects.GetRequest) (*projects.Config, error) {
	if req.GetId() == "" {
		return nil, status.Errorf(codes.InvalidArgument, "ID is required")
	}
	p := &model.Project{
		ID: req.Id,
	}
	switch err := datastore.Get(c, p); err {
	case nil:
		return p.Config, nil
	case datastore.ErrNoSuchEntity:
		return nil, status.Errorf(codes.NotFound, "no project found with ID %q", req.Id)
	default:
		return nil, errors.Annotate(err, "failed to fetch project").Err()
	}
}

// List handles a request to list all projects.
func (*Projects) List(c context.Context, req *projects.ListRequest) (*projects.ListResponse, error) {
	rsp := &projects.ListResponse{}
	if err := paged.Query(c, req.GetPageSize(), req.GetPageToken(), rsp, datastore.NewQuery(model.ProjectKind), func(p *model.Project) error {
		rsp.Projects = append(rsp.Projects, p.Config)
		return nil
	}); err != nil {
		return nil, err
	}
	return rsp, nil
}

// projectsPrelude ensures the user is authorized to use the read-only projects
// API. Always returns permission denied for write methods.
func projectsPrelude(c context.Context, methodName string, req proto.Message) (context.Context, error) {
	if !isReadOnly(methodName) {
		return c, status.Errorf(codes.PermissionDenied, "unauthorized user")
	}
	switch is, err := auth.IsMember(c, admins, writers, readers); {
	case err != nil:
		return c, err
	case is:
		logging.Debugf(c, "%s called %q:\n%s", auth.CurrentIdentity(c), methodName, req)
		return c, nil
	}
	return c, status.Errorf(codes.PermissionDenied, "unauthorized user")
}

// NewProjectsServer returns a new projects server.
func NewProjectsServer() projects.ProjectsServer {
	return &projects.DecoratedProjects{
		Prelude:  projectsPrelude,
		Service:  &Projects{},
		Postlude: gRPCifyAndLogErr,
	}
}
