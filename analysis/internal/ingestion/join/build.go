// Copyright 2024 The LUCI Authors.
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
	"sort"

	structpb "github.com/golang/protobuf/ptypes/struct"
	"google.golang.org/genproto/protobuf/field_mask"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	bbpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/common/tsmon/field"
	"go.chromium.org/luci/common/tsmon/metric"

	"go.chromium.org/luci/analysis/internal/buildbucket"
	"go.chromium.org/luci/analysis/internal/gerritchangelists"
	"go.chromium.org/luci/analysis/internal/ingestion/control"
	ctlpb "go.chromium.org/luci/analysis/internal/ingestion/control/proto"
	"go.chromium.org/luci/analysis/internal/testresults"
	pb "go.chromium.org/luci/analysis/proto/v1"
)

const (
	// userAgentTagKey is the key of the user agent tag.
	userAgentTagKey = "user_agent"
	// userAgentCQ is the value of the user agent tag, for builds started
	// by LUCI CV.
	userAgentCQ = "cq"
	// The maximum number of CLs to keep for each ingested build.
	// Avoids excessive storage consumption and calls to gerrit.
	maximumCLs = 10
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
)

// JoinBuild notifies ingestion that the given buildbucket build has finished.
// Ingestion tasks are created when all required data for this build
// (including any associated LUCI CV run and invocation) is available.
func JoinBuild(ctx context.Context, bbHost, project string, buildID int64) (processed bool, err error) {
	buildReadMask := &bbpb.BuildMask{
		Fields: &field_mask.FieldMask{
			Paths: []string{"ancestor_ids", "builder", "create_time", "infra.resultdb", "input", "output.gitiles_commit", "status", "tags"},
		},
	}

	build, err := retrieveBuild(ctx, bbHost, project, buildID, buildReadMask)
	code := status.Code(err)
	if code == codes.NotFound {
		// Build not found, handle gracefully.
		logging.Warningf(ctx, "Buildbucket build %s/%d for project %s not found (or LUCI Analysis does not have access to read it).",
			bbHost, buildID, project)
		// report metrics.
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

	hasInvocation := false
	invocationName := build.GetInfra().GetResultdb().GetInvocation()
	rdbHostName := build.GetInfra().GetResultdb().GetHostname()
	if rdbHostName != "" && invocationName != "" {
		wantInvocationName := control.BuildInvocationName(buildID)
		if invocationName != wantInvocationName {
			// If a build does not have an invocation of this form, it will never
			// be successfully joined by our implementation. It is better to
			// fail now in an obvious manner than fail later silently.
			return false, errors.Reason("build %v had unexpected ResultDB invocation (got %v, want %v)", buildID, invocationName, wantInvocationName).Err()
		}
		hasInvocation = true
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

	var changelists []*pb.Changelist
	if project == "chromeos" {
		// This path is being retained only to support Chrome OS's
		// use of LUCI Analysis Exoneration v1, in the presence
		// of inconsistently set test results sources in ResultDB.
		// Deprecate once ChromeOS fixes this up and switches
		// to exoneration v2.
		//
		// Chromium's use of LUCI Analysis Exoneration v1 does not
		// require this as it consistently sets test result sources
		// via ResultDB.
		//
		// Exoneration v2 only functions with sources set via ResultDB.
		gerritChanges := build.GetInput().GetGerritChanges()
		changelists, err = prepareChangelists(ctx, project, gerritChanges)
		if err != nil {
			return false, errors.Annotate(err, "prepare changelists").Err()
		}
	}

	commit := build.Output.GetGitilesCommit()
	if commit == nil {
		commit = build.Input.GetGitilesCommit()
	}

	result := &ctlpb.BuildResult{
		CreationTime:      timestamppb.New(build.CreateTime.AsTime()),
		Id:                buildID,
		Host:              bbHost,
		Project:           project,
		Bucket:            build.Builder.Bucket,
		Builder:           build.Builder.Builder,
		Status:            buildStatus,
		Changelists:       changelists,
		Commit:            commit,
		HasInvocation:     hasInvocation,
		ResultdbHost:      build.GetInfra().GetResultdb().Hostname,
		GardenerRotations: gardenerRotations(build.Input.GetProperties()),
	}

	if err := JoinBuildResult(ctx, buildID, project, isPresubmit, hasInvocation, result); err != nil {
		return false, errors.Annotate(err, "joining build result").Err()
	}
	// report metrics.
	buildProcessingOutcomeCounter.Add(ctx, 1, project, "success")
	return true, nil
}

func prepareChangelists(ctx context.Context, project string, gerritChanges []*bbpb.GerritChange) ([]*pb.Changelist, error) {
	// Capture the tested changelists in sorted order. This ensures that for
	// the same combination of CLs tested, the arrays are identical.
	gerritChanges = sortChangelists(gerritChanges)

	// Truncate the list of changelists to avoid storing an excessive number.
	// Apply truncation after sorting to ensure a stable set of changelists.
	if len(gerritChanges) > maximumCLs {
		gerritChanges = gerritChanges[:maximumCLs]
	}

	// Lookup the owner kind of each changelist.
	lookupRequest := make(map[gerritchangelists.Key]gerritchangelists.LookupRequest)
	for _, change := range gerritChanges {
		if err := testresults.ValidateGerritHostname(change.Host); err != nil {
			return nil, err
		}
		key := gerritchangelists.Key{
			Project: project,
			Host:    change.Host,
			Change:  change.Change,
		}
		lookupRequest[key] = gerritchangelists.LookupRequest{
			GerritProject: change.Project,
		}
	}

	ownerKinds, err := gerritchangelists.FetchOwnerKinds(ctx, lookupRequest)
	if err != nil {
		return nil, errors.Annotate(err, "retrieving gerrit owner kinds").Err()
	}

	result := make([]*pb.Changelist, 0, len(gerritChanges))
	for _, change := range gerritChanges {
		key := gerritchangelists.Key{
			Project: project,
			Host:    change.Host,
			Change:  change.Change,
		}

		result = append(result, &pb.Changelist{
			Host:      change.Host,
			Change:    change.Change,
			Patchset:  int32(change.Patchset),
			OwnerKind: ownerKinds[key],
		})
	}
	return result, nil
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

func retrieveBuild(ctx context.Context, bbHost, project string, id int64, readMask *bbpb.BuildMask) (*bbpb.Build, error) {
	bc, err := buildbucket.NewClient(ctx, bbHost, project)
	if err != nil {
		return nil, err
	}
	request := &bbpb.GetBuildRequest{
		Id:   id,
		Mask: readMask,
	}
	b, err := bc.GetBuild(ctx, request)
	switch {
	case err != nil:
		return nil, err
	}
	return b, nil
}

// gardenerRotations extracts the gardener rotations monitoring
// a buildbucket build. This is obtained from the sheriff_rotations
// build input property.
func gardenerRotations(buildInputProperties *structpb.Struct) []string {
	if buildInputProperties.GetFields() == nil {
		return nil
	}
	field := buildInputProperties.Fields["sheriff_rotations"]
	if field == nil {
		return nil
	}
	listValue := field.GetListValue()
	if listValue == nil {
		return nil
	}
	var rotations []string
	for _, value := range listValue.Values {
		rotation := value.GetStringValue()
		if rotation != "" {
			rotations = append(rotations, rotation)
		}
		// Ignore sheriff_rotation entries which are not strings.
		// This should not happen anyway.
	}
	return rotations
}

// sortChangelists sorts a slice of changelists to be in ascending
// lexicographical order by (host, change, patchset).
func sortChangelists(cls []*bbpb.GerritChange) []*bbpb.GerritChange {
	// Copy the CLs list to avoid modifying the passed arguments.
	originalCLs := cls
	cls = make([]*bbpb.GerritChange, len(originalCLs))
	copy(cls, originalCLs)

	sort.Slice(cls, func(i, j int) bool {
		// Returns true iff cls[i] is less than cls[j].
		if cls[i].Host < cls[j].Host {
			return true
		}
		if cls[i].Host == cls[j].Host && cls[i].Change < cls[j].Change {
			return true
		}
		if cls[i].Host == cls[j].Host && cls[i].Change == cls[j].Change && cls[i].Patchset < cls[j].Patchset {
			return true
		}
		return false
	})
	return cls
}
