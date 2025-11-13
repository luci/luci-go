// Copyright 2025 The LUCI Authors.
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

package testresults

import (
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	"go.chromium.org/luci/resultdb/internal/invocations"
	"go.chromium.org/luci/resultdb/internal/spanutil"
	"go.chromium.org/luci/resultdb/internal/testutil"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

func TestToSpannerMap(t *testing.T) {
	t.Parallel()
	ftt.Run("TestToSpannerMap", t, func(t *ftt.Test) {
		ctx := testutil.SpannerTestContext(t)
		now := testclock.TestRecentTimeUTC
		ctx, _ = testclock.UseTime(ctx, now)

		row := NewBuilder("inv", "test", "result").Build()

		m := row.ToSpannerMap()
		assert.That(t, m["InvocationId"], should.Equal[any](invocations.ID("inv")))
		assert.That(t, m["TestId"], should.Equal[any]("test"))
		assert.That(t, m["ResultId"], should.Equal[any]("result"))
		assert.That(t, m["Variant"], should.Resemble[any](row.Variant))
		assert.That(t, m["VariantHash"], should.Equal[any](row.VariantHash))
		assert.That(t, m["CommitTimestamp"], should.Equal[any](row.CommitTimestamp))
		assert.That(t, m["IsUnexpected"], should.Equal[any](row.IsUnexpected))
		assert.That(t, m["Status"], should.Equal[any](row.Status))
		assert.That(t, m["StatusV2"], should.Equal[any](row.StatusV2))
		assert.That(t, m["SummaryHtml"], should.Resemble[any](spanutil.Compressed([]byte(row.SummaryHTML))))
		assert.That(t, m["StartTime"], should.Equal[any](row.StartTime))
		assert.That(t, m["RunDurationUsec"], should.Equal[any](row.Duration.AsDuration().Microseconds()))
		assert.That(t, m["Tags"], should.Resemble[any](row.Tags))
		assert.That(t, m["TestMetadata"], should.Resemble[any](spanutil.Compressed(pbutil.MustMarshal(row.TestMetadata))))
		assert.That(t, m["FailureReason"], should.Resemble[any](spanutil.Compressed(pbutil.MustMarshal(row.FailureReason))))
		assert.That(t, m["Properties"], should.Resemble[any](spanutil.Compressed(pbutil.MustMarshal(row.Properties))))
		assert.That(t, m["SkipReason"], should.Equal[any](row.SkipReason))
	})
}

func TestToProto(t *testing.T) {
	t.Parallel()
	ftt.Run("TestToProto", t, func(t *ftt.Test) {
		now := testclock.TestRecentTimeUTC
		row := &TestResultRow{
			InvocationID:    "inv",
			TestID:          "test",
			ResultID:        "result",
			Variant:         pbutil.Variant("key", "val"),
			VariantHash:     "variant_hash",
			CommitTimestamp: now,
			IsUnexpected:    true,
			Status:          pb.TestStatus_FAIL,
			StatusV2:        pb.TestResult_FAILED,
			SummaryHTML:     "summary",
			StartTime:       timestamppb.New(now),
			Duration:        durationpb.New(time.Second),
			Tags:            pbutil.StringPairs("tag_key", "tag_val"),
			TestMetadata: &pb.TestMetadata{
				Name: "test_metadata",
			},
			FailureReason: &pb.FailureReason{
				PrimaryErrorMessage: "failure_reason",
			},
			Properties: &structpb.Struct{
				Fields: map[string]*structpb.Value{
					"prop_key": structpb.NewStringValue("prop_val"),
				},
			},
			SkipReason: pb.SkipReason_AUTOMATICALLY_DISABLED_FOR_FLAKINESS,
		}

		expectedProto := &pb.TestResult{
			TestId:      "test",
			ResultId:    "result",
			Variant:     pbutil.Variant("key", "val"),
			Expected:    false,
			Status:      pb.TestStatus_FAIL,
			StatusV2:    pb.TestResult_FAILED,
			SummaryHtml: "summary",
			StartTime:   timestamppb.New(now),
			Duration:    durationpb.New(time.Second),
			Tags:        pbutil.StringPairs("tag_key", "tag_val"),
			TestMetadata: &pb.TestMetadata{
				Name: "test_metadata",
			},
			FailureReason: &pb.FailureReason{
				PrimaryErrorMessage: "failure_reason",
			},
			Properties: &structpb.Struct{
				Fields: map[string]*structpb.Value{
					"prop_key": structpb.NewStringValue("prop_val"),
				},
			},
			SkipReason: pb.SkipReason_AUTOMATICALLY_DISABLED_FOR_FLAKINESS,
		}

		proto := row.ToProto()
		assert.That(t, proto, should.Resemble(expectedProto))
	})
}
