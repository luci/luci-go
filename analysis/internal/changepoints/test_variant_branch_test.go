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

package changepoints

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/analysis/internal/testutil"
	analysispb "go.chromium.org/luci/analysis/proto/v1"
	rdbpb "go.chromium.org/luci/resultdb/proto/v1"
	"go.chromium.org/luci/server/span"
)

func TestFetchUpdateTestVariantBranch(t *testing.T) {
	Convey("Fetch not found", t, func() {
		ctx := testutil.IntegrationTestContext(t)
		key := TestVariantBranchKey{
			Project:          "proj",
			TestID:           "test_id",
			VariantHash:      "variant_hash",
			GitReferenceHash: "git_hash",
		}
		tvbs, err := ReadTestVariantBranches(span.Single(ctx), []TestVariantBranchKey{key})
		So(err, ShouldBeNil)
		So(len(tvbs), ShouldEqual, 1)
		So(tvbs[0], ShouldBeNil)
	})

	Convey("Insert and fetch", t, func() {
		ctx := testutil.IntegrationTestContext(t)
		tvb1 := &TestVariantBranch{
			IsNew:            true,
			Project:          "proj_1",
			TestID:           "test_id_1",
			VariantHash:      "variant_hash_1",
			GitReferenceHash: []byte("githash1"),
			Variant: &analysispb.Variant{
				Def: map[string]string{
					"key1": "val1",
					"key2": "val2",
				},
			},
			InputBuffer: &InputBuffer{
				HotBufferCapacity: 100,
				HotBuffer: History{
					Verdicts: []PositionVerdict{
						{
							CommitPosition:   15,
							IsSimpleExpected: true,
							Hour:             time.Unix(0, 0),
						},
					},
				},
				ColdBufferCapacity: 2000,
			},
			RecentChangepointCount: 0,
		}

		tvb3 := &TestVariantBranch{
			IsNew:            true,
			Project:          "proj_3",
			TestID:           "test_id_3",
			VariantHash:      "variant_hash_3",
			GitReferenceHash: []byte("githash3"),
			InputBuffer: &InputBuffer{
				HotBufferCapacity: 100,
				HotBuffer: History{
					Verdicts: []PositionVerdict{
						{
							CommitPosition:   20,
							IsSimpleExpected: true,
							Hour:             time.Unix(0, 0),
						},
					},
				},
				ColdBufferCapacity: 2000,
			},
			RecentChangepointCount: 0,
		}

		mutation1 := tvb1.ToMutation()
		mutation3 := tvb3.ToMutation()
		testutil.MustApply(ctx, mutation1, mutation3)

		tvbks := []TestVariantBranchKey{
			makeTestVariantBranchKey("proj_1", "test_id_1", "variant_hash_1", "githash1"),
			makeTestVariantBranchKey("proj_2", "test_id_2", "variant_hash_2", "githash2"),
			makeTestVariantBranchKey("proj_3", "test_id_3", "variant_hash_3", "githash3"),
		}
		tvbs, err := ReadTestVariantBranches(span.Single(ctx), tvbks)
		So(err, ShouldBeNil)
		So(len(tvbs), ShouldEqual, 3)
		// After inserting, the record should not be new anymore.
		tvb1.IsNew = false
		// After decoding, cold buffer should be empty.
		tvb1.InputBuffer.ColdBuffer = History{Verdicts: []PositionVerdict{}}
		So(tvbs[0], ShouldResemble, tvb1)
		So(tvbs[1], ShouldBeNil)
		tvb3.IsNew = false
		tvb3.InputBuffer.ColdBuffer = History{Verdicts: []PositionVerdict{}}
		So(tvbs[2], ShouldResemble, tvb3)
	})

	Convey("Insert and update", t, func() {
		ctx := testutil.IntegrationTestContext(t)

		// Insert a new record.
		tvb := &TestVariantBranch{
			IsNew:            true,
			Project:          "proj_1",
			TestID:           "test_id_1",
			VariantHash:      "variant_hash_1",
			GitReferenceHash: []byte("githash1"),
			Variant: &analysispb.Variant{
				Def: map[string]string{
					"key1": "val1",
					"key2": "val2",
				},
			},
			InputBuffer: &InputBuffer{
				HotBufferCapacity: 100,
				HotBuffer: History{
					Verdicts: []PositionVerdict{
						{
							CommitPosition:   15,
							IsSimpleExpected: true,
							Hour:             time.Unix(0, 0),
						},
					},
				},
				ColdBufferCapacity: 2000,
			},
			RecentChangepointCount: 0,
		}

		mutation := tvb.ToMutation()
		testutil.MustApply(ctx, mutation)

		// Update the record
		tvb = &TestVariantBranch{
			Project:          "proj_1",
			TestID:           "test_id_1",
			VariantHash:      "variant_hash_1",
			GitReferenceHash: []byte("githash1"),
			Variant: &analysispb.Variant{
				Def: map[string]string{
					"key1": "val1",
					"key2": "val2",
				},
			},
			InputBuffer: &InputBuffer{
				HotBufferCapacity: 100,
				HotBuffer: History{
					Verdicts: []PositionVerdict{
						{
							CommitPosition:   16,
							IsSimpleExpected: true,
							Hour:             time.Unix(0, 0),
						},
					},
				},
				ColdBufferCapacity: 2000,
				ColdBuffer: History{
					Verdicts: []PositionVerdict{
						{
							CommitPosition:   15,
							IsSimpleExpected: true,
							Hour:             time.Unix(0, 0),
						},
					},
				},
				IsColdBufferDirty: true,
			},
			RecentChangepointCount: 0,
		}

		mutation = tvb.ToMutation()
		testutil.MustApply(ctx, mutation)

		tvbks := []TestVariantBranchKey{
			makeTestVariantBranchKey("proj_1", "test_id_1", "variant_hash_1", "githash1"),
		}
		tvbs, err := ReadTestVariantBranches(span.Single(ctx), tvbks)
		So(err, ShouldBeNil)
		So(len(tvbs), ShouldEqual, 1)
		tvb.IsNew = false
		tvb.InputBuffer.IsColdBufferDirty = false
		So(tvbs[0], ShouldResemble, tvb)
	})
}

