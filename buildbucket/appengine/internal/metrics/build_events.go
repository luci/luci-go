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
)

// BuildCreated updates metrics for a build creation event.
func BuildCreated(ctx context.Context, b *model.Build) {
	var ua string
	for _, tag := range b.Tags {
		if k, v := strpair.Parse(tag); k == "user_agent" {
			ua = v
			break
		}
	}
	V1.BuildCountCreated.Add(ctx, 1, legacyBucketName(b.Proto.Builder), b.Proto.Builder.Builder, ua)
}

// BuildStarted updates metrics for a build start event.
func BuildStarted(ctx context.Context, b *model.Build) {
	bucket := legacyBucketName(b.Proto.Builder)
	builder := b.Proto.Builder.Builder
	isCan := b.Proto.Canary

	V1.BuildCountStarted.Add(ctx, 1, bucket, builder, isCan)
	if b.Proto.GetStartTime() != nil {
		startT := b.Proto.StartTime.AsTime()
		V1.BuildDurationScheduling.Add(
			ctx, startT.Sub(b.CreateTime).Seconds(),
			bucket, builder, "", "", "", isCan)
	}
}

// BuildCompleted updates metrics for a build completion event.
func BuildCompleted(ctx context.Context, b *model.Build) {
	bucket := legacyBucketName(b.Proto.Builder)
	builder := b.Proto.Builder.Builder
	isCan := b.Proto.Canary
	reason, failReason, cancelReason := getLegacyMetricFields(b)
	end := b.Proto.EndTime.AsTime()

	V1.BuildCountCompleted.Add(ctx, 1, bucket, builder, reason, failReason, cancelReason, isCan)
	V1.BuildDurationCycle.Add(
		ctx, end.Sub(b.CreateTime).Seconds(),
		bucket, builder, reason, failReason, cancelReason, isCan)
	if b.Proto.StartTime != nil {
		V1.BuildDurationRun.Add(
			ctx, end.Sub(b.Proto.StartTime.AsTime()).Seconds(),
			bucket, builder, reason, failReason, cancelReason, isCan)
	}
}
