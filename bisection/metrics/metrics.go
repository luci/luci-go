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

// Package metrics handles sending metrics to tsmon.
package metrics

import (
	"context"
	"fmt"

	"go.chromium.org/luci/bisection/model"
	"go.chromium.org/luci/bisection/util/datastoreutil"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/tsmon"
	"go.chromium.org/luci/common/tsmon/distribution"
	"go.chromium.org/luci/common/tsmon/field"
	"go.chromium.org/luci/common/tsmon/metric"
	"go.chromium.org/luci/common/tsmon/types"
	"go.chromium.org/luci/gae/service/datastore"

	pb "go.chromium.org/luci/bisection/proto"
)

var (
	// Measure how many analyses are currently running
	runningAnalysesGauge = metric.NewInt(
		"bisection/compile/analysis/running_count",
		"The total number running compile analysis, by LUCI project.",
		&types.MetricMetadata{Units: "analyses"},
		// The LUCI Project.
		field.String("project"),
	)
	// Measure how many rerun builds are currently running
	runningRerunGauge = metric.NewInt(
		"bisection/compile/rerun/running_count",
		"The number of running rerun builds, by LUCI project.",
		&types.MetricMetadata{Units: "reruns"},
		// The LUCI Project.
		field.String("project"),
		// "running", "pending"
		field.String("status"),
		// "mac", "windows", "linux"
		field.String("platform"),
	)
	// Measure the "age" of running rerun builds
	rerunAgeMetric = metric.NewNonCumulativeDistribution(
		"bisection/compile/rerun/age",
		"The age of running reruns, by LUCI project.",
		&types.MetricMetadata{Units: "seconds"},
		distribution.DefaultBucketer,
		// The LUCI Project.
		field.String("project"),
		// "running", "pending"
		field.String("status"),
		// "mac", "windows", "linux"
		field.String("platform"),
	)
)

func init() {
	// Register metrics as global metrics, which has the effort of
	// resetting them after every flush.
	tsmon.RegisterGlobalCallback(func(ctx context.Context) {
		// Do nothing -- the metrics will be populated by the cron
		// job itself and does not need to be triggered externally.
	}, runningAnalysesGauge, runningRerunGauge, rerunAgeMetric)
}

// CollectGlobalMetrics is called in a cron job.
// It collects global metrics and send to tsmon.
func CollectGlobalMetrics(c context.Context) error {
	var errs []error
	err := collectMetricsForRunningAnalyses(c)
	if err != nil {
		err = errors.Annotate(err, "collectMetricsForRunningAnalyses").Err()
		errs = append(errs, err)
		logging.Errorf(c, err.Error())
	}
	err = collectMetricsForRunningReruns(c)
	if err != nil {
		err = errors.Annotate(err, "collectMetricsForRunningReruns").Err()
		errs = append(errs, err)
		logging.Errorf(c, err.Error())
	}
	if len(errs) > 0 {
		return errors.NewMultiError(errs...)
	}
	return nil
}

func collectMetricsForRunningAnalyses(c context.Context) error {
	runningCount, err := retrieveRunningAnalyses(c)
	if err != nil {
		return err
	}

	// Set the metric
	for proj, count := range runningCount {
		runningAnalysesGauge.Set(c, int64(count), proj)
	}
	return nil
}

func retrieveRunningAnalyses(c context.Context) (map[string]int, error) {
	q := datastore.NewQuery("CompileFailureAnalysis").Eq("run_status", pb.AnalysisRunStatus_STARTED)
	analyses := []*model.CompileFailureAnalysis{}
	err := datastore.GetAll(c, q, &analyses)
	if err != nil {
		return nil, errors.Annotate(err, "couldn't get running analyses").Err()
	}

	// To store the running analyses for each project
	runningCount := map[string]int{}
	for _, cfa := range analyses {
		build, err := datastoreutil.GetBuild(c, cfa.CompileFailure.Parent().IntID())
		if err != nil {
			return nil, errors.Annotate(err, "getting build for analysis %d", cfa.Id).Err()
		}
		if build == nil {
			return nil, fmt.Errorf("getting build for analysis %d", cfa.Id)
		}

		runningCount[build.Project] = runningCount[build.Project] + 1
	}
	return runningCount, nil
}

func collectMetricsForRunningReruns(c context.Context) error {
	// TODO (nqmtuan): Implement metrics for running reruns
	return nil
}

