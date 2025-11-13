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
	"time"

	"cloud.google.com/go/spanner"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/resultdb/internal/invocations"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

// Builder is a builder for TestResultRow for testing.
type Builder struct {
	row TestResultRow
}

// NewBuilder returns a new builder for a TestResultRow for testing.
// The builder is initialized with some default values.
func NewBuilder(invID invocations.ID, testID string, resultID string) *Builder {
	variant := pbutil.Variant("k1", "v1")
	return &Builder{
		row: TestResultRow{
			InvocationID:    invID,
			TestID:          testID,
			ResultID:        resultID,
			Variant:         variant,
			VariantHash:     pbutil.VariantHash(variant),
			CommitTimestamp: time.Date(2025, 4, 25, 1, 2, 3, 4000, time.UTC),
			IsUnexpected:    false,
			Status:          pb.TestStatus_PASS,
			StatusV2:        pb.TestResult_PASSED,
			SummaryHTML:     "<div>Test Passed</div>",
			StartTime:       timestamppb.New(time.Date(2025, 4, 25, 1, 2, 0, 0, time.UTC)),
			Duration:        durationpb.New(1 * time.Second),
			Tags:            pbutil.StringPairs("t1", "v1"),
			TestMetadata: &pb.TestMetadata{
				Name: "test_metadata_name",
			},
			FailureReason: &pb.FailureReason{
				PrimaryErrorMessage: "error message",
			},
			Properties: &structpb.Struct{
				Fields: map[string]*structpb.Value{
					"key": structpb.NewStringValue("value"),
				},
			},
			SkipReason: pb.SkipReason_AUTOMATICALLY_DISABLED_FOR_FLAKINESS,
		},
	}
}

// WithMinimalFields clears as many fields as possible on the test result while
// keeping it valid. This is useful for testing null and empty value handling.
func (b *Builder) WithMinimalFields() *Builder {
	b.row = TestResultRow{
		InvocationID:    b.row.InvocationID,
		TestID:          b.row.TestID,
		ResultID:        b.row.ResultID,
		Variant:         b.row.Variant,
		VariantHash:     b.row.VariantHash,
		CommitTimestamp: b.row.CommitTimestamp,
		IsUnexpected:    b.row.IsUnexpected,
		Status:          b.row.Status,
		StatusV2:        b.row.StatusV2,
	}
	return b
}

// WithInvocationID sets the invocation ID.
func (b *Builder) WithInvocationID(invID invocations.ID) *Builder {
	b.row.InvocationID = invID
	return b
}

// WithTestID sets the test ID.
func (b *Builder) WithTestID(testID string) *Builder {
	b.row.TestID = testID
	return b
}

// WithResultID sets the result ID.
func (b *Builder) WithResultID(resultID string) *Builder {
	b.row.ResultID = resultID
	return b
}

// WithVariant sets the variant.
func (b *Builder) WithVariant(variant *pb.Variant) *Builder {
	b.row.Variant = variant
	b.row.VariantHash = pbutil.VariantHash(variant)
	return b
}

// WithIsUnexpected sets the IsUnexpected flag.
func (b *Builder) WithIsUnexpected(isUnexpected bool) *Builder {
	b.row.IsUnexpected = isUnexpected
	return b
}

// WithStatus sets the status.
func (b *Builder) WithStatus(status pb.TestStatus) *Builder {
	b.row.Status = status
	return b
}

// WithStatusV2 sets the statusV2.
func (b *Builder) WithStatusV2(statusV2 pb.TestResult_Status) *Builder {
	b.row.StatusV2 = statusV2
	return b
}

// WithSummaryHTML sets the summary HTML.
func (b *Builder) WithSummaryHTML(summaryHTML string) *Builder {
	b.row.SummaryHTML = summaryHTML
	return b
}

// WithStartTime sets the start time.
func (b *Builder) WithStartTime(startTime *timestamppb.Timestamp) *Builder {
	b.row.StartTime = startTime
	return b
}

// WithDuration sets the duration.
func (b *Builder) WithDuration(duration *durationpb.Duration) *Builder {
	b.row.Duration = duration
	return b
}

// WithTags sets the tags.
func (b *Builder) WithTags(tags []*pb.StringPair) *Builder {
	b.row.Tags = tags
	return b
}

// WithTestMetadata sets the test metadata.
func (b *Builder) WithTestMetadata(testMetadata *pb.TestMetadata) *Builder {
	b.row.TestMetadata = testMetadata
	return b
}

// WithFailureReason sets the failure reason.
func (b *Builder) WithFailureReason(failureReason *pb.FailureReason) *Builder {
	b.row.FailureReason = failureReason
	return b
}

// WithProperties sets the properties.
func (b *Builder) WithProperties(properties *structpb.Struct) *Builder {
	b.row.Properties = properties
	return b
}

// WithSkipReason sets the skip reason.
func (b *Builder) WithSkipReason(skipReason pb.SkipReason) *Builder {
	b.row.SkipReason = skipReason
	return b
}

// Build returns the constructed TestResultRow.
func (b *Builder) Build() *TestResultRow {
	// Return a copy to avoid changes to the returned row
	// flowing back into the builder.
	r := b.row.Clone()
	return r
}

// InsertForTesting inserts the test result record for testing purposes.
func InsertForTesting(r *TestResultRow) *spanner.Mutation {
	return r.ToMutation()
}
