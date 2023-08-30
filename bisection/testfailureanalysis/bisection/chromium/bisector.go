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

// Package chromium performs bisection for test failures for Chromium project.
package chromium

import (
	"context"
	"fmt"

	"go.chromium.org/luci/bisection/internal/lucianalysis"
	"go.chromium.org/luci/bisection/model"
	"go.chromium.org/luci/bisection/rerun"
	"go.chromium.org/luci/bisection/testfailureanalysis/bisection/analysis"
	"go.chromium.org/luci/bisection/util"
	"go.chromium.org/luci/bisection/util/datastoreutil"
	bbpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/gae/service/info"
)

type Bisector struct{}

func (b *Bisector) Prepare(ctx context.Context, tfa *model.TestFailureAnalysis, luciAnalysis analysis.AnalysisClient) error {
	logging.Infof(ctx, "Run chromium bisection")
	bundle, err := datastoreutil.GetTestFailureBundle(ctx, tfa)
	if err != nil {
		return errors.Annotate(err, "get test failures").Err()
	}

	err = b.populateTestSuiteName(ctx, bundle)
	if err != nil {
		return errors.Annotate(err, "populate test suite name").Err()
	}

	err = b.populateTestNames(ctx, bundle, luciAnalysis)
	if err != nil {
		return errors.Annotate(err, "populate test names").Err()
	}

	return nil
}

func (b *Bisector) TriggerRerun(ctx context.Context, tfa *model.TestFailureAnalysis, tfs []*model.TestFailure, gitilesCommit *bbpb.GitilesCommit) (*bbpb.Build, error) {
	builder, err := util.GetTestRerunBuilder(tfa.Project)
	if err != nil {
		return nil, errors.Annotate(err, "get test rerun builder").Err()
	}

	extraProperties := getExtraProperties(ctx, tfa, tfs)

	options := &rerun.TriggerOptions{
		Builder:         builder,
		GitilesCommit:   gitilesCommit,
		Priority:        tfa.Priority,
		SampleBuildID:   tfa.FailedBuildID,
		ExtraProperties: extraProperties,
	}

	build, err := rerun.TriggerRerun(ctx, options)
	if err != nil {
		return nil, errors.Annotate(err, "trigger rerun").Err()
	}

	return build, nil
}

func getExtraProperties(ctx context.Context, tfa *model.TestFailureAnalysis, tfs []*model.TestFailure) map[string]any {
	// This may change depending on what the recipe needs.
	var testsToRun []map[string]string
	for _, tf := range tfs {
		testsToRun = append(testsToRun, map[string]string{
			"test_suite":   tf.TestSuiteName,
			"test_name":    tf.TestName,
			"test_id":      tf.TestID,
			"variant_hash": tf.VariantHash,
		})
	}
	return map[string]any{
		"analysis_id":    tfa.ID,
		"bisection_host": fmt.Sprintf("%s.appspot.com", info.AppID(ctx)),
		"tests_to_run":   testsToRun,
	}
}

// populateTestSuiteName set the correct TestSuiteName for TestFailures.
// For chromium, it gets test suite name from test variant.
func (b *Bisector) populateTestSuiteName(ctx context.Context, bundle *model.TestFailureBundle) error {
	tfs := bundle.All()
	for _, tf := range tfs {
		testSuite, ok := tf.Variant.Def["test_suite"]
		if !ok {
			return fmt.Errorf("no test suite found for test failure %d", tf.ID)
		}
		tf.TestSuiteName = testSuite
	}
	// Store in datastore.
	return datastore.RunInTransaction(ctx, func(c context.Context) error {
		for _, tf := range tfs {
			err := datastore.Put(ctx, tf)
			if err != nil {
				return errors.Annotate(err, "save test failure %d", tf.ID).Err()
			}
		}
		return nil
	}, nil)
}

// populateTestNames queries the test_verdicts table in LUCI Analysis and populate
// the TestName for all TestFailure models in bundle.
// This only triggered whenever we run a bisection (~20 times a day), so the
// cost is manageable.
func (b *Bisector) populateTestNames(ctx context.Context, bundle *model.TestFailureBundle, luciAnalysis analysis.AnalysisClient) error {
	tfs := bundle.All()
	keys := make([]lucianalysis.TestVerdictKey, len(tfs))
	for i, tf := range bundle.All() {
		keys[i] = lucianalysis.TestVerdictKey{
			TestID:      tf.TestID,
			VariantHash: tf.VariantHash,
			RefHash:     tf.RefHash,
		}
	}
	keyMap, err := luciAnalysis.ReadTestNames(ctx, bundle.Primary().Project, keys)
	if err != nil {
		return errors.Annotate(err, "read test names").Err()
	}

	// Store in datastore.
	return datastore.RunInTransaction(ctx, func(c context.Context) error {
		for _, tf := range tfs {
			key := lucianalysis.TestVerdictKey{
				TestID:      tf.TestID,
				VariantHash: tf.VariantHash,
				RefHash:     tf.RefHash,
			}
			testName, ok := keyMap[key]
			if !ok {
				return fmt.Errorf("couldn't find test name for test (%s, %s, %s)", tf.TestID, tf.VariantHash, tf.RefHash)
			}
			tf.TestName = testName
			err := datastore.Put(ctx, tf)
			if err != nil {
				return errors.Annotate(err, "save test failure %d", tf.ID).Err()
			}
		}
		return nil
	}, nil)
}
