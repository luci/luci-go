// Copyright 2018 The LUCI Authors.
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

package buildbucket

import (
	"time"

	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors/errtag"

	pb "go.chromium.org/luci/buildbucket/proto"
)

// This file contains helper functions for pb package.
// TODO(nodir): move existing helpers from pb to this file.

// BuildbucketTokenHeader is the name of gRPC metadata header indicating a
// buildbucket token.
//
// Tokens include an authenticatable purpose field, we only need one header for
// all incoming RPCs.
const BuildbucketTokenHeader = "x-buildbucket-token"

// BuildTokenHeader is the old name of the gRPC metadata header indicating the build
// token (see BuildSecrets.BuildToken). It is still used by `kitchen`, but is
// otherwise deprecated in favor of BuildbucketTokenHeader for all uses.
//
// DEPRECATED
const BuildTokenHeader = "x-build-token"

// DummyBuildbucketToken is the dummy token for led builds.
const DummyBuildbucketToken = "dummy token"

// MinUpdateBuildInterval is the minimum interval bbagent should call UpdateBuild.
const MinUpdateBuildInterval = 30 * time.Second

// Well-known experiment strings.
//
// See the Builder.experiments field documentation.
const (
	ExperimentBackendAlt          = "luci.buildbucket.backend_alt"
	ExperimentBackendGo           = "luci.buildbucket.backend_go"
	ExperimentBBAgent             = "luci.buildbucket.use_bbagent"
	ExperimentBBAgentDownloadCipd = "luci.buildbucket.agent.cipd_installation"
	ExperimentBBAgentGetBuild     = "luci.buildbucket.bbagent_getbuild"
	ExperimentBBCanarySoftware    = "luci.buildbucket.canary_software"
	ExperimentBqExporterGo        = "luci.buildbucket.bq_exporter_go"
	ExperimentNonProduction       = "luci.non_production"
	ExperimentParentTracking      = "luci.buildbucket.parent_tracking"
	ExperimentWaitForCapacity     = "luci.buildbucket.wait_for_capacity_in_slices"
)

var (
	// DuplicateTask means the backend has created multiple tasks for the same build.
	// After the build has associated with one of those tasks (either by StartBuild or RegisterBuildTask),
	// requests of associating other tasks with the build will fail with this error.
	DuplicateTask = errtag.Make("duplicate_backend_task", true)
	// TaskWithCollidedRequestID the backend has created multiple tasks for the same build,
	// and two tasks among them have generated the same request_id for calling RegisterBuildTask.
	TaskWithCollidedRequestID = errtag.Make("task_with_collided_request_id", true)
)

var (
	// DisallowedAppendTagKeys is the set of tag keys which cannot be set via
	// UpdateBuild. Clients calling UpdateBuild must strip these before making
	// the request.
	DisallowedAppendTagKeys = stringset.NewFromSlice("build_address", "buildset", "builder")
)

// WithoutDisallowedTagKeys returns tags whose key are not in
// `DisallowedAppendTagKeys`.
func WithoutDisallowedTagKeys(tags []*pb.StringPair) []*pb.StringPair {
	if len(tags) == 0 {
		return tags
	}
	ret := make([]*pb.StringPair, 0, len(tags))
	for _, tag := range tags {
		if !DisallowedAppendTagKeys.Has(tag.Key) {
			ret = append(ret, tag)
		}
	}
	return ret
}
