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
	"go.chromium.org/luci/bisection/testfailureanalysis/bisection/analysis"
	"go.chromium.org/luci/bisection/util/datastoreutil"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/gae/service/datastore"
)

// Run runs bisection for the given analysis.
func Run(ctx context.Context, tfa *model.TestFailureAnalysis, luciAnalysis analysis.AnalysisClient) error {
	logging.Infof(ctx, "Run chromium bisection")
	bundle, err := datastoreutil.GetTestFailureBundle(ctx, tfa)
	if err != nil {
		return errors.Annotate(err, "get test failures").Err()
	}

	err = populateTestSuiteName(ctx, bundle)
	if err != nil {
		return errors.Annotate(err, "populate test suite name").Err()
	}

	err = populateTestNames(ctx, bundle, luciAnalysis)
	if err != nil {
		return errors.Annotate(err, "populate test names").Err()
	}

	return nil
}

// populateTestSuiteName set the correct TestSuiteName for TestFailures.
// For chromium, it gets test suite name from test variant.
func populateTestSuiteName(ctx context.Context, bundle *model.TestFailureBundle) error {
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
func populateTestNames(ctx context.Context, bundle *model.TestFailureBundle, luciAnalysis analysis.AnalysisClient) error {
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
