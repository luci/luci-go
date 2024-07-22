// Copyright 2021 The LUCI Authors.
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

package metrics

import (
	"context"

	"go.chromium.org/luci/common/data/strpair"

	"go.chromium.org/luci/buildbucket/appengine/model"
	pb "go.chromium.org/luci/buildbucket/proto"
)

// BuildCreated updates metrics for a build creation event.
func BuildCreated(ctx context.Context, b *model.Build) {
	bpb := b.Proto.Builder

	// v1
	ua := ""
	for _, tag := range b.Tags {
		if k, v := strpair.Parse(tag); k == "user_agent" {
			ua = v
			break
		}
	}
	V1.BuildCountCreated.Add(ctx, 1, legacyBucketName(bpb.Project, bpb.Bucket), bpb.Builder, ua)

	// V2
	exps := b.ExperimentsString()
	ctx = WithBuilder(ctx, bpb.Project, bpb.Bucket, bpb.Builder)
	V2.BuildCountCreated.Add(ctx, 1, exps)

	// Custom Metrics
	cmValues := map[pb.CustomMetricBase]any{
		pb.CustomMetricBase_CUSTOM_METRIC_BASE_CREATED: int64(1),
	}
	reportToCustomMetrics(ctx, b, cmValues)
}

// BuildStarted updates metrics for a build start event.
func BuildStarted(ctx context.Context, b *model.Build) {
	bp, bpb := b.Proto, b.Proto.Builder
	var schD float64

	// v1
	legacyBucket := legacyBucketName(bpb.Project, bpb.Bucket)
	V1.BuildCountStarted.Add(ctx, 1, legacyBucket, bpb.Builder, bp.Canary)
	if bp.StartTime != nil {
		schD = bp.StartTime.AsTime().Sub(b.CreateTime).Seconds()
		V1.BuildDurationScheduling.Add(ctx, schD, legacyBucket, bpb.Builder, "", "", "", bp.Canary)
	}

	// v2
	ctx = WithBuilder(ctx, bpb.Project, bpb.Bucket, bpb.Builder)
	exps := b.ExperimentsString()
	V2.BuildCountStarted.Add(ctx, 1, exps)
	if bp.StartTime != nil {
		V2.BuildDurationScheduling.Add(ctx, schD, exps)
	}

	// Custom Metrics
	cmValues := map[pb.CustomMetricBase]any{
		pb.CustomMetricBase_CUSTOM_METRIC_BASE_STARTED: int64(1),
	}
	if bp.StartTime != nil {
		cmValues[pb.CustomMetricBase_CUSTOM_METRIC_BASE_SCHEDULING_DURATIONS] = schD
	}
	reportToCustomMetrics(ctx, b, cmValues)
}

// BuildCompleted updates metrics for a build completion event.
func BuildCompleted(ctx context.Context, b *model.Build) {
	bp, bpb := b.Proto, b.Proto.Builder
	cycleD := bp.EndTime.AsTime().Sub(b.CreateTime).Seconds()
	var runD float64

	// v1
	legacyBucket := legacyBucketName(bpb.Project, bpb.Bucket)
	_, reason, failReason, cancelReason := getLegacyMetricFields(b)
	V1.BuildCountCompleted.Add(
		ctx, 1, legacyBucket, bpb.Builder,
		reason, failReason, cancelReason, bp.Canary)
	V1.BuildDurationCycle.Add(
		ctx, cycleD, legacyBucket, bpb.Builder,
		reason, failReason, cancelReason, bp.Canary)
	if bp.StartTime != nil {
		runD = bp.EndTime.AsTime().Sub(bp.StartTime.AsTime()).Seconds()
		V1.BuildDurationRun.Add(
			ctx, runD, legacyBucket, bpb.Builder,
			reason, failReason, cancelReason, bp.Canary)
	}

	// v2
	ctx = WithBuilder(ctx, bpb.Project, bpb.Bucket, bpb.Builder)
	exps := b.ExperimentsString()
	status := pb.Status_name[int32(b.Status)]
	V2.BuildCountCompleted.Add(ctx, 1, status, exps)
	V2.BuildDurationCycle.Add(ctx, cycleD, status, exps)
	if b.Proto.StartTime != nil {
		V2.BuildDurationRun.Add(ctx, runD, status, exps)
	}

	// Custom Metrics
	cmValues := map[pb.CustomMetricBase]any{
		pb.CustomMetricBase_CUSTOM_METRIC_BASE_COMPLETED:       int64(1),
		pb.CustomMetricBase_CUSTOM_METRIC_BASE_CYCLE_DURATIONS: cycleD,
	}
	if b.Proto.StartTime != nil {
		cmValues[pb.CustomMetricBase_CUSTOM_METRIC_BASE_RUN_DURATIONS] = runD
	}
	reportToCustomMetrics(ctx, b, cmValues)
}

// ExpiredLeaseReset updates metrics for an expired lease reset.
func ExpiredLeaseReset(ctx context.Context, b *model.Build) {
	legacyBucket := legacyBucketName(b.Proto.Builder.Project, b.Proto.Builder.Bucket)
	status, _, _, _ := getLegacyMetricFields(b)
	V1.ExpiredLeaseReset.Add(ctx, 1, legacyBucket, b.Proto.Builder.Builder, status)
}
