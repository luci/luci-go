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

package resultcollector

import (
	rdbpb "go.chromium.org/luci/resultdb/proto/v1"
)

func mockedBatchGetTestVariantsResponse() *rdbpb.BatchGetTestVariantsResponse {
	return &rdbpb.BatchGetTestVariantsResponse{
		TestVariants: []*rdbpb.TestVariant{
			{
				TestId:      "ninja://test_known_flake",
				VariantHash: "variant_hash",
				Status:      rdbpb.TestVariantStatus_FLAKY,
				Results: []*rdbpb.TestResultBundle{
					{
						Result: &rdbpb.TestResult{
							Status: rdbpb.TestStatus_SKIP,
						},
					},
					{
						Result: &rdbpb.TestResult{
							Status: rdbpb.TestStatus_FAIL,
						},
					},
					{
						Result: &rdbpb.TestResult{
							Status: rdbpb.TestStatus_PASS,
						},
					},
				},
			},
			{
				TestId:      "ninja://test_consistent_failure",
				VariantHash: "variant_hash",
				Status:      rdbpb.TestVariantStatus_EXONERATED,
				Results: []*rdbpb.TestResultBundle{
					{
						Result: &rdbpb.TestResult{
							Status: rdbpb.TestStatus_FAIL,
						},
					},
				},
			},
			{
				TestId:      "ninja://test_has_unexpected",
				VariantHash: "variant_hash",
				Status:      rdbpb.TestVariantStatus_EXPECTED,
				Results: []*rdbpb.TestResultBundle{
					{
						Result: &rdbpb.TestResult{
							Status: rdbpb.TestStatus_PASS,
						},
					},
				},
			},
		},
	}
}
