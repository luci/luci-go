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

	"go.chromium.org/luci/bisection/model"
	pb "go.chromium.org/luci/bisection/proto/v1"
	"go.chromium.org/luci/bisection/util"
	"go.chromium.org/luci/bisection/util/datastoreutil"
)

var (
	// Measure how many analyses are currently running
	runningAnalysesGauge = metric.NewInt(
		"bisection/analysis/running_count",
		"The total number running compile analysis, by LUCI project.",
		&types.MetricMetadata{Units: "analyses"},
		// The LUCI Project.
		field.String("project"),
		// The type of the analysis.
		// The possible values are "compile", "test".
		field.String("type"),
	)
	// Measure how many rerun builds are currently running
	runningRerunGauge = metric.NewInt(
		"bisection/rerun/running_count",
		"The number of running rerun builds, by LUCI project.",
		&types.MetricMetadata{Units: "reruns"},
		// The LUCI Project.
		field.String("project"),
		// "running", "pending"
		field.String("status"),
		// "mac", "windows", "linux"
		field.String("platform"),
		// The type of the analysis that rerun belongs to.
		// The possible values are "compile", "test".
		field.String("type"),
	)
	// Measure the "age" of running rerun builds
	rerunAgeMetric = metric.NewNonCumulativeDistribution(
		"bisection/rerun/age",
		"The age of running reruns, by LUCI project.",
		&types.MetricMetadata{Units: "seconds"},
		distribution.DefaultBucketer,
		// The LUCI Project.
		field.String("project"),
		// "running", "pending"
		field.String("status"),
		// "mac", "windows", "linux"
		field.String("platform"),
		// The type of the analysis that rerun belongs to.
		// The possible values are "compile", "test".
		field.String("type"),
	)
)

// AnalysisType is used for sending metrics to tsmon
type AnalysisType string

const (
	AnalysisTypeCompile AnalysisType = "compile"
	AnalysisTypeTest    AnalysisType = "test"
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
		err = errors.Fmt("collectMetricsForRunningAnalyses: %w", err)
		errs = append(errs, err)
		logging.Errorf(c, err.Error())
	}
	err = collectMetricsForRunningReruns(c)
	if err != nil {
		err = errors.Fmt("collectMetricsForRunningReruns: %w", err)
		errs = append(errs, err)
		logging.Errorf(c, err.Error())
	}
	err = collectMetricsForRunningTestReruns(c)
	if err != nil {
		err = errors.Fmt("collectMetricsForRunningTestReruns: %w", err)
		errs = append(errs, err)
		logging.Errorf(c, err.Error())
	}
	if len(errs) > 0 {
		return errors.NewMultiError(errs...)
	}
	return nil
}

func collectMetricsForRunningAnalyses(c context.Context) error {
	// Compile failure analysis running count.
	compileRunningCount, err := retrieveRunningAnalyses(c)
	if err != nil {
		return err
	}
	// Test failure analysis running count.
	testRunningCount, err := retrieveRunningTestAnalyses(c)
	if err != nil {
		return err
	}
	// Set the metric
	for proj, count := range compileRunningCount {
		runningAnalysesGauge.Set(c, int64(count), proj, string(AnalysisTypeCompile))
	}
	for proj, count := range testRunningCount {
		runningAnalysesGauge.Set(c, int64(count), proj, string(AnalysisTypeTest))
	}
	return nil
}

func retrieveRunningTestAnalyses(c context.Context) (map[string]int, error) {
	q := datastore.NewQuery("TestFailureAnalysis").Eq("run_status", pb.AnalysisRunStatus_STARTED)
	analyses := []*model.TestFailureAnalysis{}
	err := datastore.GetAll(c, q, &analyses)
	if err != nil {
		return nil, errors.Fmt("get running test failure analyses: %w", err)
	}

	// To store the running analyses for each project
	runningCount := map[string]int{}
	for _, tfa := range analyses {
		runningCount[tfa.Project] = runningCount[tfa.Project] + 1
	}
	return runningCount, nil
}

