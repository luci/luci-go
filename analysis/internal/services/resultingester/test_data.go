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

package resultingester

import (
	"strings"
	"time"

	bbpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/proto/mask"
	"go.chromium.org/luci/resultdb/pbutil"
	rdbpb "go.chromium.org/luci/resultdb/proto/v1"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

var sampleVar = pbutil.Variant("k1", "v1")
var sampleTmd = &rdbpb.TestMetadata{
	Name: "test_new_failure",
}

func mockedGetBuildRsp(inv string) *bbpb.Build {
	build := &bbpb.Build{
		Builder: &bbpb.BuilderID{
			Project: "project",
			Bucket:  "ci",
			Builder: "builder",
		},
		Infra: &bbpb.BuildInfra{
			Resultdb: &bbpb.BuildInfra_ResultDB{
				Hostname:   "results.api.cr.dev",
				Invocation: inv,
			},
		},
		Status: bbpb.Status_FAILURE,
		Input: &bbpb.Build_Input{
			GerritChanges: []*bbpb.GerritChange{
				{
					Host:     "mygerrit-review.googlesource.com",
					Change:   12345,
					Patchset: 5,
				},
				{
					Host:     "anothergerrit-review.googlesource.com",
					Change:   77788,
					Patchset: 19,
				},
			},
		},
		Output: &bbpb.Build_Output{
			GitilesCommit: &bbpb.GitilesCommit{
				Host:     "myproject.googlesource.com",
				Project:  "someproject/src",
				Id:       strings.Repeat("0a", 20),
				Ref:      "refs/heads/mybranch",
				Position: 111888,
			},
		},
		AncestorIds: []int64{},
	}

	isFieldNameJSON := false
	isUpdateMask := false
	m, err := mask.FromFieldMask(buildReadMask, build, isFieldNameJSON, isUpdateMask)
	if err != nil {
		panic(err)
	}
	if err := m.Trim(build); err != nil {
		panic(err)
	}
	return build
}

func mockedQueryTestVariantsRsp() *rdbpb.QueryTestVariantsResponse {
	response := &rdbpb.QueryTestVariantsResponse{
		TestVariants: []*rdbpb.TestVariant{
			{
				TestId:      "ninja://test_consistent_failure",
				VariantHash: "hash",
				Status:      rdbpb.TestVariantStatus_EXONERATED,
				Exonerations: []*rdbpb.TestExoneration{
					// Test behaviour in the presence of multiple exoneration reasons.
					{
						Reason: rdbpb.ExonerationReason_OCCURS_ON_OTHER_CLS,
					},
					{
						Reason: rdbpb.ExonerationReason_NOT_CRITICAL,
					},
					{
						Reason: rdbpb.ExonerationReason_OCCURS_ON_MAINLINE,
					},
				},
				Results: []*rdbpb.TestResultBundle{
					{
						Result: &rdbpb.TestResult{
							Name:      "invocations/build-1234/tests/ninja%3A%2F%2Ftest_consistent_failure/results/one",
							StartTime: timestamppb.New(time.Date(2010, time.March, 1, 0, 0, 0, 0, time.UTC)),
							Status:    rdbpb.TestStatus_FAIL,
							Expected:  false,
							Duration:  durationpb.New(time.Second * 3),
						},
					},
				},
			},
			// Should ignore for test variant analysis.
			{
				TestId:      "ninja://test_expected",
				VariantHash: "hash",
				Status:      rdbpb.TestVariantStatus_EXPECTED,
				Results: []*rdbpb.TestResultBundle{
					{
						Result: &rdbpb.TestResult{
							Name:      "invocations/build-1234/tests/ninja%3A%2F%2Ftest_expected/results/one",
							StartTime: timestamppb.New(time.Date(2010, time.May, 1, 0, 0, 0, 0, time.UTC)),
							Status:    rdbpb.TestStatus_PASS,
							Expected:  true,
							Duration:  durationpb.New(time.Second * 5),
						},
					},
				},
			},
			{
				TestId:      "ninja://test_has_unexpected",
				VariantHash: "hash",
				Status:      rdbpb.TestVariantStatus_FLAKY,
				Results: []*rdbpb.TestResultBundle{
					{
						Result: &rdbpb.TestResult{
							Name:      "invocations/invocation-0b/tests/ninja%3A%2F%2Ftest_has_unexpected/results/one",
							StartTime: timestamppb.New(time.Date(2010, time.February, 1, 0, 0, 10, 0, time.UTC)),
							Status:    rdbpb.TestStatus_FAIL,
							Expected:  false,
						},
					},
					{
						Result: &rdbpb.TestResult{
							Name:      "invocations/invocation-0a/tests/ninja%3A%2F%2Ftest_has_unexpected/results/two",
							StartTime: timestamppb.New(time.Date(2010, time.February, 1, 0, 0, 20, 0, time.UTC)),
							Status:    rdbpb.TestStatus_PASS,
							Expected:  true,
						},
					},
				},
			},
			{
				TestId:      "ninja://test_known_flake",
				VariantHash: "hash_2",
				Status:      rdbpb.TestVariantStatus_UNEXPECTED,
				Variant:     pbutil.Variant("k1", "v2"),
				Results: []*rdbpb.TestResultBundle{
					{
						Result: &rdbpb.TestResult{
							Name:      "invocations/build-1234/tests/ninja%3A%2F%2Ftest_known_flake/results/one",
							StartTime: timestamppb.New(time.Date(2010, time.February, 1, 0, 0, 0, 0, time.UTC)),
							Status:    rdbpb.TestStatus_FAIL,
							Expected:  false,
							Duration:  durationpb.New(time.Second * 2),
							Tags:      pbutil.StringPairs("os", "Mac", "monorail_component", "Monorail>Component"),
						},
					},
				},
			},
			{
				TestId:       "ninja://test_new_failure",
				VariantHash:  "hash_1",
				Status:       rdbpb.TestVariantStatus_UNEXPECTED,
				Variant:      pbutil.Variant("k1", "v1"),
				TestMetadata: sampleTmd,
				Results: []*rdbpb.TestResultBundle{
					{
						Result: &rdbpb.TestResult{
							Name:      "invocations/build-1234/tests/ninja%3A%2F%2Ftest_new_failure/results/one",
							StartTime: timestamppb.New(time.Date(2010, time.January, 1, 0, 0, 0, 0, time.UTC)),
							Status:    rdbpb.TestStatus_FAIL,
							Expected:  false,
							Duration:  durationpb.New(time.Second * 1),
							Tags:      pbutil.StringPairs("random_tag", "random_tag_value", "monorail_component", "Monorail>Component"),
						},
					},
				},
			},
			{
				TestId:      "ninja://test_new_flake",
				VariantHash: "hash",
				Status:      rdbpb.TestVariantStatus_FLAKY,
				Results: []*rdbpb.TestResultBundle{
					{
						Result: &rdbpb.TestResult{
							Name:      "invocations/invocation-1234/tests/ninja%3A%2F%2Ftest_new_flake/results/two",
							StartTime: timestamppb.New(time.Date(2010, time.January, 1, 0, 0, 20, 0, time.UTC)),
							Status:    rdbpb.TestStatus_FAIL,
							Expected:  false,
							Duration:  durationpb.New(time.Second * 11),
						},
					},
					{
						Result: &rdbpb.TestResult{
							Name:      "invocations/invocation-1234/tests/ninja%3A%2F%2Ftest_new_flake/results/one",
							StartTime: timestamppb.New(time.Date(2010, time.January, 1, 0, 0, 10, 0, time.UTC)),
							Status:    rdbpb.TestStatus_FAIL,
							Expected:  false,
							Duration:  durationpb.New(time.Second * 10),
						},
					},
					{
						Result: &rdbpb.TestResult{
							Name:      "invocations/invocation-4567/tests/ninja%3A%2F%2Ftest_new_flake/results/three",
							StartTime: timestamppb.New(time.Date(2010, time.January, 1, 0, 0, 15, 0, time.UTC)),
							Status:    rdbpb.TestStatus_PASS,
							Expected:  true,
							Duration:  durationpb.New(time.Second * 12),
						},
					},
				},
			},
			{
				TestId:      "ninja://test_no_new_results",
				VariantHash: "hash",
				Status:      rdbpb.TestVariantStatus_UNEXPECTED,
				Results: []*rdbpb.TestResultBundle{
					{
						Result: &rdbpb.TestResult{
							Name:      "invocations/build-1234/tests/ninja%3A%2F%2Ftest_no_new_results/results/one",
							StartTime: timestamppb.New(time.Date(2010, time.April, 1, 0, 0, 0, 0, time.UTC)),
							Status:    rdbpb.TestStatus_FAIL,
							Expected:  false,
							Duration:  durationpb.New(time.Second * 4),
						},
					},
				},
			},
			// Should ignore for test variant analysis.
			{
				TestId:      "ninja://test_skip",
				VariantHash: "hash",
				Status:      rdbpb.TestVariantStatus_UNEXPECTEDLY_SKIPPED,
				Results: []*rdbpb.TestResultBundle{
					{
						Result: &rdbpb.TestResult{
							Name:      "invocations/build-1234/tests/ninja%3A%2F%2Ftest_skip/results/one",
							StartTime: timestamppb.New(time.Date(2010, time.February, 2, 0, 0, 0, 0, time.UTC)),
							Status:    rdbpb.TestStatus_SKIP,
							Expected:  false,
						},
					},
				},
			},
			{
				TestId:      "ninja://test_unexpected_pass",
				VariantHash: "hash",
				Status:      rdbpb.TestVariantStatus_UNEXPECTED,
				Results: []*rdbpb.TestResultBundle{
					{
						Result: &rdbpb.TestResult{
							Name:     "invocations/build-1234/tests/ninja%3A%2F%2Ftest_unexpected_pass/results/one",
							Status:   rdbpb.TestStatus_PASS,
							Expected: false,
						},
					},
				},
			},
		},
	}

	isFieldNameJSON := false
	isUpdateMask := false
	m, err := mask.FromFieldMask(testVariantReadMask, &rdbpb.TestVariant{}, isFieldNameJSON, isUpdateMask)
	if err != nil {
		panic(err)
	}
	for _, tv := range response.TestVariants {
		if err := m.Trim(tv); err != nil {
			panic(err)
		}
	}
	return response
}
