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

package memory

import (
	"context"
	"sync"

	"github.com/golang/protobuf/ptypes/empty"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/gce/api/projects/v1"
)

// Ensure Projects implements projects.ProjectsServer.
var _ projects.ProjectsServer = &Projects{}

// Projects implements projects.ProjectsServer.
type Projects struct {
	// cfg is a map of IDs to known projects.
	cfg sync.Map
}

// Delete handles a request to delete a project.
func (srv *Projects) Delete(c context.Context, req *projects.DeleteRequest) (*empty.Empty, error) {
	srv.cfg.Delete(req.GetId())
	return &empty.Empty{}, nil
}

// Ensure handles a request to create or update a project.
func (srv *Projects) Ensure(c context.Context, req *projects.EnsureRequest) (*projects.Config, error) {
	srv.cfg.Store(req.GetId(), req.GetProject())
	return req.GetProject(), nil
}

// Get handles a request to get a project.
func (srv *Projects) Get(c context.Context, req *projects.GetRequest) (*projects.Config, error) {
	cfg, ok := srv.cfg.Load(req.GetId())
	if !ok {
		return nil, status.Errorf(codes.NotFound, "no project found with ID %q", req.GetId())
	}
	return cfg.(*projects.Config), nil
}

// List handles a request to list all projects.
func (srv *Projects) List(c context.Context, req *projects.ListRequest) (*projects.ListResponse, error) {
	rsp := &projects.ListResponse{}
	// TODO(smut): Handle page tokens.
	if req.GetPageToken() != "" {
		return rsp, nil
	}
	srv.cfg.Range(func(_, val interface{}) bool {
		rsp.Projects = append(rsp.Projects, val.(*projects.Config))
		return true
	})
	return rsp, nil
}