func retrieveRunningAnalyses(c context.Context) (map[string]int, error) {
	q := datastore.NewQuery("CompileFailureAnalysis").Eq("run_status", pb.AnalysisRunStatus_STARTED)
	analyses := []*model.CompileFailureAnalysis{}
	err := datastore.GetAll(c, q, &analyses)
	if err != nil {
		return nil, errors.Fmt("couldn't get running analyses: %w", err)
	}

	// To store the running analyses for each project
	runningCount := map[string]int{}
	for _, cfa := range analyses {
		build, err := datastoreutil.GetBuild(c, cfa.CompileFailure.Parent().IntID())
		if err != nil {
			return nil, errors.Fmt("getting build for analysis %d: %w", cfa.Id, err)
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
		return errors.Fmt("couldn't get running reruns: %w", err)
	}

	// Get the metrics for rerun count and rerun age
	// Maps where each key is one project-status-platform combination
	rerunCountMap := map[rerunKey]int64{}
	rerunAgeMap := map[rerunKey]*distribution.Distribution{}
	for _, rerun := range reruns {
		proj, platform, err := projectAndPlatformForRerun(c, rerun)
		if err != nil {
			return errors.Fmt("projectForRerun %d: %w", rerun.Id, err)
		}

		rerunBuild := &model.CompileRerunBuild{
			Id: rerun.RerunBuild.IntID(),
		}
		err = datastore.Get(c, rerunBuild)
		if err != nil {
			return errors.Fmt("couldn't get rerun build %d: %w", rerun.RerunBuild.IntID(), err)
		}

		var key = rerunKey{
			Project:  proj,
			Platform: platform,
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
		runningRerunGauge.Set(c, count, k.Project, k.Status, k.Platform, string(AnalysisTypeCompile))
	}

	for k, dist := range rerunAgeMap {
		rerunAgeMetric.Set(c, dist, k.Project, k.Status, k.Platform, string(AnalysisTypeCompile))
	}

	return nil
}

func projectAndPlatformForRerun(c context.Context, rerun *model.SingleRerun) (string, string, error) {
	cfa, err := datastoreutil.GetCompileFailureAnalysis(c, rerun.Analysis.IntID())
	if err != nil {
		return "", "", err
	}
	build, err := datastoreutil.GetBuild(c, cfa.CompileFailure.Parent().IntID())
	if err != nil {
		return "", "", errors.Fmt("getting build for analysis %d: %w", cfa.Id, err)
	}
	if build == nil {
		return "", "", fmt.Errorf("build for analysis %d does not exist", cfa.Id)
	}
	return build.Project, string(build.Platform), nil
}

func collectMetricsForRunningTestReruns(c context.Context) error {
	// Query all in-progress single reruns in the last 7 days.
	// We set the limit to 7 days because there maybe cases that for some reasons
	// (e.g. crashes) that a rerun status may not be updated.
	// Any reruns more than 7 days are surely canceled by buildbucket, so it is
	// safe to exclude them.
	cutoffTime := clock.Now(c).Add(-time.Hour * 7 * 24)
	q := datastore.NewQuery("TestSingleRerun").Eq("status", pb.RerunStatus_RERUN_STATUS_IN_PROGRESS).Gt("luci_build.create_time", cutoffTime)
	reruns := []*model.TestSingleRerun{}
	err := datastore.GetAll(c, q, &reruns)
	if err != nil {
		return errors.Fmt("get running test reruns: %w", err)
	}

	// Get the metrics for rerun count and rerun age
	// Maps where each key is one project-status-platform combination
	rerunCountMap := map[rerunKey]int64{}
	rerunAgeMap := map[rerunKey]*distribution.Distribution{}
	for _, rerun := range reruns {
		os := util.GetDimensionWithKey(rerun.Dimensions, "os")
		if os == nil {
			logging.Warningf(c, "rerun dimension has no OS %d", rerun.ID)
			continue
		}
		var key = rerunKey{
			Project:  rerun.Project,
			Platform: string(model.PlatformFromOS(c, os.Value)),
		}
		if rerun.LUCIBuild.Status == buildbucketpb.Status_STATUS_UNSPECIFIED || rerun.LUCIBuild.Status == buildbucketpb.Status_SCHEDULED {
			key.Status = "pending"
		}
		if rerun.LUCIBuild.Status == buildbucketpb.Status_STARTED {
			key.Status = "running"
		}
		if key.Status != "" {
			rerunCountMap[key] = rerunCountMap[key] + 1
			if _, ok := rerunAgeMap[key]; !ok {
				rerunAgeMap[key] = distribution.New(rerunAgeMetric.Bucketer())
			}
			dur := clock.Now(c).Sub(rerun.CreateTime)
			rerunAgeMap[key].Add(dur.Seconds())
		}
	}

	// Send metrics to tsmon
	for k, count := range rerunCountMap {
		runningRerunGauge.Set(c, count, k.Project, k.Status, k.Platform, string(AnalysisTypeTest))
	}

	for k, dist := range rerunAgeMap {
		rerunAgeMetric.Set(c, dist, k.Project, k.Status, k.Platform, string(AnalysisTypeTest))
	}

	return nil
}

func rerunAgeInSeconds(c context.Context, rerun *model.SingleRerun) float64 {
	dur := clock.Now(c).Sub(rerun.CreateTime)
	return dur.Seconds()
}
