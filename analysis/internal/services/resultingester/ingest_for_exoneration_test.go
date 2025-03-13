// Copyright 2024 The LUCI Authors.
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

package resultingester

import (
	"testing"
	"time"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/analysis/internal/testresults"
	"go.chromium.org/luci/analysis/internal/testresults/lowlatency"
	"go.chromium.org/luci/analysis/internal/testutil"
	"go.chromium.org/luci/analysis/pbutil"
	analysispb "go.chromium.org/luci/analysis/proto/v1"

	_ "go.chromium.org/luci/server/tq/txn/spanner"
)

func TestIngestForExoneration(t *testing.T) {
	ftt.Run("TestIngestForExoneration", t, func(t *ftt.Test) {
		ctx := testutil.IntegrationTestContext(t)

		inputs := testInputs()

		sources := testresults.Sources{
			RefHash: pbutil.SourceRefHash(&analysispb.SourceRef{
				System: &analysispb.SourceRef_Gitiles{
					Gitiles: &analysispb.GitilesRef{
						Host:    "project.googlesource.com",
						Project: "myproject/src",
						Ref:     "refs/heads/main",
					},
				},
			}),
			Position: 16801,
			Changelists: []testresults.Changelist{
				{
					Host:      "project-review.googlesource.com",
					Change:    9991,
					Patchset:  82,
					OwnerKind: analysispb.ChangelistOwnerKind_HUMAN,
				},
			},
			IsDirty: true,
		}

		partitionTime := time.Date(2020, 2, 3, 4, 5, 6, 7, time.UTC)

		expectedResults := []*lowlatency.TestResult{
			{
				Project:          "rootproject",
				TestID:           ":module!junit:package:class#test_expected",
				VariantHash:      "hash",
				Sources:          sources,
				RootInvocationID: "test-root-invocation-name",
				InvocationID:     "test-invocation-name",
				ResultID:         "one",
				PartitionTime:    partitionTime,
				SubRealm:         "root",
				Status:           analysispb.TestResultStatus_PASS,
			},
			{
				Project:          "rootproject",
				TestID:           ":module!junit:package:class#test_flaky",
				VariantHash:      "hash",
				Sources:          sources,
				RootInvocationID: "test-root-invocation-name",
				InvocationID:     "test-invocation-name",
				ResultID:         "one",
				PartitionTime:    partitionTime,
				SubRealm:         "root",
				IsUnexpected:     true,
				Status:           analysispb.TestResultStatus_FAIL,
			},
			{
				Project:          "rootproject",
				TestID:           ":module!junit:package:class#test_flaky",
				VariantHash:      "hash",
				Sources:          sources,
				RootInvocationID: "test-root-invocation-name",
				InvocationID:     "test-invocation-name",
				ResultID:         "two",
				PartitionTime:    partitionTime,
				SubRealm:         "root",
				Status:           analysispb.TestResultStatus_PASS,
			},
			{
				Project:          "rootproject",
				TestID:           ":module!junit:package:class#test_skip",
				VariantHash:      "d70268c39e188014",
				Sources:          sources,
				RootInvocationID: "test-root-invocation-name",
				InvocationID:     "test-invocation-name",
				ResultID:         "one",
				PartitionTime:    partitionTime,
				SubRealm:         "root",
				IsUnexpected:     false,
				Status:           analysispb.TestResultStatus_SKIP,
			},
		}

		t.Run(`With sources`, func(t *ftt.Test) {
			// Base case should already have sources set.
			assert.Loosely(t, inputs.Sources, should.NotBeNil)

			ingester := IngestForExoneration{}
			err := ingester.Ingest(ctx, inputs)
			assert.Loosely(t, err, should.BeNil)

			results, err := lowlatency.ReadAllForTesting(span.Single(ctx))
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, results, should.Match(expectedResults))
		})
		t.Run(`Without sources`, func(t *ftt.Test) {
			inputs.Sources = nil

			ingester := IngestForExoneration{}
			err := ingester.Ingest(ctx, inputs)
			assert.Loosely(t, err, should.BeNil)

			results, err := lowlatency.ReadAllForTesting(span.Single(ctx))
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, results, should.HaveLength(0))
		})
	})
}
