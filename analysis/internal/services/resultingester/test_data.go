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
	"time"

	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	gerritpb "go.chromium.org/luci/common/proto/gerrit"
	"go.chromium.org/luci/resultdb/pbutil"
	rdbpb "go.chromium.org/luci/resultdb/proto/v1"

	analysispb "go.chromium.org/luci/analysis/proto/v1"
)

var sampleVar = pbutil.Variant("k1", "v1")

var originalTmd = &rdbpb.TestMetadata{
	Name: "original_name",
	Location: &rdbpb.TestLocation{
		Repo:     "old_repo",
		FileName: "old_file_name",
		Line:     567,
	},
	BugComponent: &rdbpb.BugComponent{
		System: &rdbpb.BugComponent_Monorail{
			Monorail: &rdbpb.MonorailComponent{
				Project: "chrome",
				Value:   "Blink>Component",
			},
		},
	},
	PreviousTestId: "previous_test_id",
}

var updatedTmd = &rdbpb.TestMetadata{
	Name: "updated_name",
	Location: &rdbpb.TestLocation{
		Repo:     "repo",
		FileName: "file_name",
		Line:     456,
	},
	BugComponent: &rdbpb.BugComponent{
		System: &rdbpb.BugComponent_IssueTracker{
			IssueTracker: &rdbpb.IssueTrackerComponent{
				ComponentId: 12345,
			},
		},
	},
	PropertiesSchema: "myproject.MyMessage",
	Properties: &structpb.Struct{
		Fields: map[string]*structpb.Value{
			"string":  structpb.NewStringValue("value"),
			"number":  structpb.NewNumberValue(123),
			"boolean": structpb.NewBoolValue(true),
		},
	},
	PreviousTestId: "another_previous_test_id",
}

var testProperties = &structpb.Struct{
	Fields: map[string]*structpb.Value{
		"stringkey": structpb.NewStringValue("stringvalue"),
		"numberkey": structpb.NewNumberValue(123),
	},
}

func mockedQueryRunTestVerdictsRsp() *rdbpb.QueryRunTestVerdictsResponse {
	response := &rdbpb.QueryRunTestVerdictsResponse{
		RunTestVerdicts: []*rdbpb.RunTestVerdict{
			{
				TestId:      ":module!junit:package:class#test_expected",
				VariantHash: "hash",
				Results: []*rdbpb.TestResultBundle{
					{
						Result: &rdbpb.TestResult{
							ResultId:    "one",
							Name:        "invocations/test-invocation-name/tests/:module%21junit:package:class%23test_expected/results/one",
							Status:      rdbpb.TestStatus_PASS,
							StatusV2:    rdbpb.TestResult_PASSED,
							SummaryHtml: "SummaryHTML for test_expected/one",
							Expected:    true,
							StartTime:   timestamppb.New(time.Date(2010, time.March, 1, 0, 0, 0, 0, time.UTC)),
							Duration:    durationpb.New(time.Second*3 + time.Microsecond),
							Tags:        []*rdbpb.StringPair{{Key: "test-key", Value: "test-value"}},
							FrameworkExtensions: &rdbpb.FrameworkExtensions{
								WebTest: &rdbpb.WebTest{
									Status:     rdbpb.WebTest_PASS,
									IsExpected: true,
								},
							},
						},
					},
				},
				TestMetadata: updatedTmd,
			},
			{
				TestId:      ":module!junit:package:class#test_flaky",
				VariantHash: "hash",
				Results: []*rdbpb.TestResultBundle{
					{
						Result: &rdbpb.TestResult{
							ResultId:    "one",
							Name:        "invocations/test-invocation-name/tests/:module%21junit:package:class%23test_flaky/results/one",
							StartTime:   timestamppb.New(time.Date(2010, time.February, 1, 0, 0, 10, 0, time.UTC)),
							Status:      rdbpb.TestStatus_FAIL,
							StatusV2:    rdbpb.TestResult_FAILED,
							SummaryHtml: "SummaryHTML for test_flaky/one",
							FailureReason: &rdbpb.FailureReason{
								PrimaryErrorMessage: "abc.def(123): unexpected nil-deference",
								Errors: []*rdbpb.FailureReason_Error{
									{
										Message: "abc.def(123): unexpected nil-deference",
									},
								},
							},
							Expected: false,
						},
					},
					{
						Result: &rdbpb.TestResult{
							ResultId:    "two",
							Name:        "invocations/test-invocation-name/tests/:module%21junit:package:class%23test_flaky/results/two",
							StartTime:   timestamppb.New(time.Date(2010, time.February, 1, 0, 0, 20, 0, time.UTC)),
							Status:      rdbpb.TestStatus_PASS,
							StatusV2:    rdbpb.TestResult_PASSED,
							SummaryHtml: "SummaryHTML for test_flaky/two",
							Expected:    true,
						},
					},
				},
			},
			{
				TestId:      ":module!junit:package:class#test_skip",
				Variant:     sampleVar,
				VariantHash: pbutil.VariantHash(sampleVar),
				Results: []*rdbpb.TestResultBundle{
					{
						Result: &rdbpb.TestResult{
							ResultId:    "one",
							Name:        "invocations/test-invocation-name/tests/:module%21junit:package:class%23test_skip/results/one",
							Expected:    true,
							Status:      rdbpb.TestStatus_SKIP,
							StatusV2:    rdbpb.TestResult_SKIPPED,
							SummaryHtml: "SummaryHTML for test_skip/one",
							SkipReason:  rdbpb.SkipReason_AUTOMATICALLY_DISABLED_FOR_FLAKINESS,
							SkippedReason: &rdbpb.SkippedReason{
								Kind:          rdbpb.SkippedReason_DISABLED_AT_DECLARATION,
								ReasonMessage: "Test has @Ignored annotation.",
							},
							Properties: testProperties,
						},
					},
				},
				TestMetadata: originalTmd,
			},
		},
	}

	return response
}

