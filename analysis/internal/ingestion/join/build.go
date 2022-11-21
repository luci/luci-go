// Copyright 2022 The LUCI Authors.
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

package join

import (
	"context"
	"fmt"
	"strings"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/common/tsmon/field"
	"go.chromium.org/luci/common/tsmon/metric"
	"google.golang.org/genproto/protobuf/field_mask"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/analysis/internal/buildbucket"
	"go.chromium.org/luci/analysis/internal/ingestion/control"
	ctlpb "go.chromium.org/luci/analysis/internal/ingestion/control/proto"
	"go.chromium.org/luci/analysis/internal/resultdb"
	"go.chromium.org/luci/analysis/internal/testresults"
	pb "go.chromium.org/luci/analysis/proto/v1"
	bbpb "go.chromium.org/luci/buildbucket/proto"
)

const (
	// userAgentTagKey is the key of the user agent tag.
	userAgentTagKey = "user_agent"
	// userAgentCQ is the value of the user agent tag, for builds started
	// by LUCI CV.
	userAgentCQ = "cq"
)

var (
	buildProcessingOutcomeCounter = metric.NewCounter(
		"analysis/ingestion/pubsub/buildbucket_build_processing_outcome",
		"The number of buildbucket builds processed by LUCI Analysis,"+
			" by processing outcome (e.g. success, permission denied).",
		nil,
		// The LUCI Project.
		field.String("project"),
		// "success", "permission_denied".
		field.String("status"))

	ancestorCounter = metric.NewCounter(
		"analysis/ingestion/ancestor_build_status",
		"The status retrieving ancestor builds in ingestion tasks, by build project.",
		nil,
		// The LUCI Project.
		field.String("project"),
		// "no_bb_access_to_ancestor",
		// "no_resultdb_invocation_on_ancestor",
		// "ok".
		field.String("ancestor_status"))
)

// JoinBuild notifies ingestion that the given buildbucket build has finished.
// Ingestion tasks are created for buildbucket builds when all required data
// for a build (including any associated LUCI CV run) is available.
func JoinBuild(ctx context.Context, bbHost, project string, buildID int64) (processed bool, err error) {
	buildReadMask := &field_mask.FieldMask{
		Paths: []string{"ancestor_ids", "builder", "create_time", "infra.resultdb", "input", "output", "status", "tags"},
	}
	build, err := retrieveBuild(ctx, bbHost, project, buildID, buildReadMask)
	code := status.Code(err)
	if code == codes.NotFound {
		// Build not found, handle gracefully.
		logging.Warningf(ctx, "Buildbucket build %s/%d for project %s not found (or LUCI Analysis does not have access to read it).",
			bbHost, buildID, project)
		buildProcessingOutcomeCounter.Add(ctx, 1, project, "permission_denied")
		return false, nil
	}
	if err != nil {
		return false, transient.Tag.Apply(errors.Annotate(err, "retrieving buildbucket build").Err())
	}

	if build.CreateTime.GetSeconds() <= 0 {
		return false, errors.New("build did not have create time specified")
	}

	userAgents := extractTagValues(build.Tags, userAgentTagKey)
	isPresubmit := len(userAgents) == 1 && userAgents[0] == userAgentCQ

	id := control.BuildID(bbHost, buildID)

	hasInvocation := false
	invocationName := build.GetInfra().GetResultdb().GetInvocation()
	rdbHostName := build.GetInfra().GetResultdb().GetHostname()
	if rdbHostName != "" && invocationName != "" {
		wantInvocationName := control.BuildInvocationName(buildID)
		if invocationName != wantInvocationName {
			// If a build does not have an invocation of this form, it will never
			// be successfully joined by our implementation. It is better to
			// fail now in an obvious manner than fail later silently.
			return false, errors.Reason("build %v had unexpected ResultDB invocation (got %v, want %v)", id, invocationName, wantInvocationName).Err()
		}
		hasInvocation = true
	}

	isIncludedByAncestor := false
	if len(build.AncestorIds) > 0 && hasInvocation {
		// If the build has an ancestor build, see if its immediate
		// ancestor is accessible by LUCI Analysis and has a ResultDB
		// invocation (likely indicating it includes the test results
		// from this build).
		ancestorBuildID := build.AncestorIds[len(build.AncestorIds)-1]
		var err error
		isIncludedByAncestor, err = includedByAncestorBuild(ctx, buildID, ancestorBuildID, rdbHostName, project)
		if err != nil {
			return false, transient.Tag.Apply(err)
		}
	}

	var buildStatus pb.BuildStatus
	switch build.Status {
	case bbpb.Status_CANCELED:
		buildStatus = pb.BuildStatus_BUILD_STATUS_CANCELED
	case bbpb.Status_SUCCESS:
		buildStatus = pb.BuildStatus_BUILD_STATUS_SUCCESS
	case bbpb.Status_FAILURE:
		buildStatus = pb.BuildStatus_BUILD_STATUS_FAILURE
	case bbpb.Status_INFRA_FAILURE:
		buildStatus = pb.BuildStatus_BUILD_STATUS_INFRA_FAILURE
	default:
		return false, fmt.Errorf("build has unknown status: %v", build.Status)
	}

	gerritChanges := build.GetInput().GetGerritChanges()
	changelists := make([]*ctlpb.BuildResult_Changelist, 0, len(gerritChanges))
	for _, change := range gerritChanges {
		if !strings.HasSuffix(change.Host, testresults.GerritHostnameSuffix) {
			return false, fmt.Errorf(`gerrit host %q does not end in expected suffix %q`, change.Host, testresults.GerritHostnameSuffix)
		}
		host := strings.TrimSuffix(change.Host, testresults.GerritHostnameSuffix)
		changelists = append(changelists, &ctlpb.BuildResult_Changelist{
			Host:     host,
			Change:   change.Change,
			Patchset: change.Patchset,
		})
	}

	commit := build.Output.GetGitilesCommit()
	if commit == nil {
		commit = build.Input.GetGitilesCommit()
	}

	result := &ctlpb.BuildResult{
		CreationTime:         timestamppb.New(build.CreateTime.AsTime()),
		Id:                   buildID,
		Host:                 bbHost,
		Project:              project,
		Builder:              build.Builder.Builder,
		Status:               buildStatus,
		Changelists:          changelists,
		Commit:               commit,
		HasInvocation:        hasInvocation,
		ResultdbHost:         build.GetInfra().GetResultdb().Hostname,
		IsIncludedByAncestor: isIncludedByAncestor,
	}
	if err := JoinBuildResult(ctx, id, project, isPresubmit, hasInvocation, result); err != nil {
		return false, errors.Annotate(err, "joining build result").Err()
	}
	buildProcessingOutcomeCounter.Add(ctx, 1, project, "success")
	return true, nil
}

