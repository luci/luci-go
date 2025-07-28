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
	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	"go.chromium.org/luci/resultdb/internal/invocations"
	"go.chromium.org/luci/resultdb/internal/spanutil"
	"go.chromium.org/luci/resultdb/internal/testmetadata"
	"go.chromium.org/luci/resultdb/internal/testutil"
	"go.chromium.org/luci/resultdb/internal/testutil/insert"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

func TestFieldExistenceBitField(t *testing.T) {
	ftt.Run(`FieldExistenceBitField`, t, func(t *ftt.Test) {
		t.Run("full test metadata", func(t *ftt.Test) {
			md := fakeFullTestMetadata("testname")

			res := fieldExistenceBitField(md)
			assert.Loosely(t, res, should.Equal(0b11111))
		})

		t.Run("partial test metadata", func(t *ftt.Test) {
			md := &pb.TestMetadata{
				Location:     &pb.TestLocation{FileName: "testfilename"},
				BugComponent: &pb.BugComponent{},
			}

			res := fieldExistenceBitField(md)
			assert.Loosely(t, res, should.Equal(0b10100))
		})

		t.Run("empty test metadata", func(t *ftt.Test) {
			md := &pb.TestMetadata{}

			res := fieldExistenceBitField(md)
			assert.Loosely(t, res, should.Equal(0b00000))
		})
	})
}

