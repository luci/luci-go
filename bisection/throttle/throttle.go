// Copyright 2023 The LUCI Authors.
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

// Package throttle analysis current running reruns and send task to test failure detector.
package throttle

import (
	"context"
	"time"

	"go.chromium.org/luci/bisection/internal/config"
	"go.chromium.org/luci/bisection/model"
	pb "go.chromium.org/luci/bisection/proto/v1"
	tpb "go.chromium.org/luci/bisection/task/proto"
	"go.chromium.org/luci/bisection/testfailuredetection"
	"go.chromium.org/luci/bisection/util"
	buildbucketpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/gae/service/datastore"
)

const (
	// Rerun that is pending for more than 5 minutes should be
	// considered as congested.
	congestedPendingThreshold = -time.Minute * 5
	// Rerun that is older than 7 days should be excluded.
	// Because there maybe cases that for some reasons
	// (e.g. crashes) that status may not be updated.
	// Any reruns more than 7 days are surely canceled by buildbucket, so it is
	// safe to exclude them.
	cutoffThreshold = -time.Hour * 7 * 24
)

func CronHandler(ctx context.Context) error {
	// TODO (nqmtuan): This does not seem to return chromium project in prod.
	// Enable this back when we know what is wrong.
	// projectsToProcess, err := config.SupportedProjects(ctx)
	// if err != nil {
	// 	return errors.Annotate(err, "supported projects").Err()
	// }
	projectsToProcess := []string{"chromium", "chrome"}
	for _, project := range projectsToProcess {
		count, err := dailyAnalysisCount(ctx, project)
		if err != nil {
			return errors.Annotate(err, "daily analysis count").Err()
		}
		dailyLimit, err := dailyLimit(ctx, project)
		if err != nil {
			return errors.Annotate(err, "daily limit").Err()
		}
		if count >= dailyLimit {
			logging.Warningf(ctx, "%d reached daily limit %d for project %s", count, dailyLimit, project)
			return nil
		}
		rerunBuilds, err := congestedCompileReruns(ctx, project)
		if err != nil {
			return errors.Annotate(err, "obtain congested compile reruns").Err()
		}
		testReruns, err := congestedTestReruns(ctx, project)
		if err != nil {
			return errors.Annotate(err, "obtain congested test reruns").Err()
		}
		dimensionExcludes := []*pb.Dimension{}
		for _, d := range allRerunDimensions(rerunBuilds, testReruns) {
			if dim := util.GetDimensionWithKey(d, "os"); dim != nil {
				dimensionExcludes = append(dimensionExcludes, dim)
			}
		}
		util.SortDimension(dimensionExcludes)
		task := &tpb.TestFailureDetectionTask{
			Project:           project,
			DimensionExcludes: dimensionExcludes,
		}
		if err := testfailuredetection.Schedule(ctx, task); err != nil {
			return errors.Annotate(err, "schedule test failure detection task").Err()
		}
		logging.Infof(ctx, "Test failure detection task scheduled %v", task)
	}
	return nil
}

func dailyAnalysisCount(ctx context.Context, project string) (int, error) {
	cutoffTime := clock.Now(ctx).Add(-time.Hour * 24)
	q := datastore.NewQuery("TestFailureAnalysis").Eq("project", project).Gt("create_time", cutoffTime)
	analyses := []*model.TestFailureAnalysis{}
	err := datastore.GetAll(ctx, q, &analyses)
	if err != nil {
		return 0, errors.Annotate(err, "get analyses").Err()
	}
	count := 0
	for _, tfa := range analyses {
		if tfa.Status != pb.AnalysisStatus_DISABLED && tfa.Status != pb.AnalysisStatus_UNSUPPORTED {
			count++
		}
	}
	return count, nil
}

func congestedCompileReruns(ctx context.Context, project string) ([]*model.SingleRerun, error) {
	cutoffTime := clock.Now(ctx).Add(cutoffThreshold)
	pendingCutoffTime := clock.Now(ctx).Add(congestedPendingThreshold)
	q := datastore.NewQuery("CompileRerunBuild").
		Eq("status", buildbucketpb.Status_SCHEDULED).
		Eq("project", project).
		Gt("create_time", cutoffTime).
		Lt("create_time", pendingCutoffTime)
	rerunBuilds := []*model.CompileRerunBuild{}
	err := datastore.GetAll(ctx, q, &rerunBuilds)
	if err != nil {
		return nil, errors.Annotate(err, "get scheduled CompileRerunBuilds").Err()
	}
	reruns := []*model.SingleRerun{}
	for _, r := range rerunBuilds {
		rerun := []*model.SingleRerun{}
		q := datastore.NewQuery("SingleRerun").Eq("rerun_build", datastore.KeyForObj(ctx, r))
		err := datastore.GetAll(ctx, q, &rerun)
		if err != nil {
			return nil, errors.Annotate(err, "get rerun with CompileRerunBuilds ID %d", r.Id).Err()
		}
		reruns = append(reruns, rerun...)
	}
	return reruns, nil
}

func congestedTestReruns(ctx context.Context, project string) ([]*model.TestSingleRerun, error) {
	cutoffTime := clock.Now(ctx).Add(cutoffThreshold)
	pendingCutoffTime := clock.Now(ctx).Add(congestedPendingThreshold)
	q := datastore.NewQuery("TestSingleRerun").
		Eq("luci_build.status", buildbucketpb.Status_SCHEDULED).
		Eq("luci_build.project", project).
		Gt("luci_build.create_time", cutoffTime).
		Lt("luci_build.create_time", pendingCutoffTime)
	reruns := []*model.TestSingleRerun{}
	err := datastore.GetAll(ctx, q, &reruns)
	if err != nil {
		return nil, errors.Annotate(err, "get scheduled TestSingleRerun").Err()
	}
	return reruns, nil
}

func allRerunDimensions(rerunBuilds []*model.SingleRerun, testReruns []*model.TestSingleRerun) []*pb.Dimensions {
	dims := []*pb.Dimensions{}
	for _, r := range rerunBuilds {
		dims = append(dims, r.Dimensions)
	}
	for _, r := range testReruns {
		dims = append(dims, r.Dimensions)
	}
	return dims
}

func dailyLimit(ctx context.Context, project string) (int, error) {
	cfg, err := config.Project(ctx, project)
	if err != nil {
		return 0, err
	}
	return (int)(cfg.TestAnalysisConfig.GetDailyLimit()), nil
}
