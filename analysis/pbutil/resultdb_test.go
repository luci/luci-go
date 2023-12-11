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

package pbutil

import (
	"encoding/hex"
	"testing"

	rdbpb "go.chromium.org/luci/resultdb/proto/v1"

	pb "go.chromium.org/luci/analysis/proto/v1"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestResultDB(t *testing.T) {
	Convey("FailureReasonFromResultDB", t, func() {
		rdbFailureReason := &rdbpb.FailureReason{
			PrimaryErrorMessage: "Some error message.",
		}
		fr := FailureReasonFromResultDB(rdbFailureReason)
		So(fr, ShouldResembleProto, &pb.FailureReason{
			PrimaryErrorMessage: "Some error message.",
		})
	})
	Convey("SkipReasonFromResultDB", t, func() {
		// Confirm LUCI Analysis handles every skip reason defined by ResultDB.
		// This test is designed to break if ResultDB extends the set of
		// allowed values, without a corresponding update to LUCI Analysis.
		for _, v := range rdbpb.SkipReason_value {
			rdbReason := rdbpb.SkipReason(v)

			reason := SkipReasonFromResultDB(rdbReason)
			if rdbReason == rdbpb.SkipReason_SKIP_REASON_UNSPECIFIED {
				So(reason, ShouldEqual, pb.TestVerdictStatus_TEST_VERDICT_STATUS_UNSPECIFIED)
				continue
			}
			So(reason, ShouldNotEqual, pb.TestVerdictStatus_TEST_VERDICT_STATUS_UNSPECIFIED)
		}

	})
	Convey("TestMetadataFromResultDB", t, func() {
		rdbTestMetadata := &rdbpb.TestMetadata{
			Name: "name",
			Location: &rdbpb.TestLocation{
				Repo:     "repo",
				FileName: "fileName",
				Line:     123,
			},
		}
		tmd := TestMetadataFromResultDB(rdbTestMetadata)
		So(tmd, ShouldResembleProto, &pb.TestMetadata{
			Name: "name",
			Location: &pb.TestLocation{
				Repo:     "repo",
				FileName: "fileName",
				Line:     123,
			}})
	})
	Convey("TestResultStatusFromResultDB", t, func() {
		// Confirm LUCI Analysis handles every test status defined by ResultDB.
		// This test is designed to break if ResultDB extends the set of
		// allowed values, without a corresponding update to LUCI Analysis.
		for _, v := range rdbpb.TestStatus_value {
			rdbStatus := rdbpb.TestStatus(v)
			if rdbStatus == rdbpb.TestStatus_STATUS_UNSPECIFIED {
				continue
			}

			status := TestResultStatusFromResultDB(rdbStatus)
			So(status, ShouldNotEqual, pb.TestResultStatus_TEST_RESULT_STATUS_UNSPECIFIED)
		}
	})
	Convey("TestVerdictStatusFromResultDB", t, func() {
		// Confirm LUCI Analysis handles every test variant status defined by ResultDB.
		// This test is designed to break if ResultDB extends the set of
		// allowed values, without a corresponding update to LUCI Analysis.
		for _, v := range rdbpb.TestVariantStatus_value {
			rdbStatus := rdbpb.TestVariantStatus(v)
			if rdbStatus == rdbpb.TestVariantStatus_TEST_VARIANT_STATUS_UNSPECIFIED ||
				rdbStatus == rdbpb.TestVariantStatus_UNEXPECTED_MASK {
				continue
			}

			status := TestVerdictStatusFromResultDB(rdbStatus)
			So(status, ShouldNotEqual, pb.TestVerdictStatus_TEST_VERDICT_STATUS_UNSPECIFIED)
		}
	})
	Convey("ExonerationReasonFromResultDB", t, func() {
		// Confirm LUCI Analysis handles every exoneration reason defined by
		// ResultDB.
		// This test is designed to break if ResultDB extends the set of
		// allowed values, without a corresponding update to LUCI Analysis.
		for _, v := range rdbpb.ExonerationReason_value {
			rdbReason := rdbpb.ExonerationReason(v)
			if rdbReason == rdbpb.ExonerationReason_EXONERATION_REASON_UNSPECIFIED {
				continue
			}

			reason := ExonerationReasonFromResultDB(rdbReason)
			So(reason, ShouldNotEqual, pb.ExonerationReason_EXONERATION_REASON_UNSPECIFIED)
		}
	})
	Convey("BugComponent from ResultDB Metadata", t, func() {
		Convey("using issuetracker bug component", func() {
			resultDbTmd := &rdbpb.TestMetadata{
				BugComponent: &rdbpb.BugComponent{
					System: &rdbpb.BugComponent_IssueTracker{
						IssueTracker: &rdbpb.IssueTrackerComponent{
							ComponentId: 12345,
						},
					},
				},
			}
			converted := TestMetadataFromResultDB(resultDbTmd)

			So(converted.BugComponent.System.(*pb.BugComponent_IssueTracker).IssueTracker.ComponentId, ShouldEqual, 12345)
		})
		Convey("using monorail bug component", func() {
			resultDbTmd := &rdbpb.TestMetadata{
				BugComponent: &rdbpb.BugComponent{
					System: &rdbpb.BugComponent_Monorail{
						Monorail: &rdbpb.MonorailComponent{
							Project: "chrome",
							Value:   "Blink>Data",
						},
					},
				},
			}
			converted := TestMetadataFromResultDB(resultDbTmd)

			So(converted.BugComponent.System.(*pb.BugComponent_Monorail).Monorail.Project, ShouldEqual, "chrome")
			So(converted.BugComponent.System.(*pb.BugComponent_Monorail).Monorail.Value, ShouldEqual, "Blink>Data")
		})
	})
	Convey("SourcesFromResultDB", t, func() {
		rdbSources := &rdbpb.Sources{
			GitilesCommit: &rdbpb.GitilesCommit{
				Host:       "project.googlesource.com",
				Project:    "myproject/src",
				Ref:        "refs/heads/main",
				CommitHash: "abcdefabcd1234567890abcdefabcd1234567890",
				Position:   16801,
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
		converted := SourcesFromResultDB(rdbSources)
		So(converted, ShouldResembleProto, &pb.Sources{
			GitilesCommit: &pb.GitilesCommit{
				Host:       "project.googlesource.com",
				Project:    "myproject/src",
				Ref:        "refs/heads/main",
				CommitHash: "abcdefabcd1234567890abcdefabcd1234567890",
				Position:   16801,
			},
			Changelists: []*pb.GerritChange{
				{
					Host:     "project-review.googlesource.com",
					Project:  "myproject/src2",
					Change:   9991,
					Patchset: 82,
				},
			},
			IsDirty: true,
		})
	})
	Convey("SourceRef to resultdb", t, func() {
		sourceRef := &pb.SourceRef{
			System: &pb.SourceRef_Gitiles{
				Gitiles: &pb.GitilesRef{
					Host:    "host",
					Project: "proj",
					Ref:     "ref",
				},
			},
		}
		sourceRef1 := SourceRefToResultDB(sourceRef)
		So(sourceRef1, ShouldResembleProto, &rdbpb.SourceRef{
			System: &rdbpb.SourceRef_Gitiles{
				Gitiles: &rdbpb.GitilesRef{
					Host:    "host",
					Project: "proj",
					Ref:     "ref",
				},
			},
		})
	})
	Convey("RefFromSources", t, func() {
		sources := &pb.Sources{
			GitilesCommit: &pb.GitilesCommit{
				Host:       "project.googlesource.com",
				Project:    "myproject/src",
				Ref:        "refs/heads/main",
				CommitHash: "abcdefabcd1234567890abcdefabcd1234567890",
				Position:   16801,
			},
		}
		ref := SourceRefFromSources(sources)
		So(ref, ShouldResembleProto, &pb.SourceRef{
			System: &pb.SourceRef_Gitiles{
				Gitiles: &pb.GitilesRef{
					Host:    "project.googlesource.com",
					Project: "myproject/src",
					Ref:     "refs/heads/main",
				},
			},
		})
	})
	Convey("RefHash", t, func() {
		ref := &pb.SourceRef{
			System: &pb.SourceRef_Gitiles{
				Gitiles: &pb.GitilesRef{
					Host:    "project.googlesource.com",
					Project: "myproject/src",
					Ref:     "refs/heads/main",
				},
			},
		}
		hash := SourceRefHash(ref)
		So(hex.EncodeToString(hash), ShouldEqual, `5d47c679cf080cb5`)
	})
}
