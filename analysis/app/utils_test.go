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

package app

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	bbpb "go.chromium.org/luci/buildbucket/proto"
	bbv1 "go.chromium.org/luci/common/api/buildbucket/buildbucket/v1"
	"go.chromium.org/luci/common/errors"
	cvv0 "go.chromium.org/luci/cv/api/v0"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/analysis/internal/buildbucket"
	"go.chromium.org/luci/analysis/internal/cv"
	_ "go.chromium.org/luci/analysis/internal/services/resultingester" // Needed to ensure task class is registered.
)

// cvCreateTime is the create time assigned to CV Runs, for testing.
var cvCreateTime = time.Date(2025, time.December, 1, 2, 3, 4, 5678, time.UTC)

func ingestBuild(ctx context.Context, build *buildBuilder) error {
	builds := map[int64]*bbpb.Build{
		build.buildID: build.BuildProto(),
	}
	ctx = buildbucket.UseFakeClient(ctx, builds)

	r := &http.Request{Body: makeBBReq(build.BuildPubSubMessage(), bbHost)}
	project, processed, err := bbPubSubHandlerImpl(ctx, r)
	if err != nil {
		return err
	}
	if !processed {
		return errors.New("expected processed to be true")
	}
	if project != "buildproject" {
		return fmt.Errorf("unexpected project (got %q, want %q)", project, "buildproject")
	}
	return nil
}

func ingestFinalization(ctx context.Context, buildID int64) error {
	// Process invocation finalization.
	r := &http.Request{Body: makeInvocationFinalizedReq(buildID, "invproject:realm")}
	project, processed, err := invocationFinalizedPubSubHandlerImpl(ctx, r)
	if err != nil {
		return err
	}
	if !processed {
		return errors.New("expected processed to be true")
	}
	if project != "invproject" {
		return fmt.Errorf("unexpected project (got %q, want %q)", project, "invproject")
	}
	return nil
}

func ingestCVRun(ctx context.Context, buildIDs []int64) error {
	var tryjobs []*cvv0.Tryjob
	for _, id := range buildIDs {
		tryjobs = append(tryjobs, tryjob(id))
	}
	run := &cvv0.Run{
		Id:         "projects/cvproject/runs/123e4567-e89b-12d3-a456-426614174000",
		Mode:       "FULL_RUN",
		Status:     cvv0.Run_FAILED,
		Owner:      "chromium-autoroll@skia-public.iam.gserviceaccount.com",
		CreateTime: timestamppb.New(cvCreateTime),
		Tryjobs:    tryjobs,
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
	r := &http.Request{Body: makeCVRunReq(run.Id)}
	project, processed, err := cvPubSubHandlerImpl(ctx, r)
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
	buildID       int64
	hasInvocation bool
	tags          []string
	createTime    time.Time
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
	return &bbpb.Build{
		Builder: &bbpb.BuilderID{
			Project: "buildproject",
			Bucket:  "bucket",
			Builder: "builder",
		},
		Id:         b.buildID,
		CreateTime: timestamppb.New(b.createTime),
		Tags:       tags,
		Infra: &bbpb.BuildInfra{
			Resultdb: rdb,
		},
	}
}

func (b *buildBuilder) BuildPubSubMessage() bbv1.LegacyApiCommonBuildMessage {
	return bbv1.LegacyApiCommonBuildMessage{
		Project:   "buildproject",
		Bucket:    "bucket",
		Id:        b.buildID,
		Status:    bbv1.StatusCompleted,
		CreatedTs: bbv1.FormatTimestamp(b.createTime),
		Tags:      b.tags,
	}
}
