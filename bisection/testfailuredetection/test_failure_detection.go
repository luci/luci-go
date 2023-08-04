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

// Package testfailuredetection analyses recent test failures with
// the changepoint analysis from LUCI analysis, and select test failures to bisect.
package testfailuredetection

import (
	"context"
	"math"

	"go.chromium.org/luci/bisection/internal/lucianalysis"
	"go.chromium.org/luci/bisection/model"
	pb "go.chromium.org/luci/bisection/proto/v1"
	"go.chromium.org/luci/bisection/rerun"
	tpb "go.chromium.org/luci/bisection/task/proto"
	"go.chromium.org/luci/bisection/testfailureanalysis/bisection"
	"go.chromium.org/luci/bisection/util"
	"go.chromium.org/luci/bisection/util/datastoreutil"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server"
	"go.chromium.org/luci/server/tq"
	"google.golang.org/protobuf/proto"
)

const (
	taskClass = "test-failure-detection"
	queue     = "test-failure-detection"
)

// RegisterTaskClass registers the task class for tq dispatcher.
func RegisterTaskClass(srv *server.Server, luciAnalysisProject string) error {
	ctx := srv.Context
	ac, err := lucianalysis.NewClient(ctx, srv.Options.CloudProject, luciAnalysisProject)
	if err != nil {
		return err
	}
	srv.RegisterCleanup(func(context.Context) {
		ac.Close()
	})
	tq.RegisterTaskClass(tq.TaskClass{
		ID:        taskClass,
		Prototype: (*tpb.TestFailureDetectionTask)(nil),
		Queue:     queue,
		Kind:      tq.NonTransactional,
		Handler: func(c context.Context, payload proto.Message) error {
			task := payload.(*tpb.TestFailureDetectionTask)
			logging.Infof(c, "Processing test failure detection task with project = %s", task.Project)
			d := &TestFailureDetector{LUCIAnalysis: ac}
			err := d.Find(ctx, task)
			if err != nil {
				err = errors.Annotate(err, "run detection").Err()
				logging.Errorf(ctx, err.Error())
				// If the error is transient, return err to retry.
				if transient.Tag.In(err) {
					return err
				}
				return nil
			}
			return nil
		},
	})
	return nil
}

// Schedule enqueues a task to find test failures to bisect.
func Schedule(ctx context.Context, task *tpb.TestFailureDetectionTask) error {
	return tq.AddTask(ctx, &tq.Task{Payload: task})
}

type analysisClient interface {
	ReadTestFailures(ctx context.Context, opts lucianalysis.ReadTestFailuresOptions) ([]*lucianalysis.BuilderRegressionGroup, error)
}

type TestFailureDetector struct {
	LUCIAnalysis analysisClient
}

// Find finds and group test failures to send to bisector.
func (d *TestFailureDetector) Find(ctx context.Context, task *tpb.TestFailureDetectionTask) error {
	opts := lucianalysis.ReadTestFailuresOptions{
		Project:          task.Project,
		VariantPredicate: task.VariantPredicate,
	}
	groups, err := d.LUCIAnalysis.ReadTestFailures(ctx, opts)
	if err != nil {
		return errors.Annotate(err, "read test failures").Err()
	}
	if len(groups) == 0 {
		logging.Infof(ctx, "No test failure is found for %s", task.Project)
		return nil
	}
	bundles := []testFailureBundle{}
	for _, g := range groups {
		bundle, err := newTestFailureBundle(task.Project, g)
		if err != nil {
			return errors.Annotate(err, "new test failure bundle").Err()
		}
		// Use the redundancy score of the primary test failure as
		// the redundancy score of this test failure bundle.
		rs, err := redundancyScore(ctx, bundle.primary())
		if err != nil {
			return errors.Annotate(err, "calculate redundancy score").Err()
		}
		if rs == 1 {
			// Test failures in this bundle are completely redundant.
			// This bundle should be skipped.
			continue
		}
		bundle.primary().RedundancyScore = rs
		bundles = append(bundles, bundle)
	}
	if len(bundles) == 0 {
		logging.Infof(ctx, "Cannot find new test failures to bisect for project %s", task.Project)
		return nil
	}
	bestBundle := First(bundles)
	if err := saveTestFailuresAndAnalysis(ctx, bestBundle); err != nil {
		return errors.Annotate(err, "save test failure and analysis").Err()
	}
	return nil
}

type testFailureBundle []*model.TestFailure

func (b testFailureBundle) primary() *model.TestFailure {
	if !b[0].IsPrimary {
		panic("primary failure is must be the first failure in the list")
	}
	return b[0]
}

