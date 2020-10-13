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

	"go.chromium.org/luci/cipd/common"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/grpc/appstatus"

	"go.chromium.org/luci/buildbucket/appengine/internal/perm"
	pb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/buildbucket/protoutil"
)

// validateExecutable validates the given executable.
func validateExecutable(exe *pb.Executable) error {
	var err error
	switch {
	case exe.GetCipdPackage() != "":
		return errors.Reason("cipd_package must not be specified").Err()
	case exe.GetCipdVersion() != "" && teeErr(common.ValidateInstanceVersion(exe.CipdVersion), &err) != nil:
		return errors.Annotate(err, "cipd_version").Err()
	default:
		return nil
	}
}

// validateSchedule validates the given request.
func validateSchedule(req *pb.ScheduleBuildRequest) error {
	var err error
	switch {
	case strings.Contains(req.GetRequestId(), "/"):
		return errors.Reason("request_id cannot contain '/'").Err()
	case req.GetBuilder() == nil && req.GetTemplateBuildId() == 0:
		return errors.Reason("builder or template_build_id is required").Err()
	case req.Builder != nil && teeErr(protoutil.ValidateRequiredBuilderID(req.Builder), &err) != nil:
		return errors.Annotate(err, "builder").Err()
	case teeErr(validateExecutable(req.Exe), &err) != nil:
		return errors.Annotate(err, "exe").Err()
	case req.GitilesCommit != nil && teeErr(validateCommitWithRef(req.GitilesCommit), &err) != nil:
		return errors.Annotate(err, "gitiles_commit").Err()
	case teeErr(validateTags(req.Tags, TagNew), &err) != nil:
		return errors.Annotate(err, "tags").Err()
	case req.Priority < 0 || req.Priority > 255:
		return errors.Reason("priority must be in [0, 255]").Err()
	}

	// TODO(crbug/1042991): Validate Properties, Gerrit Changes, Dimensions, Notify.
	return nil
}

// ScheduleBuild handles a request to schedule a build. Implements pb.BuildsServer.
func (*Builds) ScheduleBuild(ctx context.Context, req *pb.ScheduleBuildRequest) (*pb.Build, error) {
	if err := validateSchedule(req); err != nil {
		return nil, appstatus.BadRequest(err)
	}

	// TODO(crbug/1042991): Ensure Builder, Project are set (i.e. load TemplateBuildId if specified).
	if err := perm.HasInBucket(ctx, perm.BuildsAdd, req.Builder.Project, req.Builder.Bucket); err != nil {
		return nil, err
	}

	return nil, nil
}
