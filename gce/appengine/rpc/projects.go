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

	"github.com/golang/protobuf/ptypes/empty"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/gae/service/datastore"
	"go.chromium.org/luci/common/errors"

	"go.chromium.org/luci/gce/api/projects/v1"
	"go.chromium.org/luci/gce/appengine/model"
)

// Projects implements projects.ProjectsServer.
type Projects struct {
}

// Ensure Projects implements projects.ProjectsServer.
var _ projects.ProjectsServer = &Projects{}

// Delete handles a request to delete a project.
func (*Projects) Delete(c context.Context, req *projects.DeleteRequest) (*empty.Empty, error) {
	if req.GetId() == "" {
		return nil, status.Errorf(codes.InvalidArgument, "ID is required")
	}
	if err := datastore.Delete(c, &model.Project{ID: req.Id}); err != nil {
		return nil, errors.Annotate(err, "failed to delete project").Err()
	}
	return &empty.Empty{}, nil
}

// Ensure handles a request to create or update a project.
func (*Projects) Ensure(c context.Context, req *projects.EnsureRequest) (*projects.Config, error) {
	if req.GetId() == "" {
		return nil, status.Errorf(codes.InvalidArgument, "ID is required")
	}
	p := &model.Project{
		ID:     req.Id,
		Config: *req.Project,
	}
	if err := datastore.Put(c, p); err != nil {
		return nil, errors.Annotate(err, "failed to store project").Err()
	}
	return &p.Config, nil
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
		return &p.Config, nil
	case datastore.ErrNoSuchEntity:
		return nil, status.Errorf(codes.NotFound, "no project found with ID %q", req.Id)
	default:
		return nil, errors.Annotate(err, "failed to fetch project").Err()
	}
}

// List handles a request to list all projects.
func (*Projects) List(c context.Context, req *projects.ListRequest) (*projects.ListResponse, error) {
	q, err := pageQuery(c, req, datastore.NewQuery(model.ProjectKind))
	if err != nil {
		return nil, err
	}
	rsp := &projects.ListResponse{}
	var getCur datastore.CursorCB
	if err := datastore.Run(c, q, func(p *model.Project, f datastore.CursorCB) error {
		rsp.Projects = append(rsp.Projects, &p.Config)
		getCur = f
		return nil
	}); err != nil {
		return nil, errors.Annotate(err, "failed to fetch projects").Err()
	}
	if getCur != nil {
		cur, err := getCur()
		if err != nil {
			return nil, errors.Annotate(err, "failed to fetch cursor").Err()
		}
		rsp.NextPageToken = cur.String()
	}
	return rsp, nil
}

// NewProjectsServer returns a new projects server.
func NewProjectsServer() projects.ProjectsServer {
	return &projects.DecoratedProjects{
		Prelude:  readOnlyAuthPrelude,
		Service:  &Projects{},
		Postlude: gRPCifyAndLogErr,
	}
}