func includedByAncestorBuild(ctx context.Context, buildID, ancestorBuildID int64, rdbHost string, project string) (bool, error) {
	ancestorInvName := control.BuildInvocationName(ancestorBuildID)

	// The ancestor build may not be in the same project as the build we are
	// considering ingesting. We cannot use project-scoped credentials,
	// and instead must use privileged access granted to us. We should
	// be careful not to leak information about this invocation to the
	// project we are ingesting (except for the inclusion of the child
	// in it as that is unavoidable for the purposes of implementing
	// only-once ingestion).
	rc, err := resultdb.NewPrivilegedClient(ctx, rdbHost)
	if err != nil {
		return false, transient.Tag.Apply(err)
	}
	ancestorInv, err := rc.GetInvocation(ctx, ancestorInvName)
	code := status.Code(err)
	if code == codes.NotFound || code == codes.PermissionDenied {
		logging.Warningf(ctx, "Ancestor build ResultDB Invocation %s/%d for project %s not found (or LUCI Analysis does not have access to read it).",
			rdbHost, ancestorBuildID, project)
		// Invocation on the ancestor build not found or permission denied.
		// Continue ingestion of this build.
		ancestorCounter.Add(ctx, 1, project, "resultdb_invocation_on_ancestor_not_found")
		return false, nil
	}
	if err != nil {
		return false, transient.Tag.Apply(errors.Annotate(err, "fetch ancestor build ResultDB invocation").Err())
	}

	containsThisBuild := false

	buildInvocation := control.BuildInvocationName(buildID)
	for _, inv := range ancestorInv.IncludedInvocations {
		if inv == buildInvocation {
			containsThisBuild = true
		}
	}

	if !containsThisBuild {
		// The ancestor build's invocation does not contain the ResultDB
		// invocation of this build. Continue ingestion of this build.
		ancestorCounter.Add(ctx, 1, project, "resultdb_invocation_on_ancestor_does_not_contain")
		return false, nil
	}

	// The ancestor build also has a ResultDB invocation, and it
	// contains this invocation. We will ingest the ancestor build
	// only to avoid ingesting the same test results multiple times.
	ancestorCounter.Add(ctx, 1, project, "ok")
	return true, nil
}

func extractTagValues(tags []*bbpb.StringPair, key string) []string {
	var values []string
	for _, tag := range tags {
		if tag.Key == key {
			values = append(values, tag.Value)
		}
	}
	return values
}

func retrieveBuild(ctx context.Context, bbHost, project string, id int64, readMask *field_mask.FieldMask) (*bbpb.Build, error) {
	bc, err := buildbucket.NewClient(ctx, bbHost, project)
	if err != nil {
		return nil, err
	}
	request := &bbpb.GetBuildRequest{
		Id: id,
		Mask: &bbpb.BuildMask{
			Fields: readMask,
		},
	}
	b, err := bc.GetBuild(ctx, request)
	switch {
	case err != nil:
		return nil, err
	}
	return b, nil
}
