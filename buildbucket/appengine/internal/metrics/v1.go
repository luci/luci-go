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
	"fmt"
	"math"

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
		"bucket":               field.String("bucket"),
		"builder":              field.String("builder"),
		"canary":               field.Bool("canary"),
		"cancelation_reason":   field.String("cancelation_reason"),
		"failure_reason":       field.String("failure_reason"),
		"must_be_never_leased": field.Bool("must_be_never_leased"),
		"result":               field.String("result"),
		"status":               field.String("status"),
		"user_agent":           field.String("user_agent"),
	}

	// V1 is a collection of metric objects for V1 metrics.
	// Note: when adding new metrics here, please also update bbInternalMetrics
	// in config/config.go.
	V1 = struct {
		BuildCount              metric.Int
		BuildCountCreated       metric.Counter
		BuildCountStarted       metric.Counter
		BuildCountCompleted     metric.Counter
		BuildDurationCycle      metric.CumulativeDistribution
		BuildDurationRun        metric.CumulativeDistribution
		BuildDurationScheduling metric.CumulativeDistribution
		ExpiredLeaseReset       metric.Counter
		MaxAgeScheduled         metric.Float
	}{
		BuildCount: metric.NewInt(
			"buildbucket/builds/count",
			"Number of pending/running prod builds", nil,
			bFields("status")...,
		),
		BuildCountCreated: metric.NewCounter(
			"buildbucket/builds/created",
			"Build creation", nil,
			bFields("user_agent")...,
		),
		BuildCountStarted: metric.NewCounter(
			"buildbucket/builds/started",
			"Build start", nil,
			bFields("canary")...,
		),
		BuildCountCompleted: metric.NewCounter(
			"buildbucket/builds/completed",
			"Build completion, including success, failure and cancellation", nil,
			bFields("result", "failure_reason", "cancelation_reason", "canary")...,
		),
		BuildDurationCycle: newbuildDurationMetric(
			"buildbucket/builds/cycle_durations",
			"Duration between build creation and completion",
		),
		BuildDurationRun: newbuildDurationMetric(
			"buildbucket/builds/run_durations",
			"Duration between build start and completion",
		),
		BuildDurationScheduling: newbuildDurationMetric(
			"buildbucket/builds/scheduling_durations",
			"Duration between build creation and start",
		),
		ExpiredLeaseReset: metric.NewCounter(
			"buildbucket/builds/lease_expired",
			"Build lease expirations", nil,
			bFields("status")...,
		),
		MaxAgeScheduled: metric.NewFloat(
			"buildbucket/builds/max_age_scheduled",
			"Age of the oldest SCHEDULED build",
			&types.MetricMetadata{Units: types.Seconds},
			bFields("must_be_never_leased")...,
		),
	}
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

func getLegacyMetricFields(b *model.Build) (legacyStatus, result, failureR, cancelationR string) {
	// The default values are "" instead of UNSET for backwards compatibility.
	switch b.Status {
	case pb.Status_SCHEDULED:
		legacyStatus = model.Scheduled.String()
	case pb.Status_STARTED:
		legacyStatus = model.Started.String()
	case pb.Status_SUCCESS:
		legacyStatus = model.Completed.String()
		result = model.Success.String()
	case pb.Status_FAILURE:
		legacyStatus = model.Completed.String()
		result = model.Failure.String()
		failureR = model.BuildFailure.String()
	case pb.Status_INFRA_FAILURE:
		legacyStatus = model.Completed.String()
		if b.Proto.StatusDetails.GetTimeout() != nil {
			result = model.Canceled.String()
			cancelationR = model.TimeoutCanceled.String()
		} else {
			result = model.Failure.String()
			failureR = model.InfraFailure.String()
		}
	case pb.Status_CANCELED:
		legacyStatus = model.Completed.String()
		result = model.Canceled.String()
		cancelationR = model.ExplicitlyCanceled.String()
	default:
		panic(fmt.Sprintf("getLegacyMetricFields: invalid status %q", b.Status))
	}
	return
}

// legacyBucketName returns the V1 luci bucket name.
// e.g., "luci.chromium.try".
func legacyBucketName(project, bucket string) string {
	return fmt.Sprintf("luci.%s.%s", project, bucket)
}
