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

package uniquetestvariants

import (
	"context"
	"testing"
	"time"

	"cloud.google.com/go/spanner"
	"go.chromium.org/luci/resultdb/internal/spanutil"
	"go.chromium.org/luci/resultdb/internal/testutil"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
	"go.chromium.org/luci/server/span"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func createVariantForTest(id string) *pb.Variant {
	return &pb.Variant{
		Def: map[string]string{
			"key1": "val" + id,
			"key2": "val" + id,
		},
	}
}

func createTestResultForTest(testID string, vHash string, variant *pb.Variant) *pb.TestResult {
	return &pb.TestResult{
		TestId:      testID,
		VariantHash: vHash,
		Variant:     variant,
	}
}

func TestFromTestResults(t *testing.T) {
	Convey(`FromTestResults`, t, func() {
		variant1 := createVariantForTest("1")
		variant2 := createVariantForTest("2")

		results := []*pb.TestResult{
			createTestResultForTest("test-id-1", pbutil.VariantHash(variant1), variant1),
			createTestResultForTest("test-id-1", pbutil.VariantHash(variant2), variant2),
			createTestResultForTest("test-id-2", pbutil.VariantHash(variant1), variant1),
			createTestResultForTest("test-id-1", pbutil.VariantHash(variant1), variant1),
		}

		utvs := FromTestResults("chromium:public", results)

		// Duplicate variants should be removed.
		So(utvs, ShouldHaveLength, 3)
		expectedUTV1 := &UniqueTestVariant{
			UniqueTestVariantID: UniqueTestVariantID{
				Realm:       "chromium:public",
				TestID:      "test-id-1",
				VariantHash: pbutil.VariantHash(variant1),
			},
			Variant:        variant1,
			LastRecordTime: time.Time{},
		}
		So(utvs[expectedUTV1.UniqueTestVariantID], ShouldResemble, expectedUTV1)

		expectedUTV2 := &UniqueTestVariant{
			UniqueTestVariantID: UniqueTestVariantID{
				Realm:       "chromium:public",
				TestID:      "test-id-1",
				VariantHash: pbutil.VariantHash(variant2),
			},
			Variant:        variant2,
			LastRecordTime: time.Time{},
		}
		So(utvs[expectedUTV2.UniqueTestVariantID], ShouldResemble, expectedUTV2)

		expectedUTV3 := &UniqueTestVariant{
			UniqueTestVariantID: UniqueTestVariantID{
				Realm:       "chromium:public",
				TestID:      "test-id-2",
				VariantHash: pbutil.VariantHash(variant1),
			},
			Variant:        variant1,
			LastRecordTime: time.Time{},
		}
		So(utvs[expectedUTV3.UniqueTestVariantID], ShouldResemble, expectedUTV3)
	})
}

func TestFilterIDsRecordedAfter(t *testing.T) {
	Convey(`FilterIDsRecordedAfter`, t, func() {
		ctx := testutil.SpannerTestContext(t)
		variant1 := createVariantForTest("1")
		variant2 := createVariantForTest("2")

		utv1 := &UniqueTestVariant{
			UniqueTestVariantID: UniqueTestVariantID{
				Realm:       "chromium:public",
				TestID:      "test-id-1",
				VariantHash: pbutil.VariantHash(variant1),
			},
			Variant:        variant1,
			LastRecordTime: time.Now().Add(-60 * time.Hour),
		}

		utv2 := &UniqueTestVariant{
			UniqueTestVariantID: UniqueTestVariantID{
				Realm:       "chromium:public",
				TestID:      "test-id-2",
				VariantHash: pbutil.VariantHash(variant1),
			},
			Variant:        variant1,
			LastRecordTime: time.Now().Add(-36 * time.Hour),
		}

		utv3 := &UniqueTestVariant{
			UniqueTestVariantID: UniqueTestVariantID{
				Realm:       "chromium:public",
				TestID:      "test-id-3",
				VariantHash: pbutil.VariantHash(variant1),
			},
			Variant:        variant1,
			LastRecordTime: time.Now().Add(-12 * time.Hour),
		}

		// Add an additional record with a different variant.
		utv4 := &UniqueTestVariant{
			UniqueTestVariantID: UniqueTestVariantID{
				Realm:       "chromium:public",
				TestID:      "test-id-1",
				VariantHash: pbutil.VariantHash(variant2),
			},
			Variant:        variant2,
			LastRecordTime: time.Now().Add(-12 * time.Hour),
		}

		// Add an additional record with a different realm.
		utv5 := &UniqueTestVariant{
			UniqueTestVariantID: UniqueTestVariantID{
				Realm:       "chromium:public-2",
				TestID:      "test-id-2",
				VariantHash: pbutil.VariantHash(variant1),
			},
			Variant:        variant1,
			LastRecordTime: time.Now().Add(-12 * time.Hour),
		}

		utvs := []*UniqueTestVariant{utv1, utv2, utv3, utv4, utv5}
		_, err := span.ReadWriteTransaction(ctx, func(ctx context.Context) error {
			for _, utv := range utvs {
				row := map[string]interface{}{
					"Realm":          utv.Realm,
					"TestId":         utv.TestID,
					"VariantHash":    utv.VariantHash,
					"Variant":        utv.Variant,
					"LastRecordTime": utv.LastRecordTime,
				}
				mut := spanner.InsertOrUpdateMap("UniqueTestVariants", spanutil.ToSpannerMap(row))
				span.BufferWrite(ctx, mut)
			}
			return nil
		})
		So(err, ShouldBeNil)

		utvIDs := []UniqueTestVariantID{utv1.UniqueTestVariantID, utv2.UniqueTestVariantID, utv3.UniqueTestVariantID}

		utvIDsAfterFilter, err := FilterIDsRecordedAfter(span.Single(ctx), utvIDs, time.Now().Add(-24*time.Hour))
		So(err, ShouldBeNil)
		So(utvIDsAfterFilter, ShouldHaveLength, 1)
		So(utvIDsAfterFilter, ShouldContain, utv3.UniqueTestVariantID)

		utvIDsAfterFilter, err = FilterIDsRecordedAfter(span.Single(ctx), utvIDs, time.Now().Add(-48*time.Hour))
		So(err, ShouldBeNil)
		So(utvIDsAfterFilter, ShouldHaveLength, 2)
		So(utvIDsAfterFilter, ShouldContain, utv2.UniqueTestVariantID)
		So(utvIDsAfterFilter, ShouldContain, utv3.UniqueTestVariantID)
	})
}

func TestInsertOrUpdate(t *testing.T) {
	Convey(`InsertOrUpdate`, t, func() {
		ctx := testutil.SpannerTestContext(t)

		variant1 := createVariantForTest("1")
		variant2 := createVariantForTest("2")

		utvID1 := UniqueTestVariantID{Realm: "chromium:public", TestID: "test-id-1-1", VariantHash: pbutil.VariantHash(variant1)}
		utvID2 := UniqueTestVariantID{Realm: "chromium:public", TestID: "test-id-1-1", VariantHash: pbutil.VariantHash(variant2)}
		utvID3 := UniqueTestVariantID{Realm: "chromium:public", TestID: "test-id-1-2", VariantHash: pbutil.VariantHash(variant1)}
		utvID4 := UniqueTestVariantID{Realm: "chromium:public", TestID: "test-id-1-2", VariantHash: pbutil.VariantHash(variant2)}

		// Record variants from test results.
		commitTimestamp, err := span.ReadWriteTransaction(ctx, func(ctx context.Context) error {
			results := []*pb.TestResult{
				createTestResultForTest("test-id-1-1", pbutil.VariantHash(variant1), variant1),
				createTestResultForTest("test-id-1-1", pbutil.VariantHash(variant2), variant2),
				createTestResultForTest("test-id-1-2", pbutil.VariantHash(variant1), variant1),
			}
			utvs := FromTestResults("chromium:public", results)
			InsertOrUpdate(ctx, FromMap(utvs)...)
			return nil
		})
		So(err, ShouldBeNil)

		// Check if unique variant 1 is recorded.
		utv1, err := Read(span.Single(ctx), utvID1)
		So(err, ShouldBeNil)
		So(utv1.Variant, ShouldResembleProto, variant1)
		So(utv1.LastRecordTime, ShouldEqual, commitTimestamp)

		// Check if unique variant 2 is recorded.
		utv2, err := Read(span.Single(ctx), utvID2)
		So(err, ShouldBeNil)
		So(utv2.Variant, ShouldResembleProto, variant2)
		So(utv2.LastRecordTime, ShouldEqual, commitTimestamp)

		// Check if unique variant 3 is recorded.
		utv3, err := Read(span.Single(ctx), utvID3)
		So(err, ShouldBeNil)
		So(utv3.Variant, ShouldResembleProto, variant1)
		So(utv3.LastRecordTime, ShouldEqual, commitTimestamp)

		// Unique variant 4 should not exist.
		utv4, err := Read(span.Single(ctx), utvID4)
		So(err, ShouldNotBeNil)
		So(utv4, ShouldBeNil)

		// Record more variants from test results after one second.
		time.Sleep(time.Second)
		updatedCommitTimestamp, err := span.ReadWriteTransaction(ctx, func(ctx context.Context) error {
			results := []*pb.TestResult{
				createTestResultForTest("test-id-1-1", pbutil.VariantHash(variant1), variant1),
				createTestResultForTest("test-id-1-2", pbutil.VariantHash(variant2), variant2),
			}
			utvs := FromTestResults("chromium:public", results)
			InsertOrUpdate(ctx, FromMap(utvs)...)
			return nil
		})
		So(err, ShouldBeNil)

		// Unique variant 1 should have the updated lastRecordTime.
		utv1, err = Read(span.Single(ctx), utvID1)
		So(err, ShouldBeNil)
		So(utv1.Variant, ShouldResembleProto, variant1)
		So(utv1.LastRecordTime, ShouldNotEqual, commitTimestamp)
		So(utv1.LastRecordTime, ShouldEqual, updatedCommitTimestamp)

		// Unique variant 2 should be unchanged.
		utv2, err = Read(span.Single(ctx), utvID2)
		So(err, ShouldBeNil)
		So(utv2.Variant, ShouldResembleProto, variant2)
		So(utv2.LastRecordTime, ShouldEqual, commitTimestamp)

		// Unique variant 3 should be unchanged.
		utv3, err = Read(span.Single(ctx), utvID3)
		So(err, ShouldBeNil)
		So(utv3.Variant, ShouldResembleProto, variant1)
		So(utv3.LastRecordTime, ShouldEqual, commitTimestamp)

		// Unique variant 4 should be recorded.
		utv4, err = Read(span.Single(ctx), utvID4)
		So(err, ShouldBeNil)
		So(utv4.Variant, ShouldResembleProto, variant2)
		So(utv4.LastRecordTime, ShouldEqual, updatedCommitTimestamp)
	})
}

func TestQuery(t *testing.T) {
	Convey(`Query`, t, func() {
		ctx := testutil.SpannerTestContext(t)

		variant1 := createVariantForTest("1")
		variant2 := createVariantForTest("2")
		variant3 := createVariantForTest("3")
		variant4 := createVariantForTest("4")
		variant5 := createVariantForTest("5")

		// Record variants from test results.
		_, err := span.ReadWriteTransaction(ctx, func(ctx context.Context) error {
			// Create test results with predefined variant hashes so we know how the
			// unique test variants will be sorted.
			results := []*pb.TestResult{
				createTestResultForTest("test-id-2-1", "hash1", variant1),
				createTestResultForTest("test-id-2-1", "hash2", variant2),
				createTestResultForTest("test-id-2-1", "hash3", variant3),
				createTestResultForTest("test-id-2-1", "hash4", variant4),
				createTestResultForTest("test-id-2-2", "hash5", variant5),
			}
			utvs := FromTestResults("chromium:public", results)
			InsertOrUpdate(ctx, FromMap(utvs)...)
			return nil
		})
		So(err, ShouldBeNil)

		Convey(`E2E`, func() {
			// Query the first page.
			req := &pb.QueryUniqueTestVariantsRequest{
				Realm:    "chromium:public",
				TestId:   "test-id-2-1",
				PageSize: 2,
			}
			res, err := Query(span.Single(ctx), req)
			So(err, ShouldBeNil)
			pageToken := res.GetNextPageToken()

			expected := &pb.QueryUniqueTestVariantsResponse{
				Variants: []*pb.UniqueTestVariant{
					{
						Realm:       "chromium:public",
						TestId:      "test-id-2-1",
						VariantHash: "hash1",
						Variant:     variant1,
					},
					{
						Realm:       "chromium:public",
						TestId:      "test-id-2-1",
						VariantHash: "hash2",
						Variant:     variant2,
					},
				},
				NextPageToken: pageToken,
			}

			So(res, ShouldResembleProto, expected)
			So(pageToken, ShouldNotBeEmpty)

			// Query the second page.
			req = &pb.QueryUniqueTestVariantsRequest{
				Realm:     "chromium:public",
				TestId:    "test-id-2-1",
				PageSize:  2,
				PageToken: pageToken,
			}
			res, err = Query(span.Single(ctx), req)
			So(err, ShouldBeNil)

			expected = &pb.QueryUniqueTestVariantsResponse{
				Variants: []*pb.UniqueTestVariant{
					{
						Realm:       "chromium:public",
						TestId:      "test-id-2-1",
						VariantHash: "hash3",
						Variant:     variant3,
					},
					{
						Realm:       "chromium:public",
						TestId:      "test-id-2-1",
						VariantHash: "hash4",
						Variant:     variant4,
					},
				},
			}

			So(err, ShouldBeNil)
			So(res, ShouldResembleProto, expected)
		})
	})
}
