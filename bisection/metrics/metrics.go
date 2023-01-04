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
	"time"

	"go.chromium.org/luci/bisection/model"
	"go.chromium.org/luci/bisection/util/datastoreutil"
	buildbucketpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/clock"
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

// rerunKey is keys for maps for runningRerunGauge and rerunAgeMetric
type rerunKey struct {
	Project  string
	Status   string
	Platform string
}

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
	// Query all in-progress single reruns in the last 7 days.
	// We set the limit to 7 days because there maybe cases that for some reasons
	// (e.g. crashes) that a rerun status may not be updated.
	// Any reruns more than 7 days are surely canceled by buildbucket, so it is
	// safe to exclude them.
	cutoffTime := clock.Now(c).Add(-time.Hour * 7 * 24)
	q := datastore.NewQuery("SingleRerun").Eq("Status", pb.RerunStatus_RERUN_STATUS_IN_PROGRESS).Gt("create_time", cutoffTime)
	reruns := []*model.SingleRerun{}
	err := datastore.GetAll(c, q, &reruns)
	if err != nil {
		return errors.Annotate(err, "couldn't get running reruns").Err()
	}

	// Get the metrics for rerun count and rerun age
	// Maps where each key is one project-status-platform combination
	rerunCountMap := map[rerunKey]int64{}
	rerunAgeMap := map[rerunKey]*distribution.Distribution{}
	for _, rerun := range reruns {
		proj, err := projectForRerun(c, rerun)
		if err != nil {
			return errors.Annotate(err, "projectForRerun %d", rerun.Id).Err()
		}

		rerunBuild := &model.CompileRerunBuild{
			Id: rerun.RerunBuild.IntID(),
		}
		err = datastore.Get(c, rerunBuild)
		if err != nil {
			return errors.Annotate(err, "couldn't get rerun build %d", rerun.RerunBuild.IntID()).Err()
		}

		// TODO (nqmtuan): Populate the platform information for rerun in datastore,
		// so we can send the real one to tsmon.
		var key rerunKey = rerunKey{
			Project: proj,
		}
		if rerunBuild.Status == buildbucketpb.Status_STATUS_UNSPECIFIED || rerunBuild.Status == buildbucketpb.Status_SCHEDULED {
			key.Status = "pending"
		}
		if rerunBuild.Status == buildbucketpb.Status_STARTED {
			key.Status = "running"
		}
		if key.Status != "" {
			rerunCountMap[key] = rerunCountMap[key] + 1
			if _, ok := rerunAgeMap[key]; !ok {
				rerunAgeMap[key] = distribution.New(rerunAgeMetric.Bucketer())
			}
			rerunAgeMap[key].Add(rerunAgeInSeconds(c, rerun))
		}
	}

	// Send metrics to tsmon
	for k, count := range rerunCountMap {
		runningRerunGauge.Set(c, count, k.Project, k.Status, k.Platform)
	}

	for k, dist := range rerunAgeMap {
		rerunAgeMetric.Set(c, dist, k.Project, k.Status, k.Platform)
	}

	return nil
}

func projectForRerun(c context.Context, rerun *model.SingleRerun) (string, error) {
	cfa, err := datastoreutil.GetCompileFailureAnalysis(c, rerun.Analysis.IntID())
	if err != nil {
		return "", err
	}
	build, err := datastoreutil.GetBuild(c, cfa.CompileFailure.Parent().IntID())
	if err != nil {
		return "", errors.Annotate(err, "getting build for analysis %d", cfa.Id).Err()
	}
	if build == nil {
		return "", fmt.Errorf("build for analysis %d does not exist", cfa.Id)
	}
	return build.Project, nil
}

func rerunAgeInSeconds(c context.Context, rerun *model.SingleRerun) float64 {
	dur := clock.Now(c).Sub(rerun.CreateTime)
	return dur.Seconds()
}
