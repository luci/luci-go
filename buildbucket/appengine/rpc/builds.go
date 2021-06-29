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
	"regexp"

	"google.golang.org/genproto/protobuf/field_mask"

	"go.chromium.org/luci/common/proto/mask"

	pb "go.chromium.org/luci/buildbucket/proto"
)

var (
	sha1Regex = regexp.MustCompile(`^[a-f0-9]{40}$`)

	// defMask is the default field mask to use for GetBuild requests.
	defMask = mask.MustFromReadMask(&pb.Build{},
		"builder",
		"canary",
		"create_time",
		"created_by",
		"critical",
		"end_time",
		"id",
		"input.experimental",
		"input.gerrit_changes",
		"input.gitiles_commit",
		"number",
		"start_time",
		"status",
		"status_details",
		"update_time",
	)
)

// TODO(crbug/1042991): Move to a common location.
func getFieldMask(fields *field_mask.FieldMask) (*mask.Mask, error) {
	if len(fields.GetPaths()) == 0 {
		return defMask, nil
	}
	return mask.FromFieldMask(fields, &pb.Build{}, false, false)
}

// getBuildsSubMask returns the sub mask for "builds.*"
func getBuildsSubMask(fields *field_mask.FieldMask) (*mask.Mask, error) {
	if len(fields.GetPaths()) == 0 {
		return defMask, nil
	}
	m, err := mask.FromFieldMask(fields, &pb.SearchBuildsResponse{}, false, false)
	if err != nil {
		return nil, err
	}
	return m.Submask("builds.*")
}

// Builds implements pb.BuildsServer.
type Builds struct{}

// Ensure Builds implements projects.ProjectsServer.
var _ pb.BuildsServer = &Builds{}

// NewBuilds returns a new pb.BuildsServer.
func NewBuilds() pb.BuildsServer {
	return &pb.DecoratedBuilds{
		Prelude:  logDetails,
		Service:  &Builds{},
		Postlude: commonPostlude,
	}
}
