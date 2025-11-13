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
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/resultdb/internal/invocations"
	"go.chromium.org/luci/resultdb/internal/spanutil"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

// TestResultRow corresponds to the schema of the TestResults Spanner table.
type TestResultRow struct {
	InvocationID    invocations.ID
	TestID          string
	ResultID        string
	Variant         *pb.Variant
	VariantHash     string
	CommitTimestamp time.Time
	IsUnexpected    bool
	Status          pb.TestStatus
	StatusV2        pb.TestResult_Status
	SummaryHTML     string
	StartTime       *timestamppb.Timestamp
	Duration        *durationpb.Duration
	Tags            []*pb.StringPair
	TestMetadata    *pb.TestMetadata
	FailureReason   *pb.FailureReason
	Properties      *structpb.Struct
	SkipReason      pb.SkipReason
}

// Clone makes a deep copy of the row.
func (r *TestResultRow) Clone() *TestResultRow {
	ret := *r
	if r.Variant != nil {
		ret.Variant = proto.Clone(r.Variant).(*pb.Variant)
	}
	if r.Tags != nil {
		ret.Tags = make([]*pb.StringPair, len(r.Tags))
		for i, tp := range r.Tags {
			ret.Tags[i] = proto.Clone(tp).(*pb.StringPair)
		}
	}
	if r.TestMetadata != nil {
		ret.TestMetadata = proto.Clone(r.TestMetadata).(*pb.TestMetadata)
	}
	if r.FailureReason != nil {
		ret.FailureReason = proto.Clone(r.FailureReason).(*pb.FailureReason)
	}
	if r.Properties != nil {
		ret.Properties = proto.Clone(r.Properties).(*structpb.Struct)
	}
	return &ret
}

// ToSpannerMap converts the row to a map for spanner.
func (r *TestResultRow) ToSpannerMap() map[string]any {
	return map[string]any{
		"InvocationId":    r.InvocationID,
		"TestId":          r.TestID,
		"ResultId":        r.ResultID,
		"Variant":         r.Variant,
		"VariantHash":     r.VariantHash,
		"CommitTimestamp": r.CommitTimestamp,
		"IsUnexpected":    r.IsUnexpected,
		"Status":          r.Status,
		"StatusV2":        r.StatusV2,
		"SummaryHtml":     spanutil.Compressed([]byte(r.SummaryHTML)),
		"StartTime":       r.StartTime,
		"RunDurationUsec": r.Duration.AsDuration().Microseconds(),
		"Tags":            r.Tags,
		"TestMetadata":    spanutil.Compressed(pbutil.MustMarshal(r.TestMetadata)),
		"FailureReason":   spanutil.Compressed(pbutil.MustMarshal(r.FailureReason)),
		"Properties":      spanutil.Compressed(pbutil.MustMarshal(r.Properties)),
		"SkipReason":      r.SkipReason,
	}
}

// ToMutation returns a spanner mutation to insert the row.
func (r *TestResultRow) ToMutation() *spanner.Mutation {
	return spanutil.InsertMap("TestResults", r.ToSpannerMap())
}

// ToProto converts the row to a TestResult proto.
func (r *TestResultRow) ToProto() *pb.TestResult {
	return &pb.TestResult{
		TestId:        r.TestID,
		ResultId:      r.ResultID,
		Variant:       r.Variant,
		Expected:      !r.IsUnexpected,
		Status:        r.Status,
		StatusV2:      r.StatusV2,
		SummaryHtml:   r.SummaryHTML,
		StartTime:     r.StartTime,
		Duration:      r.Duration,
		Tags:          r.Tags,
		TestMetadata:  r.TestMetadata,
		FailureReason: r.FailureReason,
		Properties:    r.Properties,
		SkipReason:    r.SkipReason,
	}
}