func TestInsertToInputBuffer(t *testing.T) {
	Convey("Insert simple test variant", t, func() {
		tvb := &TestVariantBranch{
			InputBuffer: &InputBuffer{
				HotBufferCapacity:  10,
				ColdBufferCapacity: 100,
			},
		}
		payload := samplePayload(12)
		tv := &rdbpb.TestVariant{
			Status: rdbpb.TestVariantStatus_EXPECTED,
			Results: []*rdbpb.TestResultBundle{
				{
					Result: &rdbpb.TestResult{
						Expected:  true,
						StartTime: timestamppb.New(time.Unix(3600*10, 0)),
					},
				},
			},
		}
		pv, err := toPositionVerdict(tv, payload)
		So(err, ShouldBeNil)
		tvb.InsertToInputBuffer(pv)
		So(len(tvb.InputBuffer.HotBuffer.Verdicts), ShouldEqual, 1)

		So(tvb.InputBuffer.HotBuffer.Verdicts[0], ShouldResemble, PositionVerdict{
			CommitPosition:   12,
			IsSimpleExpected: true,
			Hour:             tv.Results[0].Result.StartTime.AsTime(),
		})
	})

	Convey("Insert non-simple test variant", t, func() {
		tvb := &TestVariantBranch{
			InputBuffer: &InputBuffer{
				HotBufferCapacity:  10,
				ColdBufferCapacity: 100,
			},
		}
		payload := samplePayload(12)
		tv := &rdbpb.TestVariant{
			Status: rdbpb.TestVariantStatus_FLAKY,
			Results: []*rdbpb.TestResultBundle{
				{
					Result: &rdbpb.TestResult{
						Name:      "invocations/run-1/tests/abc",
						Expected:  false,
						StartTime: timestamppb.New(time.Unix(3600*10, 0)),
					},
				},
				{
					Result: &rdbpb.TestResult{
						Name:      "invocations/run-1/tests/abc",
						Expected:  false,
						StartTime: timestamppb.New(time.Unix(3600*11, 0)),
					},
				},
				{
					Result: &rdbpb.TestResult{
						Name:      "invocations/run-1/tests/abc",
						Expected:  true,
						StartTime: timestamppb.New(time.Unix(3600*11, 0)),
					},
				},
				{
					Result: &rdbpb.TestResult{
						Name:      "invocations/run-2/tests/abc",
						Expected:  false,
						StartTime: timestamppb.New(time.Unix(3600*11, 0)),
					},
				},
				{
					Result: &rdbpb.TestResult{
						Name:      "invocations/run-3/tests/abc",
						Expected:  true,
						StartTime: timestamppb.New(time.Unix(3600*11, 0)),
					},
				},
				{
					Result: &rdbpb.TestResult{
						Name:      "invocations/run-3/tests/abc",
						Expected:  true,
						StartTime: timestamppb.New(time.Unix(3600*11, 0)),
					},
				},
				{
					Result: &rdbpb.TestResult{
						Name:      "invocations/run-4/tests/abc",
						Expected:  true,
						StartTime: timestamppb.New(time.Unix(3600*11, 0)),
					},
				},
			},
		}
		pv, err := toPositionVerdict(tv, payload)
		So(err, ShouldBeNil)
		tvb.InsertToInputBuffer(pv)
		So(len(tvb.InputBuffer.HotBuffer.Verdicts), ShouldEqual, 1)

		So(tvb.InputBuffer.HotBuffer.Verdicts[0], ShouldResemble, PositionVerdict{
			CommitPosition:   12,
			IsSimpleExpected: false,
			Hour:             tv.Results[0].Result.StartTime.AsTime(),
			Details: VerdictDetails{
				IsExonerated: false,
				Runs: []Run{
					{
						ExpectedResultCount:   1,
						UnexpectedResultCount: 2,
					},
					{
						ExpectedResultCount:   0,
						UnexpectedResultCount: 1,
					},
					{
						ExpectedResultCount:   2,
						UnexpectedResultCount: 0,
					},
					{
						ExpectedResultCount:   1,
						UnexpectedResultCount: 0,
					},
				},
			},
		})
	})
}

func makeTestVariantBranchKey(proj string, testID string, variantHash string, gitHash string) TestVariantBranchKey {
	return TestVariantBranchKey{
		Project:          proj,
		TestID:           testID,
		VariantHash:      variantHash,
		GitReferenceHash: gitHash,
	}
}