func TestUpdateTestMetadata(t *testing.T) {
	ftt.Run(`Error`, t, func(t *ftt.Test) {
		ctx := testutil.SpannerTestContext(t)
		testInvocationID := invocations.ID("inv1")

		t.Run(`Invocation not finalized`, func(t *ftt.Test) {
			testutil.MustApply(ctx, t, insert.Invocation(testInvocationID, pb.Invocation_FINALIZING, nil))

			err := updateTestMetadata(ctx, testInvocationID)
			assert.Loosely(t, err, should.ErrLike("Invocation is not finalized"))
		})
	})

	ftt.Run(`Skip invocation`, t, func(t *ftt.Test) {
		ctx := testutil.SpannerTestContext(t)
		testInvocationID := invocations.ID("inv1")

		t.Run(`Invocation has no sourceSpec`, func(t *ftt.Test) {
			testutil.MustApply(ctx, t, insert.Invocation(testInvocationID, pb.Invocation_FINALIZED, nil))

			err := updateTestMetadata(ctx, testInvocationID)
			assert.Loosely(t, err, should.BeNil)
		})

		t.Run(`Invocation has sources inherited`, func(t *ftt.Test) {
			testutil.MustApply(ctx, t, insert.Invocation(testInvocationID, pb.Invocation_FINALIZED, map[string]any{
				"InheritSources": true,
			}))

			err := updateTestMetadata(ctx, testInvocationID)
			assert.Loosely(t, err, should.BeNil)
		})
	})

	ftt.Run(`No error`, t, func(t *ftt.Test) {
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
			testutil.MustReadRow(ctx, t, "TestMetadata", key, map[string]any{
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
			assert.Loosely(t, err, should.BeNil)
			row.SourceRef = &pb.SourceRef{}
			err = proto.Unmarshal(compressedSourceRef, row.SourceRef)
			assert.Loosely(t, err, should.BeNil)
			// Validate.
			// ShouldResemble does not work on struct with nested proto buffer.
			// So we compare each proto field separately.
			assert.Loosely(t, row.TestMetadata, should.Match(expected.TestMetadata))
			assert.Loosely(t, row.SourceRef, should.Match(expected.SourceRef))
			row.TestMetadata = nil
			row.SourceRef = nil
			expected.TestMetadata = nil
			expected.SourceRef = nil
			expected.LastUpdated = time.Time{} // Remove lastupdated.
			assert.Loosely(t, row, should.Match(&expected))
		}
		t.Run(`Save test metadata for new test`, func(t *ftt.Test) {
			invID := "inv1"
			insertInvocationWithTestResults(ctx, t, invID, invSources(2), []*pb.TestResult{
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
			assert.Loosely(t, err, should.BeNil)
			expected := baseTestMetadata
			expected.TestMetadata = fakeFullTestMetadata("testname") // From test result.
			expected.Position = 2                                    //From Invocation sources.
			verifyDBTestMetadata(ctx, expected)
		})

		t.Run(`Update test metadata - position advanced, less metadata fields, metadata row expired`, func(t *ftt.Test) {
			invID := "inv1"
			insertInvocationWithTestResults(ctx, t, invID, invSources(3), []*pb.TestResult{
				{
					Name:         pbutil.TestResultName(invID, "testID", "0"),
					TestMetadata: &pb.TestMetadata{Name: "updatedtestname"},
				}},
			)
			existingTestMetadata := baseTestMetadata
			existingTestMetadata.TestMetadata = fakeFullTestMetadata("testname")
			existingTestMetadata.LastUpdated = time.Now().Add(-49 * time.Hour)
			existingTestMetadata.Position = 2
			insertTestMetadata(ctx, t, &existingTestMetadata)

			err := updateTestMetadata(ctx, invocations.ID(invID))
			assert.Loosely(t, err, should.BeNil)
			expected := baseTestMetadata
			expected.TestMetadata = &pb.TestMetadata{Name: "updatedtestname"} // From the test result.
			expected.Position = 3                                             // From the invocation sources.
			verifyDBTestMetadata(ctx, expected)
		})

		t.Run(`Update test metadata - commit position advanced, same metadata fields`, func(t *ftt.Test) {
			invID := "inv1"
			insertInvocationWithTestResults(ctx, t, invID, invSources(3), []*pb.TestResult{
				{
					Name:         pbutil.TestResultName(invID, "testID", "0"),
					TestMetadata: fakeFullTestMetadata("updatedtestname"),
				}},
			)
			existingTestMetadata := baseTestMetadata
			existingTestMetadata.LastUpdated = time.Now().Add(-1 * time.Hour)
			existingTestMetadata.TestMetadata = fakeFullTestMetadata("testname")
			existingTestMetadata.Position = 2
			insertTestMetadata(ctx, t, &existingTestMetadata)

			err := updateTestMetadata(ctx, invocations.ID(invID))
			assert.Loosely(t, err, should.BeNil)
			expected := baseTestMetadata
			expected.TestMetadata = fakeFullTestMetadata("updatedtestname") // From test result.
			expected.Position = 3                                           // From Invocation sources.
			verifyDBTestMetadata(ctx, expected)
		})

		t.Run(`Update test metadata - same position more metadata fields`, func(t *ftt.Test) {
			invID := "inv1"
			insertInvocationWithTestResults(ctx, t, invID, invSources(2), []*pb.TestResult{
				{
					Name:         pbutil.TestResultName(invID, "testID", "0"),
					TestMetadata: fakeFullTestMetadata("updatedtestname"),
				}},
			)
			existingTestMetadata := baseTestMetadata
			existingTestMetadata.LastUpdated = time.Now().Add(-1 * time.Hour)
			existingTestMetadata.TestMetadata = &pb.TestMetadata{Name: "testname"}
			existingTestMetadata.Position = 2
			insertTestMetadata(ctx, t, &existingTestMetadata)

			err := updateTestMetadata(ctx, invocations.ID(invID))
			assert.Loosely(t, err, should.BeNil)
			expected := baseTestMetadata
			expected.TestMetadata = fakeFullTestMetadata("updatedtestname") // From test result.
			expected.Position = 2                                           // From Invocation sources.
			verifyDBTestMetadata(ctx, expected)
		})

		t.Run(`No Update - lower position`, func(t *ftt.Test) {
			invID := "inv1"
			insertInvocationWithTestResults(ctx, t, invID, invSources(2), []*pb.TestResult{
				{
					Name:         pbutil.TestResultName(invID, "testID", "0"),
					TestMetadata: fakeFullTestMetadata("updatedtestname"),
				}},
			)
			existingTestMetadata := baseTestMetadata
			existingTestMetadata.LastUpdated = time.Now().Add(-1 * time.Hour)
			existingTestMetadata.TestMetadata = &pb.TestMetadata{Name: "testname"}
			existingTestMetadata.Position = math.MaxInt64
			insertTestMetadata(ctx, t, &existingTestMetadata)

			err := updateTestMetadata(ctx, invocations.ID(invID))
			assert.Loosely(t, err, should.BeNil)
			verifyDBTestMetadata(ctx, existingTestMetadata)
		})

		t.Run(`No Update - same position same metadata fields`, func(t *ftt.Test) {
			invID := "inv1"
			insertInvocationWithTestResults(ctx, t, invID, invSources(2), []*pb.TestResult{
				{
					Name:         pbutil.TestResultName(invID, "testID", "0"),
					TestMetadata: &pb.TestMetadata{Name: "updatedTestname"},
				}},
			)
			existingTestMetadata := baseTestMetadata
			existingTestMetadata.LastUpdated = time.Now().Add(-1 * time.Hour)
			existingTestMetadata.TestMetadata = &pb.TestMetadata{Name: "testname"}
			existingTestMetadata.Position = 2
			insertTestMetadata(ctx, t, &existingTestMetadata)

			err := updateTestMetadata(ctx, invocations.ID(invID))
			assert.Loosely(t, err, should.BeNil)
			verifyDBTestMetadata(ctx, existingTestMetadata)
		})

		t.Run(`No Update -  position advance and less metadata fields`, func(t *ftt.Test) {
			invID := "inv1"
			insertInvocationWithTestResults(ctx, t, invID, invSources(3), []*pb.TestResult{
				{
					Name:         pbutil.TestResultName(invID, "testID", "0"),
					TestMetadata: &pb.TestMetadata{Name: "updatedTestname"},
				}},
			)
			existingTestMetadata := baseTestMetadata
			existingTestMetadata.LastUpdated = time.Now().Add(-1 * time.Hour)
			existingTestMetadata.TestMetadata = fakeFullTestMetadata("testname")
			existingTestMetadata.Position = 2
			insertTestMetadata(ctx, t, &existingTestMetadata)

			err := updateTestMetadata(ctx, invocations.ID(invID))
			assert.Loosely(t, err, should.BeNil)
			verifyDBTestMetadata(ctx, existingTestMetadata)
		})

		t.Run(`Save test metadata for new tests from different invocations`, func(t *ftt.Test) {
			testInvocationID := invocations.ID("inv1")
			includedInvocationID := invocations.ID("includedinv")
			testutil.MustApply(ctx, t,
				testutil.CombineMutations(
					insert.InvocationWithInclusions(testInvocationID, pb.Invocation_FINALIZED, map[string]any{
						"Realm":   "testproject:testrealm",
						"Sources": spanutil.Compressed(pbutil.MustMarshal(invSources(2)))}, includedInvocationID),
					insert.InvocationWithInclusions(includedInvocationID, pb.Invocation_FINALIZED, map[string]any{
						"Realm":          "testproject:otherRealm",
						"InheritSources": true}),
				)...)
			testutil.MustApply(ctx, t, insert.TestResultMessages(t, []*pb.TestResult{
				{
					Name:         pbutil.TestResultName(string(testInvocationID), "testID", "1"),
					TestMetadata: &pb.TestMetadata{Name: "testname"},
				},
				{
					Name:         pbutil.TestResultName(string(includedInvocationID), "includedTestID", "1"),
					TestMetadata: &pb.TestMetadata{Name: "includedtestname"},
				}})...)

			err := updateTestMetadata(ctx, testInvocationID)
			assert.Loosely(t, err, should.BeNil)
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

func insertInvocationWithTestResults(ctx context.Context, t testing.TB, invID string, sources *pb.Sources, testResults []*pb.TestResult) {
	t.Helper()
	testutil.MustApply(ctx, t,
		insert.Invocation(invocations.ID(invID), pb.Invocation_FINALIZED, map[string]any{
			"Realm":   "testproject:testrealm",
			"Sources": spanutil.Compressed(pbutil.MustMarshal(sources))}))
	testutil.MustApply(ctx, t, insert.TestResultMessages(t, testResults)...)
}

func insertTestMetadata(ctx context.Context, t testing.TB, tm *testmetadata.TestMetadataRow) {
	testutil.MustApply(ctx, t, insert.TestMetadataRows([]*testmetadata.TestMetadataRow{tm})...)
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
