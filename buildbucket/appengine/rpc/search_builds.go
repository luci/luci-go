// Copyright 2020 The LUCI Authors.
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
	"strings"

	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/grpc/appstatus"

	pb "go.chromium.org/luci/buildbucket/proto"
)

// validateSearch validates the given request.
func validateSearch(req *pb.SearchBuildsRequest) error {
	if req.GetPageSize() < 0 {
		return appstatus.Errorf(codes.InvalidArgument, "page_size cannot be negative")
	}
	pr := req.GetPredicate()
	if b := pr.GetBuilder(); b != nil {
		if err := validateBuilderID(b); err != nil {
			return err
		}
	}
	for i, ch := range pr.GetGerritChanges() {
		switch {
		case ch.Host == "":
			return appstatus.Errorf(codes.InvalidArgument, "gerrit_change[%d].host is required", i)
		case ch.Change == 0:
			return appstatus.Errorf(codes.InvalidArgument, "gerrit_change[%d].change is required", i)
		case ch.Patchset == 0:
			return appstatus.Errorf(codes.InvalidArgument, "gerrit_change[%d].patchset is required", i)
		}
	}
	if c := pr.GetOutputGitilesCommit(); c != nil {
		switch {
		case c.Host == "":
			return appstatus.Errorf(codes.InvalidArgument, "output_gitiles_commit.host is required")
		case c.Project == "":
			return appstatus.Errorf(codes.InvalidArgument, "output_gitiles_commit.project is required")
		case c.Id != "":
			switch {
			case c.Ref != "" || c.Position != 0:
				return appstatus.Errorf(codes.InvalidArgument, "output_gitiles_commit.id is mutually exclusive with (ref and position)")
			case !sha1Regex.MatchString(c.Id):
				return appstatus.Errorf(codes.InvalidArgument, "output_gitiles_commit.id must match %q", sha1Regex.String())
			}
		case c.Ref != "":
			if !strings.HasPrefix(c.Ref, "refs/") {
				return appstatus.Errorf(codes.InvalidArgument, "output_gitiles_commit.ref must match refs/.*")
			}
		default:
			return appstatus.Errorf(codes.InvalidArgument, "one of output_gitiles_commit.id or output_gitiles_commit.ref is required")
		}
	}
	if pr.GetBuild() != nil && pr.CreateTime != nil {
		return appstatus.Errorf(codes.InvalidArgument, "output_gitiles_commit.build is mutually exclusive with output_gitiles_commit.create_time")
	}
	// TODO(crbug/1042991): Potentially validate tags (buildset is validated in Python SearchBuilds).
	// TODO(crbug/1053813): Disallow empty predicate.
	return nil
}

// SearchBuilds handles a request to search for builds. Implements pb.BuildsServer.
func (*Builds) SearchBuilds(ctx context.Context, req *pb.SearchBuildsRequest) (*pb.SearchBuildsResponse, error) {
	if err := validateSearch(req); err != nil {
		return nil, err
	}
	_, err := getFieldMask(req.GetFields())
	if err != nil {
		return nil, appstatus.Errorf(codes.InvalidArgument, "invalid field mask")
	}
	// TODO(crbug/1042991): Search for the requested builds.
	return nil, nil
}
