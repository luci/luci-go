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
	"fmt"
	"math"

	"go.chromium.org/luci/common/data/strpair"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/sync/parallel"
	"go.chromium.org/luci/common/tsmon/distribution"
	"go.chromium.org/luci/common/tsmon/field"
	"go.chromium.org/luci/common/tsmon/metric"
	"go.chromium.org/luci/common/tsmon/types"
	"go.chromium.org/luci/server/auth"

	"go.chromium.org/luci/buildbucket/appengine/model"
	"go.chromium.org/luci/buildbucket/appengine/tasks"
	taskdefs "go.chromium.org/luci/buildbucket/appengine/tasks/defs"
	pb "go.chromium.org/luci/buildbucket/proto"
)

var (
	// A common set of field definitions for build metrics.
	fieldDefs = map[string]field.Field{
		"bucket":             field.String("bucket"),
		"builder":            field.String("builder"),
		"canary":             field.Bool("canary"),
		"cancelation_reason": field.String("cancelation_reason"),
		"failure_reason":     field.String("failure_reason"),
		"result":             field.String("result"),
		"user_agent":         field.String("user_agent"),
	}

	buildCountCreated = metric.NewCounter(
		"buildbucket/builds/created",
		"Build creation", nil,
		bFields("user_agent")...,
	)
	buildCountStarted = metric.NewCounter(
		"buildbucket/builds/started",
		"Build start", nil,
		bFields("canary")...,
	)
	buildCountCompleted = metric.NewCounter(
		"buildbucket/builds/completed",
		"Build completion, including success, failure and cancellation", nil,
		bFields("result", "failure_reason", "cancelation_reason", "canary")...,
	)

	buildDurationCycle = newbuildDurationMetric(
		"buildbucket/builds/cycle_durations",
		"Duration between build creation and completion",
	)
	buildDurationRun = newbuildDurationMetric(
		"buildbucket/builds/run_durations",
		"Duration between build start and completion",
	)
	buildDurationScheduling = newbuildDurationMetric(
		"buildbucket/builds/scheduling_durations",
		"Duration between build creation and start",
	)
)

func bFields(extraFields ...string) []field.Field {
	fs := make([]field.Field, 2+len(extraFields))
	fs[0], fs[1] = fieldDefs["bucket"], fieldDefs["builder"]
	for i, n := range extraFields {
		f, ok := fieldDefs[n]
		if !ok {
			panic(fmt.Sprintf("unknown build field %q", n))
		}
		fs[i+2] = f
	}
	return fs
}

func newbuildDurationMetric(name, description string, extraFields ...string) metric.CumulativeDistribution {
	fs := []string{"result", "failure_reason", "cancelation_reason", "canary"}
	return metric.NewCumulativeDistribution(
		name, description, &types.MetricMetadata{Units: types.Seconds},
		// Bucketer for 1s..48h range
		distribution.GeometricBucketer(math.Pow(10, 0.053), 100),
		bFields(append(fs, extraFields...)...)...,
	)
}

func getLegacyMetricFields(b *model.Build) (result, failureR, cancelationR string) {
	// The default values are "" instead of UNSET for backwards compatibility.
	switch b.Status {
	case pb.Status_SCHEDULED:
	case pb.Status_STARTED:
	case pb.Status_SUCCESS:
		result = model.Success.String()
	case pb.Status_FAILURE:
		result = model.Failure.String()
		failureR = model.BuildFailure.String()
	case pb.Status_INFRA_FAILURE:
		if b.Proto.StatusDetails.GetTimeout() != nil {
			result = model.Canceled.String()
			cancelationR = model.TimeoutCanceled.String()
		} else {
			result = model.Failure.String()
			failureR = model.InfraFailure.String()
		}
	case pb.Status_CANCELED:
		result = model.Canceled.String()
		cancelationR = model.ExplicitlyCanceled.String()
	default:
		panic(fmt.Sprintf("getLegacyMetricFields: invalid status %q", b.Status))
	}
	return
}

func buildCreated(ctx context.Context, b *model.Build) {
	var ua string
	for _, tag := range b.Tags {
		if k, v := strpair.Parse(tag); k == "user_agent" {
			ua = v
			break
		}
	}
	buildCountCreated.Add(ctx, 1, legacyBucketName(b.Proto.Builder), b.Proto.Builder.Builder, ua)
}

func buildStarted(ctx context.Context, b *model.Build) {
	logging.Infof(ctx, "Build %d: started", b.ID)
	buildCountStarted.Add(
		ctx, 1,
		legacyBucketName(b.Proto.Builder), b.Proto.Builder.Builder, b.Proto.Canary,
	)
	if b.Proto.GetStartTime() != nil {
		startT := b.Proto.StartTime.AsTime()
		buildDurationScheduling.Add(
			ctx, startT.Sub(b.CreateTime).Seconds(),
			legacyBucketName(b.Proto.Builder), b.Proto.Builder.Builder, "", "", "", b.Proto.Canary,
		)
	}
}

func buildStarting(ctx context.Context, b *model.Build) error {
	return notifyPubSub(ctx, b)
}

func buildCompleted(ctx context.Context, b *model.Build) {
	r, fr, cr := getLegacyMetricFields(b)
	logging.Infof(ctx, "Build %d: completed by %q with status %q", b.ID, auth.CurrentIdentity(ctx), r)
	buildCountCompleted.Add(
		ctx, 1,
		legacyBucketName(b.Proto.Builder), b.Proto.Builder.Builder, r, fr, cr, b.Proto.Canary)

	endT := b.Proto.EndTime.AsTime()
	buildDurationCycle.Add(
		ctx, endT.Sub(b.CreateTime).Seconds(),
		legacyBucketName(b.Proto.Builder), b.Proto.Builder.Builder, r, fr, cr, b.Proto.Canary,
	)
	if b.Proto.StartTime != nil {
		startT := b.Proto.StartTime.AsTime()
		buildDurationRun.Add(
			ctx, endT.Sub(startT).Seconds(),
			legacyBucketName(b.Proto.Builder), b.Proto.Builder.Builder, r, fr, cr, b.Proto.Canary,
		)
	}
}

func buildCompleting(ctx context.Context, b *model.Build) error {
	bqTask := &taskdefs.ExportBigQuery{BuildId: b.ID}
	invTask := &taskdefs.FinalizeResultDB{BuildId: b.ID}
	return parallel.FanOutIn(func(tks chan<- func() error) {
		tks <- func() error { return notifyPubSub(ctx, b) }
		tks <- func() error { return tasks.ExportBigQuery(ctx, bqTask) }
		tks <- func() error { return tasks.FinalizeResultDB(ctx, invTask) }
	})
}

// legacyBucketName returns the V1 luci bucket name.
// e.g., "luci.chromium.try".
func legacyBucketName(bid *pb.BuilderID) string {
	return fmt.Sprintf("luci.%s.%s", bid.Project, bid.Bucket)
}
