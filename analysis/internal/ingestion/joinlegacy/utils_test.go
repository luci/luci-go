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

package joinlegacy

import (
	"context"
	"fmt"
	"strings"
	"time"

	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	bbpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/errors"
	cvv0 "go.chromium.org/luci/cv/api/v0"
	cvv1 "go.chromium.org/luci/cv/api/v1"
	rdbpb "go.chromium.org/luci/resultdb/proto/v1"

	"go.chromium.org/luci/analysis/internal/buildbucket"
	"go.chromium.org/luci/analysis/internal/cv"
	controlpb "go.chromium.org/luci/analysis/internal/ingestion/control/proto"
	pb "go.chromium.org/luci/analysis/proto/v1"

	_ "go.chromium.org/luci/analysis/internal/services/verdictingester" // Needed to ensure task class is registered.
)

// cvCreateTime is the create time assigned to CV Runs, for testing.
var cvCreateTime = time.Date(2025, time.December, 1, 2, 3, 4, 5678, time.UTC)

func ingestBuild(ctx context.Context, build *buildBuilder) error {
	builds := map[int64]*bbpb.Build{
		build.buildID: build.BuildProto(),
	}
	ctx = buildbucket.UseFakeClient(ctx, builds)

	processed, err := JoinBuild(ctx, bbHost, "buildproject", build.buildID)
	if err != nil {
		return err
	}
	if !processed {
		return errors.New("expected processed to be true")
	}
	return nil
}

func ingestFinalization(ctx context.Context, buildID int64) error {
	// Process invocation finalization.
	notification := makeInvocationFinalizedNotification(buildID, "invproject:realm")
	processed, err := JoinInvocation(ctx, notification)
	if err != nil {
		return err
	}
	if !processed {
		return errors.New("expected processed to be true")
	}
	return nil
}

func ingestCVRun(ctx context.Context, buildIDs []int64) error {
	var tryjobs []*cvv0.TryjobInvocation
	for _, id := range buildIDs {
		tryjobs = append(tryjobs, tryjob(id))
	}
	run := &cvv0.Run{
		Id:                "projects/cvproject/runs/123e4567-e89b-12d3-a456-426614174000",
		Mode:              "FULL_RUN",
		Status:            cvv0.Run_FAILED,
		Owner:             "chromium-autoroll@skia-public.iam.gserviceaccount.com",
		CreateTime:        timestamppb.New(cvCreateTime),
		TryjobInvocations: tryjobs,
		Cls: []*cvv0.GerritChange{
			{
				Host:     "chromium-review.googlesource.com",
				Change:   12345,
				Patchset: 1,
			},
		},
	}
	runs := map[string]*cvv0.Run{
		run.Id: run,
	}
	ctx = cv.UseFakeClient(ctx, runs)

	// Process presubmit run.
	message := makeCVRunPubSub(run.Id)
	project, processed, err := JoinCVRun(ctx, message)
	if err != nil {
		return err
	}
	if !processed {
		return errors.New("expected processed to be true")
	}
	if project != "cvproject" {
		return fmt.Errorf("unexpected project (got %q, want %q)", project, "cvproject")
	}
	return nil
}

type buildBuilder struct {
	buildID             int64
	hasInvocation       bool
	tags                []string
	createTime          time.Time
	ancestorIDs         []int64
	containedByAncestor bool
	gardenerRotations   []string
}

func newBuildBuilder(buildID int64) *buildBuilder {
	return &buildBuilder{
		buildID:    buildID,
		createTime: time.Date(2031, time.December, 1, 2, 3, 4, 5000, time.UTC),
	}
}

func (b *buildBuilder) WithInvocation() *buildBuilder {
	b.hasInvocation = true
	return b
}

func (b *buildBuilder) WithTags(tags []string) *buildBuilder {
	b.tags = tags
	return b
}

func (b *buildBuilder) WithCreateTime(t time.Time) *buildBuilder {
	b.createTime = t
	return b
}

func (b *buildBuilder) WithAncestorIDs(ancestorIDs []int64) *buildBuilder {
	b.ancestorIDs = ancestorIDs
	return b
}

func (b *buildBuilder) WithContainedByAncestor(contained bool) *buildBuilder {
	b.containedByAncestor = contained
	return b
}