func newTestFailureBundle(project string, group *lucianalysis.BuilderRegressionGroup) (testFailureBundle, error) {
	testFailures := make([]*model.TestFailure, len(group.TestVariants))
	for i, tv := range group.TestVariants {
		variant, err := util.VariantPB(tv.Variant.String())
		if err != nil {
			return nil, err
		}
		testFailures[i] = &model.TestFailure{
			ID:          0,
			Project:     project,
			TestID:      tv.TestID.String(),
			VariantHash: tv.VariantHash.String(),
			Variant:     variant,
			RefHash:     group.RefHash.String(),
			Ref: &pb.SourceRef{System: &pb.SourceRef_Gitiles{
				Gitiles: &pb.GitilesRef{
					Host:    group.Ref.Gitiles.Host.String(),
					Project: group.Ref.Gitiles.Project.String(),
					Ref:     group.Ref.Gitiles.Ref.String(),
				},
			}},
			RegressionStartPosition:  group.RegressionStartPosition.Int64,
			RegressionEndPosition:    group.RegressionEndPosition.Int64,
			StartPositionFailureRate: group.StartPositionFailureRate,
			EndPositionFailureRate:   group.EndPositionFailureRate,
			IsPrimary:                i == 0,
			IsDiverged:               false,
			AnalysisKey:              nil,
			RedundancyScore:          0,
			StartHour:                group.StartHour.Timestamp.UTC(),
		}
	}
	return testFailureBundle(testFailures), nil
}

// RedundancyScore returns a floating point number between 0 and 1 inclusive.
func redundancyScore(c context.Context, tf *model.TestFailure) (float64, error) {
	sameTestVariant, err := datastoreutil.GetTestFailures(c, tf.Project, tf.TestID, tf.RefHash, tf.VariantHash)
	if err != nil {
		return 0, errors.Annotate(err, "get test failures of same test variant").Err()
	}
	for _, a := range sameTestVariant {
		if numberOfOverlapCommit(tf.RegressionStartPosition, tf.RegressionEndPosition,
			a.RegressionStartPosition, a.RegressionEndPosition) > 0 {
			return 1, nil
		}
	}
	maxOverlap := float64(0)
	sameTest, err := datastoreutil.GetTestFailures(c, tf.Project, tf.TestID, tf.RefHash, "")
	if err != nil {
		return 0, errors.Annotate(err, "get test failures of same test").Err()
	}
	for _, t := range sameTest {
		overlap := regressionRangeOverlap(tf.RegressionStartPosition, tf.RegressionEndPosition,
			t.RegressionStartPosition, t.RegressionEndPosition)
		maxOverlap = math.Max(maxOverlap, overlap)
	}
	if maxOverlap < 0 || maxOverlap > 1 {
		return 0, errors.New("maxOverlap must between 0 to 1 inclusive. this suggests something wrong with the implementation")
	}
	return maxOverlap, nil
}

func numberOfOverlapCommit(rl1, ru1, rl2, ru2 int64) float64 {
	return math.Min(float64(ru1), float64(ru2)) - math.Max(float64(rl1), float64(rl2)) + 1
}

func regressionRangeOverlap(rl1, ru1, rl2, ru2 int64) float64 {
	return math.Max(0, numberOfOverlapCommit(rl1, ru1, rl2, ru2)) / float64(ru1-rl1+ru2-rl2+2)
}

// SaveTestFailureAndAnalysis saves the test failures and a test failures analysis into datastore.
// It also transactionally enqueue a task to bisector.
func saveTestFailuresAndAnalysis(ctx context.Context, bundle testFailureBundle) error {
	testFailureAnalysis := &model.TestFailureAnalysis{
		Project:    bundle.primary().Project,
		CreateTime: clock.Now(ctx),
		Status:     pb.AnalysisStatus_CREATED,
		Priority:   rerun.PriorityTestFailure,
	}
	return datastore.RunInTransaction(ctx, func(ctx context.Context) error {
		if err := datastore.AllocateIDs(ctx, testFailureAnalysis); err != nil {
			return errors.Annotate(err, "allocate datastore ID for test failure analysis").Err()
		}
		for _, testFailure := range bundle {
			testFailure.AnalysisKey = datastore.KeyForObj(ctx, testFailureAnalysis)
		}
		if err := datastore.Put(ctx, bundle); err != nil {
			return errors.Annotate(err, "save test failures").Err()
		}
		testFailureAnalysis.TestFailure = datastore.KeyForObj(ctx, bundle.primary())
		if err := datastore.Put(ctx, testFailureAnalysis); err != nil {
			return errors.Annotate(err, "save test failure analysis").Err()
		}
		// Send task to bisector transactionally.
		if err := bisection.Schedule(ctx, testFailureAnalysis.ID); err != nil {
			return errors.Annotate(err, "send task to bisector").Err()
		}
		return nil
	}, nil)
}