func resultdbParentInvocationForTesting() *rdbpb.Invocation {
	return &rdbpb.Invocation{
		Name:  "invocations/test-invocation-name",
		Realm: "invproject:inv",
		Tags: []*rdbpb.StringPair{
			{
				Key:   "tag-key",
				Value: "tag-value",
			},
			{
				Key:   "tag-key2",
				Value: "tag-value2",
			},
		},
		Properties: &structpb.Struct{
			Fields: map[string]*structpb.Value{
				"prop-key": structpb.NewStringValue("prop-value"),
			},
		},
	}
}

func gerritChangesByHostForTesting() map[string][]*gerritpb.ChangeInfo {
	result := make(map[string][]*gerritpb.ChangeInfo)
	result["project-review.googlesource.com"] = []*gerritpb.ChangeInfo{
		{
			Number:  9991,
			Project: "myproject/src2",
			Owner: &gerritpb.AccountInfo{
				Email: "username@chromium.org",
			},
		},
	}
	return result
}

func resultdbSourcesForTesting() *rdbpb.Sources {
	return &rdbpb.Sources{
		BaseSources: &rdbpb.Sources_GitilesCommit{
			GitilesCommit: &rdbpb.GitilesCommit{
				Host:       "project.googlesource.com",
				Project:    "myproject/src",
				Ref:        "refs/heads/main",
				CommitHash: "abcdefabcd1234567890abcdefabcd1234567890",
				Position:   16801,
			},
		},
		Changelists: []*rdbpb.GerritChange{
			{
				Host:     "project-review.googlesource.com",
				Project:  "myproject/src2",
				Change:   9991,
				Patchset: 82,
			},
		},
		IsDirty: true,
	}
}

func resolvedSourcesForTesting() *analysispb.Sources {
	return &analysispb.Sources{
		BaseSources: &analysispb.Sources_GitilesCommit{
			GitilesCommit: &analysispb.GitilesCommit{
				Host:       "project.googlesource.com",
				Project:    "myproject/src",
				Ref:        "refs/heads/main",
				CommitHash: "abcdefabcd1234567890abcdefabcd1234567890",
				Position:   16801,
			},
		},
		Changelists: []*analysispb.GerritChange{
			{
				Host:      "project-review.googlesource.com",
				Project:   "myproject/src2",
				Change:    9991,
				Patchset:  82,
				OwnerKind: analysispb.ChangelistOwnerKind_HUMAN,
			},
		},
		IsDirty: true,
	}
}

func testInputs() Inputs {
	return Inputs{
		Project:          "rootproject",
		SubRealm:         "root",
		ResultDBHost:     "fake.rdb.host",
		RootInvocationID: "test-root-invocation-name",
		InvocationID:     "test-invocation-name",
		PageNumber:       1,
		PartitionTime:    time.Date(2020, 2, 3, 4, 5, 6, 7, time.UTC),
		Sources:          resolvedSourcesForTesting(),
		Parent:           resultdbParentInvocationForTesting(),
		Verdicts:         mockedQueryRunTestVerdictsRsp().RunTestVerdicts,
	}
}