func (b *buildBuilder) WithGardenerRotations(rotations []string) *buildBuilder {
	b.gardenerRotations = rotations
	return b
}

func (b *buildBuilder) BuildProto() *bbpb.Build {
	rdb := &bbpb.BuildInfra_ResultDB{}
	if b.hasInvocation {
		rdb.Hostname = "rdb.test.instance"
		rdb.Invocation = fmt.Sprintf("invocations/build-%v", b.buildID)
	}
	var tags []*bbpb.StringPair
	for _, t := range b.tags {
		vals := strings.SplitN(t, ":", 2)
		tags = append(tags, &bbpb.StringPair{
			Key:   vals[0],
			Value: vals[1],
		})
	}
	inputProperties := &structpb.Struct{
		Fields: make(map[string]*structpb.Value),
	}
	if len(b.gardenerRotations) > 0 {
		// Prepare build input properties like:
		// "sheriff_rotations": ["rotation1", "rotation2"].
		rotationsField := &structpb.ListValue{}
		for _, rotation := range b.gardenerRotations {
			rotationsField.Values = append(rotationsField.Values, structpb.NewStringValue(rotation))
		}
		inputProperties.Fields["sheriff_rotations"] = structpb.NewListValue(rotationsField)
	}

	return &bbpb.Build{
		Builder: &bbpb.BuilderID{
			Project: "buildproject",
			Bucket:  "bucket",
			Builder: "builder",
		},
		AncestorIds: b.ancestorIDs,
		Id:          b.buildID,
		CreateTime:  timestamppb.New(b.createTime),
		Status:      bbpb.Status_SUCCESS,
		Tags:        tags,
		Input: &bbpb.Build_Input{
			GerritChanges: []*bbpb.GerritChange{
				{
					Host:     "myproject-review.googlesource.com",
					Project:  "my/src",
					Change:   81818181,
					Patchset: 9292,
				},
				{
					Host:     "otherproject-review.googlesource.com",
					Project:  "other/src",
					Change:   71717171,
					Patchset: 1212,
				},
				{
					Host:     "do.not.query.untrusted.gerrit.instance",
					Project:  "other/src",
					Change:   92929292,
					Patchset: 5656,
				},
			},
		},
		Output: &bbpb.Build_Output{
			GitilesCommit: &bbpb.GitilesCommit{
				Host:     "coolproject-review.googlesource.com",
				Project:  "coolproject/src",
				Id:       "00112233445566778899aabbccddeeff00112233",
				Ref:      "refs/heads/branch",
				Position: 555888,
			},
		},
		Infra: &bbpb.BuildInfra{
			Resultdb: rdb,
		},
	}
}

func (b *buildBuilder) ExpectedResult() *controlpb.BuildResult {
	var rdbHost string
	if b.hasInvocation {
		rdbHost = "rdb.test.instance"
	}

	return &controlpb.BuildResult{
		Host:         bbHost,
		Id:           b.buildID,
		CreationTime: timestamppb.New(b.createTime),
		Project:      "buildproject",
		Bucket:       "bucket",
		Builder:      "builder",
		Status:       pb.BuildStatus_BUILD_STATUS_SUCCESS,
		Commit: &bbpb.GitilesCommit{
			Host:     "coolproject-review.googlesource.com",
			Project:  "coolproject/src",
			Id:       "00112233445566778899aabbccddeeff00112233",
			Ref:      "refs/heads/branch",
			Position: 555888,
		},
		HasInvocation:        b.hasInvocation,
		ResultdbHost:         rdbHost,
		IsIncludedByAncestor: b.containedByAncestor,
		GardenerRotations:    b.gardenerRotations,
	}
}

func makeCVRunPubSub(runID string) *cvv1.PubSubRun {
	return &cvv1.PubSubRun{
		Id:       runID,
		Status:   cvv1.Run_SUCCEEDED,
		Hostname: "cvhost",
	}
}

func makeInvocationFinalizedNotification(buildID int64, realm string) *rdbpb.InvocationFinalizedNotification {
	return &rdbpb.InvocationFinalizedNotification{
		Invocation: fmt.Sprintf("invocations/build-%v", buildID),
		Realm:      realm,
	}
}
