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

	"go.chromium.org/luci/bisection/model"
	"go.chromium.org/luci/bisection/util/datastoreutil"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/gae/service/datastore"
)

// Run runs bisection for the given analysis.
func Run(ctx context.Context, tfa *model.TestFailureAnalysis) error {
	logging.Infof(ctx, "Run chromium bisection")
	bundle, err := datastoreutil.GetTestFailureBundle(ctx, tfa)
	if err != nil {
		return errors.Annotate(err, "get test failures").Err()
	}

	err = populateTestSuiteName(ctx, bundle)
	if err != nil {
		return errors.Annotate(err, "populate test suite name").Err()
	}

	err = populateTestNames(ctx, bundle)
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
func populateTestNames(ctx context.Context, bundle *model.TestFailureBundle) error {
	// TODO(nqmtuan): Implement this (depends on LUCI Analysis client on
	// crrev.com/c/4713866)
	// The query is something like
	// SELECT
	// 	test_id,
	// 	variant_hash,
	// 	source_ref_hash,
	// 	ARRAY_AGG (
	// 		(	SELECT value FROM UNNEST(tv.results[0].tags) WHERE KEY = "test_name")
	// 			ORDER BY tv.partition_time DESC
	// 			LIMIT 1
	// 		)[OFFSET(0)] as test_name,
	// FROM `luci-analysis.chromium.test_verdicts` tv
	// WHERE
	// 	(
	// 		(
	// 			test_id = <test_id_1>
	// 			AND variant_hash = <variant_hash_1>
	// 			AND source_ref_hash = <source_ref_hash_1>
	// 		)
	// 		OR
	// 	 	(
	// 			test_id = <test_id_2>
	// 			AND variant_hash = <variant_hash_2>
	// 			AND source_ref_hash = <source_ref_hash_2>
	// 		)
	// 	)
	// 	AND partition_time >= TIMESTAMP_SUB(CURRENT_TIMESTAMP(), INTERVAL 10 DAY)
	// GROUP BY test_id, variant_hash, source_ref_hash
	return nil
}
