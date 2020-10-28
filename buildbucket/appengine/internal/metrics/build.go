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

package metrics

import (
	"context"
	"fmt"
	"math"

	"github.com/golang/protobuf/ptypes"

	"go.chromium.org/luci/common/data/strpair"
	"go.chromium.org/luci/common/tsmon/distribution"
	"go.chromium.org/luci/common/tsmon/field"
	"go.chromium.org/luci/common/tsmon/metric"
	"go.chromium.org/luci/common/tsmon/types"

	"go.chromium.org/luci/buildbucket/appengine/model"
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
		"status":             field.String("status"),
		"user_agent":         field.String("user_agent"),
	}

	// BuildCounts tracks the occurrences of build events.
	BuildCounts = struct {
		Created              metric.Counter
		Started              metric.Counter
		Completed            metric.Counter
		Leased               metric.Counter
		LeaseExpired         metric.Counter
		LeaseExtensionFailed metric.Counter
	}{
		metric.NewCounter(
			"buildbucket/builds/Created", "Build creation", nil,
			bFields("bucket", "builder", "user_agent")...,
		),
		metric.NewCounter(
			"buildbucket/builds/Started", "Build start", nil,
			bFields("bucket", "builder", "canary")...,
		),
		metric.NewCounter(
			"buildbucket/builds/Completed", "Build completion, including success, failure and cancellation", nil,
			bFields("bucket", "builder", "result", "failure_reason", "cancelation_reason", "canary")...,
		),
		metric.NewCounter(
			"buildbucket/builds/leases", "Successful build leases or lease extensions", nil,
			bFields("bucket", "builder")...,
		),
		metric.NewCounter(
			"buildbucket/builds/lease_expired", "Build lease expirations", nil,
			bFields("bucket", "builder", "status")...,
		),
		metric.NewCounter(
			"buildbucket/builds/heartbeats", "Failures to extend a build lease", nil,
		),
	}

	// BuildDurations tracks the durations of build status transitions
	BuildDurations = struct {
		Cycle      metric.CumulativeDistribution
		Run        metric.CumulativeDistribution
		Scheduling metric.CumulativeDistribution
	}{
		bdMetric("buildbucket/builds/cycle_durations", "Duration between build creation and completion"),
		bdMetric("buildbucket/builds/run_durations", "Duration between build start and completion"),
		bdMetric("buildbucket/builds/scheduling_durations", "Duration between build creation and start"),
	}
)

func bFields(names ...string) []field.Field {
	fs := make([]field.Field, len(names))
	for i, n := range names {
		f, ok := fieldDefs[n]
		if !ok {
			panic(fmt.Sprintf("unknown build field %q", n))
		}
		fs[i] = f
	}
	return fs
}

func bdMetric(path string, desc string) metric.CumulativeDistribution {
	return metric.NewCumulativeDistribution(
		path, desc, &types.MetricMetadata{Units: types.Seconds},
		// Bucketer for 1s..48h range
		distribution.GeometricBucketer(math.Pow(10, 0.053), 100),
		bFields("bucket", "builder", "result", "failure_reason", "cancelation_reason", "canary")...,
	)
}

func getLegacyMetricFields(b *model.Build) (status, result, failureR, cancelationR string) {
	// The default values are "" instead of UNSET for backwards compatibility.
	switch b.Status {
	case pb.Status_SCHEDULED:
		status = model.Scheduled.String()
	case pb.Status_STARTED:
		status = model.Started.String()
	case pb.Status_SUCCESS:
		status = model.Completed.String()
		result = model.Success.String()
	case pb.Status_FAILURE:
		status = model.Completed.String()
		result = model.Failure.String()
		failureR = model.BuildFailure.String()
	case pb.Status_INFRA_FAILURE:
		status = model.Completed.String()
		if b.Proto.StatusDetails.GetTimeout() != nil {
			result = model.Canceled.String()
			cancelationR = model.TimeoutCanceled.String()
		} else {
			result = model.Failure.String()
			failureR = model.InfraFailure.String()
		}
	case pb.Status_CANCELED:
		status = model.Completed.String()
		result = model.Canceled.String()
		cancelationR = model.ExplicitlyCanceled.String()
	}
	return
}

// BuildCreated reports a Build creation.
func BuildCreated(ctx context.Context, b *model.Build) {
	var user_agent string
	for _, tag := range b.Tags {
		if k, v := strpair.Parse(tag); k == "user_agent" {
			user_agent = v
			break
		}
	}
	BuildCounts.Created.Add(ctx, 1, b.BucketID, b.BuilderID, user_agent)
}

// BuildStarted reports a Build start.
func BuildStarted(ctx context.Context, b *model.Build) {
	_, r, fr, cr := getLegacyMetricFields(b)
	BuildCounts.Started.Add(ctx, 1, b.BucketID, b.BuilderID, b.Canary)
	if b.Proto.GetStartTime() != nil {
		startT, _ := ptypes.Timestamp(b.Proto.StartTime)
		BuildDurations.Scheduling.Add(
			ctx, startT.Sub(b.CreateTime).Seconds(), b.BucketID, b.BuilderID, r, fr, cr, b.Canary,
		)
	}
}

// BuildCompleted reports a Build completion.
func BuildCompleted(ctx context.Context, b *model.Build) {
	_, r, fr, cr := getLegacyMetricFields(b)
	BuildCounts.Completed.Add(ctx, 1, b.BucketID, b.BuilderID, r, fr, cr, b.Canary)

	endT, _ := ptypes.Timestamp(b.Proto.EndTime)
	BuildDurations.Cycle.Add(
		ctx, endT.Sub(b.CreateTime).Seconds(), b.BucketID, b.BuilderID, r, fr, cr, b.Canary,
	)
	if b.Proto.StartTime != nil {
		startT, _ := ptypes.Timestamp(b.Proto.StartTime)
		BuildDurations.Run.Add(
			ctx, endT.Sub(startT).Seconds(), b.BucketID, b.BuilderID, r, fr, cr, b.Canary,
		)
	}
}

// BuildLeased reports a Build lease.
func BuildLeased(ctx context.Context, b *model.Build) {
	BuildCounts.Leased.Add(ctx, 1, b.BucketID, b.BuilderID)
}

// BuildLeaseExpired reports a Build expiration.
func BuildLeaseExpired(ctx context.Context, b *model.Build) {
	legacyStatus, _, _, _ := getLegacyMetricFields(b)
	BuildCounts.LeaseExpired.Add(ctx, 1, b.BucketID, b.BuilderID, legacyStatus)
}

// HeartbeatFailure reports a heartbeat failure.
func HeartbeatFailure(ctx context.Context) {
	BuildCounts.LeaseExtensionFailed.Add(ctx, 1)
}
