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

package testmetadataupdator

import (
	"context"
	"math"
	"testing"
	"time"

	"cloud.google.com/go/spanner"
	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/resultdb/internal/invocations"
	"go.chromium.org/luci/resultdb/internal/spanutil"
	"go.chromium.org/luci/resultdb/internal/testmetadata"
	"go.chromium.org/luci/resultdb/internal/testutil"
	"go.chromium.org/luci/resultdb/internal/testutil/insert"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

func TestFieldExistenceBitField(t *testing.T) {
	Convey(`FieldExistenceBitField`, t, func() {
		Convey("full test metadata", func() {
			md := fakeFullTestMetadata("testname")

			res := fieldExistenceBitField(md)
			So(res, ShouldEqual, 0b11111)
		})

		Convey("partial test metadata", func() {
			md := &pb.TestMetadata{
				Location:     &pb.TestLocation{FileName: "testfilename"},
				BugComponent: &pb.BugComponent{},
			}

			res := fieldExistenceBitField(md)
			So(res, ShouldEqual, 0b10100)
		})

		Convey("empty test metadata", func() {
			md := &pb.TestMetadata{}

			res := fieldExistenceBitField(md)
			So(res, ShouldEqual, 0b00000)
		})
	})
}

func TestUpdateTestMetadata(t *testing.T) {

	Convey(`Error`, t, func() {
		ctx := testutil.SpannerTestContext(t)
		testInvocationID := invocations.ID("inv1")

		Convey(`Invocation not finalized`, func() {
			testutil.MustApply(ctx, insert.Invocation(testInvocationID, pb.Invocation_FINALIZING, nil))

			err := updateTestMetadata(ctx, testInvocationID)
			So(err, ShouldErrLike, "Invocation is not finalized")
		})
	})

	Convey(`Skip invocation`, t, func() {
		ctx := testutil.SpannerTestContext(t)
		testInvocationID := invocations.ID("inv1")

		Convey(`Invocation has no sourceSpec`, func() {
			testutil.MustApply(ctx, insert.Invocation(testInvocationID, pb.Invocation_FINALIZED, nil))

			err := updateTestMetadata(ctx, testInvocationID)
			So(err, ShouldBeNil)
		})

		Convey(`Invocation has sources inherited`, func() {
			testutil.MustApply(ctx, insert.Invocation(testInvocationID, pb.Invocation_FINALIZED, map[string]any{
				"InheritSources": true,
			}))

			err := updateTestMetadata(ctx, testInvocationID)
			So(err, ShouldBeNil)
		})
	})

	Convey(`No error`, t, func() {
		ctx := testutil.SpannerTestContext(t)

		invSources := func(position int64) *pb.Sources {
			return &pb.Sources{
				GitilesCommit: &pb.GitilesCommit{
					Host:       "testHost",
					Project:    "testProject",
					Ref:        "testRef",
					CommitHash: "testCommitHash",
					Position:   position,
				},
				IsDirty: false}
		}
		sourceRef := &pb.SourceRef{
			System: &pb.SourceRef_Gitiles{
				Gitiles: &pb.GitilesRef{
					Host:    "testHost",
					Project: "testProject",
					Ref:     "testRef",
				},
			},
		}
		baseTestMetadata := testmetadata.TestMetadataRow{
			Project:      "testproject",
			TestID:       "testID",
			RefHash:      pbutil.SourceRefHash(sourceRef),
			SubRealm:     "testrealm",
			TestMetadata: &pb.TestMetadata{},
			SourceRef:    sourceRef,
			Position:     0,
		}

		verifyDBTestMetadata := func(ctx context.Context, expected testmetadata.TestMetadataRow) {
			// Read from spanner.
			row := &testmetadata.TestMetadataRow{}
			var compressedTestMetadata spanutil.Compressed
			var compressedSourceRef spanutil.Compressed
			key := spanner.Key{expected.Project, expected.TestID, expected.RefHash, expected.SubRealm}
			testutil.MustReadRow(ctx, "TestMetadata", key, map[string]any{
				"Project":      &row.Project,
				"TestId":       &row.TestID,
				"RefHash":      &row.RefHash,
				"SubRealm":     &row.SubRealm,
				"TestMetadata": &compressedTestMetadata,
				"SourceRef":    &compressedSourceRef,
				"Position":     &row.Position,
			})
			row.TestMetadata = &pb.TestMetadata{}
			err := proto.Unmarshal(compressedTestMetadata, row.TestMetadata)
			So(err, ShouldBeNil)
			row.SourceRef = &pb.SourceRef{}
			err = proto.Unmarshal(compressedSourceRef, row.SourceRef)
			So(err, ShouldBeNil)
			// Validate.
			// ShouldResemble does not work on struct with nested proto buffer.
			// So we compare each proto field separately.
			So(row.TestMetadata, ShouldResembleProto, expected.TestMetadata)
			So(row.SourceRef, ShouldResembleProto, expected.SourceRef)
			row.TestMetadata = nil
			row.SourceRef = nil
			expected.TestMetadata = nil
			expected.SourceRef = nil
			expected.LastUpdated = time.Time{} // Remove lastupdated.
			So(row, ShouldResemble, &expected)
		}
		Convey(`Save test metadata for new test`, func() {
			invID := "inv1"
			insertInvocationWithTestResults(ctx, invID, invSources(2), []*pb.TestResult{
				{
					Name:         pbutil.TestResultName(invID, "testID", "1"),
					TestMetadata: &pb.TestMetadata{Name: "othertestname"},
				},
				{
					Name:         pbutil.TestResultName(invID, "testID", "0"),
					TestMetadata: fakeFullTestMetadata("testname"),
				}},
			)

			err := updateTestMetadata(ctx, invocations.ID(invID))
			So(err, ShouldBeNil)
			expected := baseTestMetadata
			expected.TestMetadata = fakeFullTestMetadata("testname") // From test result.
			expected.Position = 2                                    //From Invocation sources.
			verifyDBTestMetadata(ctx, expected)
		})

		Convey(`Update test metadata - position advanced, less metadata fields, metadata row expired`, func() {
			invID := "inv1"
			insertInvocationWithTestResults(ctx, invID, invSources(3), []*pb.TestResult{
				{
					Name:         pbutil.TestResultName(invID, "testID", "0"),
					TestMetadata: &pb.TestMetadata{Name: "updatedtestname"},
				}},
			)
			existingTestMetadata := baseTestMetadata
			existingTestMetadata.TestMetadata = fakeFullTestMetadata("testname")
			existingTestMetadata.LastUpdated = time.Now().Add(-49 * time.Hour)
			existingTestMetadata.Position = 2
			insertTestMetadata(ctx, &existingTestMetadata)

			err := updateTestMetadata(ctx, invocations.ID(invID))
			So(err, ShouldBeNil)
			expected := baseTestMetadata
			expected.TestMetadata = &pb.TestMetadata{Name: "updatedtestname"} // From the test result.
			expected.Position = 3                                             // From the invocation sources.
			verifyDBTestMetadata(ctx, expected)
		})

		Convey(`Update test metadata - commit position advanced, same metadata fields`, func() {
			invID := "inv1"
			insertInvocationWithTestResults(ctx, invID, invSources(3), []*pb.TestResult{
				{
					Name:         pbutil.TestResultName(invID, "testID", "0"),
					TestMetadata: fakeFullTestMetadata("updatedtestname"),
				}},
			)
			existingTestMetadata := baseTestMetadata
			existingTestMetadata.LastUpdated = time.Now().Add(-1 * time.Hour)
			existingTestMetadata.TestMetadata = fakeFullTestMetadata("testname")
			existingTestMetadata.Position = 2
			insertTestMetadata(ctx, &existingTestMetadata)

			err := updateTestMetadata(ctx, invocations.ID(invID))
			So(err, ShouldBeNil)
			expected := baseTestMetadata
			expected.TestMetadata = fakeFullTestMetadata("updatedtestname") // From test result.
			expected.Position = 3                                           // From Invocation sources.
			verifyDBTestMetadata(ctx, expected)
		})

		Convey(`Update test metadata - same position more metadata fields`, func() {
			invID := "inv1"
			insertInvocationWithTestResults(ctx, invID, invSources(2), []*pb.TestResult{
				{
					Name:         pbutil.TestResultName(invID, "testID", "0"),
					TestMetadata: fakeFullTestMetadata("updatedtestname"),
				}},
			)
			existingTestMetadata := baseTestMetadata
			existingTestMetadata.LastUpdated = time.Now().Add(-1 * time.Hour)
			existingTestMetadata.TestMetadata = &pb.TestMetadata{Name: "testname"}
			existingTestMetadata.Position = 2
			insertTestMetadata(ctx, &existingTestMetadata)

			err := updateTestMetadata(ctx, invocations.ID(invID))
			So(err, ShouldBeNil)
			expected := baseTestMetadata
			expected.TestMetadata = fakeFullTestMetadata("updatedtestname") // From test result.
			expected.Position = 2                                           // From Invocation sources.
			verifyDBTestMetadata(ctx, expected)
		})

		Convey(`No Update - lower position`, func() {
			invID := "inv1"
			insertInvocationWithTestResults(ctx, invID, invSources(2), []*pb.TestResult{
				{
					Name:         pbutil.TestResultName(invID, "testID", "0"),
					TestMetadata: fakeFullTestMetadata("updatedtestname"),
				}},
			)
			existingTestMetadata := baseTestMetadata
			existingTestMetadata.LastUpdated = time.Now().Add(-1 * time.Hour)
			existingTestMetadata.TestMetadata = &pb.TestMetadata{Name: "testname"}
			existingTestMetadata.Position = math.MaxInt64
			insertTestMetadata(ctx, &existingTestMetadata)

			err := updateTestMetadata(ctx, invocations.ID(invID))
			So(err, ShouldBeNil)
			verifyDBTestMetadata(ctx, existingTestMetadata)
		})

		Convey(`No Update - same position same metadata fields`, func() {
			invID := "inv1"
			insertInvocationWithTestResults(ctx, invID, invSources(2), []*pb.TestResult{
				{
					Name:         pbutil.TestResultName(invID, "testID", "0"),
					TestMetadata: &pb.TestMetadata{Name: "updatedTestname"},
				}},
			)
			existingTestMetadata := baseTestMetadata
			existingTestMetadata.LastUpdated = time.Now().Add(-1 * time.Hour)
			existingTestMetadata.TestMetadata = &pb.TestMetadata{Name: "testname"}
			existingTestMetadata.Position = 2
			insertTestMetadata(ctx, &existingTestMetadata)

			err := updateTestMetadata(ctx, invocations.ID(invID))
			So(err, ShouldBeNil)
			verifyDBTestMetadata(ctx, existingTestMetadata)
		})

		Convey(`No Update -  position advance and less metadata fields`, func() {
			invID := "inv1"
			insertInvocationWithTestResults(ctx, invID, invSources(3), []*pb.TestResult{
				{
					Name:         pbutil.TestResultName(invID, "testID", "0"),
					TestMetadata: &pb.TestMetadata{Name: "updatedTestname"},
				}},
			)
			existingTestMetadata := baseTestMetadata
			existingTestMetadata.LastUpdated = time.Now().Add(-1 * time.Hour)
			existingTestMetadata.TestMetadata = fakeFullTestMetadata("testname")
			existingTestMetadata.Position = 2
			insertTestMetadata(ctx, &existingTestMetadata)

			err := updateTestMetadata(ctx, invocations.ID(invID))
			So(err, ShouldBeNil)
			verifyDBTestMetadata(ctx, existingTestMetadata)
		})

		Convey(`Save test metadata for new tests from different invocations`, func() {
			testInvocationID := invocations.ID("inv1")
			includedInvocationID := invocations.ID("includedinv")
			testutil.MustApply(ctx,
				testutil.CombineMutations(
					insert.InvocationWithInclusions(testInvocationID, pb.Invocation_FINALIZED, map[string]any{
						"Realm":   "testproject:testrealm",
						"Sources": spanutil.Compressed(pbutil.MustMarshal(invSources(2)))}, includedInvocationID),
					insert.InvocationWithInclusions(includedInvocationID, pb.Invocation_FINALIZED, map[string]any{
						"Realm":          "testproject:otherRealm",
						"InheritSources": true}),
				)...)
			testutil.MustApply(ctx, insert.TestResultMessages([]*pb.TestResult{
				{
					Name:         pbutil.TestResultName(string(testInvocationID), "testID", "1"),
					TestMetadata: &pb.TestMetadata{Name: "testname"},
				},
				{
					Name:         pbutil.TestResultName(string(includedInvocationID), "includedTestID", "1"),
					TestMetadata: &pb.TestMetadata{Name: "includedtestname"},
				}})...)

			err := updateTestMetadata(ctx, testInvocationID)
			So(err, ShouldBeNil)
			expected1 := baseTestMetadata
			expected1.TestMetadata = &pb.TestMetadata{Name: "testname"}
			expected1.Position = 2
			expected2 := baseTestMetadata
			expected2.TestID = "includedTestID"
			expected2.SubRealm = "otherRealm"
			expected2.TestMetadata = &pb.TestMetadata{Name: "includedtestname"}
			expected2.Position = 2
			verifyDBTestMetadata(ctx, expected1)
			verifyDBTestMetadata(ctx, expected2)
		})
	})
}

func insertInvocationWithTestResults(ctx context.Context, invID string, sources *pb.Sources, testResults []*pb.TestResult) {
	testutil.MustApply(ctx,
		insert.Invocation(invocations.ID(invID), pb.Invocation_FINALIZED, map[string]any{
			"Realm":   "testproject:testrealm",
			"Sources": spanutil.Compressed(pbutil.MustMarshal(sources))}))
	testutil.MustApply(ctx, insert.TestResultMessages(testResults)...)
}

func insertTestMetadata(ctx context.Context, tm *testmetadata.TestMetadataRow) {
	testutil.MustApply(ctx, insert.TestMetadataRows([]*testmetadata.TestMetadataRow{tm})...)
}

func fakeFullTestMetadata(testname string) *pb.TestMetadata {
	return &pb.TestMetadata{
		Name: testname,
		Location: &pb.TestLocation{
			Repo:     "testrepo",
			FileName: "testFileName",
			Line:     10,
		},
		BugComponent: &pb.BugComponent{
			System: &pb.BugComponent_IssueTracker{
				IssueTracker: &pb.IssueTrackerComponent{
					ComponentId: 100,
				},
			},
		},
	}
}
