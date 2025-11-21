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

package testresultsv2

import (
	"time"

	"cloud.google.com/go/spanner"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"

	"go.chromium.org/luci/resultdb/internal/rootinvocations"
	"go.chromium.org/luci/resultdb/internal/spanutil"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

// Builder is a builder for TestResultRow for testing.
type Builder struct {
	row TestResultRow
}

// NewBuilder returns a new builder for a TestResultRow for testing.
func NewBuilder() *Builder {
	return &Builder{
		row: TestResultRow{
			ID: ID{
				RootInvocationShardID: rootinvocations.ShardID{
					RootInvocationID: "root-inv-id",
					ShardIndex:       1,
				},
				ModuleName:        "module-name",
				ModuleScheme:      "module-scheme",
				ModuleVariantHash: "variant-hash",
				CoarseName:        "coarse-name",
				FineName:          "fine-name",
				CaseName:          "case-name",
				WorkUnitID:        "work-unit-id",
				ResultID:          "result-id",
			},
			ModuleVariant:    pbutil.Variant("k", "v"),
			CreateTime:       time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
			Realm:            "test-project:test-realm",
			StatusV2:         pb.TestResult_FAILED,
			SummaryHTML:      "summary",
			StartTime:        spanner.NullTime{Time: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC), Valid: true},
			RunDurationNanos: spanner.NullInt64{Int64: 1000, Valid: true},
			Tags:             pbutil.StringPairs("tag", "value"),
			TestMetadata: &pb.TestMetadata{
				Name: "test-name",
			},
			FailureReason: &pb.FailureReason{
				Kind: pb.FailureReason_CRASH,
				Errors: []*pb.FailureReason_Error{
					{
						Message: "error-message",
						Trace:   "error-trace",
					},
				},
			},
			Properties: &structpb.Struct{Fields: map[string]*structpb.Value{"key": structpb.NewStringValue("value")}},
			SkipReason: pb.SkipReason_AUTOMATICALLY_DISABLED_FOR_FLAKINESS,
			SkippedReason: &pb.SkippedReason{
				Kind:          pb.SkippedReason_DISABLED_AT_DECLARATION,
				ReasonMessage: "reason-message",
				Trace:         "trace",
			},
			FrameworkExtensions: &pb.FrameworkExtensions{
				WebTest: &pb.WebTest{
					IsExpected: true,
					Status:     pb.WebTest_FAIL,
				},
			},
		},
	}
}

// WithRootInvocationShardID sets the RootInvocationShardID.
func (b *Builder) WithRootInvocationShardID(id rootinvocations.ShardID) *Builder {
	b.row.ID.RootInvocationShardID = id
	return b
}

// WithModuleName sets the ModuleName.
func (b *Builder) WithModuleName(moduleName string) *Builder {
	b.row.ID.ModuleName = moduleName
	return b
}

// WithModuleScheme sets the ModuleScheme.
func (b *Builder) WithModuleScheme(moduleScheme string) *Builder {
	b.row.ID.ModuleScheme = moduleScheme
	return b
}

// WithModuleVariantHash sets the ModuleVariantHash.
func (b *Builder) WithModuleVariantHash(hash string) *Builder {
	b.row.ID.ModuleVariantHash = hash
	return b
}

// WithCoarseName sets the CoarseName.
func (b *Builder) WithCoarseName(coarseName string) *Builder {
	b.row.ID.CoarseName = coarseName
	return b
}

// WithFineName sets the FineName.
func (b *Builder) WithFineName(fineName string) *Builder {
	b.row.ID.FineName = fineName
	return b
}

// WithCaseName sets the CaseName.
func (b *Builder) WithCaseName(caseName string) *Builder {
	b.row.ID.CaseName = caseName
	return b
}

// WithWorkUnitID sets the WorkUnitID.
func (b *Builder) WithWorkUnitID(workUnitID string) *Builder {
	b.row.ID.WorkUnitID = workUnitID
	return b
}

// WithResultID sets the ResultID.
func (b *Builder) WithResultID(resultID string) *Builder {
	b.row.ID.ResultID = resultID
	return b
}

// WithModuleVariant sets the ModuleVariant.
func (b *Builder) WithModuleVariant(variant *pb.Variant) *Builder {
	b.row.ModuleVariant = variant
	return b
}

// WithCreateTime sets the CreateTime.
func (b *Builder) WithCreateTime(createTime time.Time) *Builder {
	b.row.CreateTime = createTime
	return b
}

// WithRealm sets the Realm.
func (b *Builder) WithRealm(realm string) *Builder {
	b.row.Realm = realm
	return b
}

// WithStatusV2 sets the StatusV2.
func (b *Builder) WithStatusV2(status pb.TestResult_Status) *Builder {
	b.row.StatusV2 = status
	return b
}

// WithSummaryHTML sets the SummaryHTML.
func (b *Builder) WithSummaryHTML(summaryHTML string) *Builder {
	b.row.SummaryHTML = summaryHTML
	return b
}

// WithStartTime sets the StartTime.
func (b *Builder) WithStartTime(startTime time.Time) *Builder {
	b.row.StartTime = spanner.NullTime{Time: startTime, Valid: true}
	return b
}

// WithRunDurationNanos sets the RunDurationNanos.
func (b *Builder) WithRunDurationNanos(runDurationNanos int64) *Builder {
	b.row.RunDurationNanos = spanner.NullInt64{Int64: runDurationNanos, Valid: true}
	return b
}

// WithTags sets the Tags.
func (b *Builder) WithTags(tags []*pb.StringPair) *Builder {
	b.row.Tags = tags
	return b
}

// WithTestMetadata sets the TestMetadata.
func (b *Builder) WithTestMetadata(metadata *pb.TestMetadata) *Builder {
	b.row.TestMetadata = metadata
	return b
}

// WithFailureReason sets the FailureReason.
func (b *Builder) WithFailureReason(reason *pb.FailureReason) *Builder {
	b.row.FailureReason = reason
	return b
}

// WithProperties sets the Properties.
func (b *Builder) WithProperties(properties *structpb.Struct) *Builder {
	b.row.Properties = properties
	return b
}

// WithSkippedReason sets the SkippedReason.
func (b *Builder) WithSkippedReason(skippedReason *pb.SkippedReason) *Builder {
	b.row.SkippedReason = skippedReason
	return b
}

// WithFrameworkExtensions sets the FrameworkExtensions.
func (b *Builder) WithFrameworkExtensions(extensions *pb.FrameworkExtensions) *Builder {
	b.row.FrameworkExtensions = extensions
	return b
}

// Build returns the TestResultRow.
func (b *Builder) Build() *TestResultRow {
	ret := b.row
	// Clone mutable fields to avoid side effects.
	ret.ModuleVariant = proto.Clone(b.row.ModuleVariant).(*pb.Variant)
	if ret.Tags != nil {
		ret.Tags = make([]*pb.StringPair, len(b.row.Tags))
		for i, p := range b.row.Tags {
			ret.Tags[i] = proto.Clone(p).(*pb.StringPair)
		}
	}
	if ret.TestMetadata != nil {
		ret.TestMetadata = proto.Clone(b.row.TestMetadata).(*pb.TestMetadata)
	}
	if ret.FailureReason != nil {
		ret.FailureReason = proto.Clone(b.row.FailureReason).(*pb.FailureReason)
		PopulateFailureReasonOutputOnlyFields(ret.FailureReason)
	}
	if ret.Properties != nil {
		ret.Properties = proto.Clone(b.row.Properties).(*structpb.Struct)
	}
	if ret.SkippedReason != nil {
		ret.SkippedReason = proto.Clone(b.row.SkippedReason).(*pb.SkippedReason)
	}
	if ret.FrameworkExtensions != nil {
		ret.FrameworkExtensions = proto.Clone(b.row.FrameworkExtensions).(*pb.FrameworkExtensions)
	}
	// Make sure the test data is valid by only setting the fields that are allowed for the given status.
	if ret.StatusV2 != pb.TestResult_FAILED {
		ret.FailureReason = nil
	}
	if ret.StatusV2 != pb.TestResult_SKIPPED {
		ret.SkippedReason = nil
	}
	return &ret
}

// InsertForTesting inserts the test result row for testing purposes.
func InsertForTesting(tr *TestResultRow) *spanner.Mutation {
	row := map[string]any{
		"RootInvocationShardId": tr.ID.RootInvocationShardID.RowID(),
		"ModuleName":            tr.ID.ModuleName,
		"ModuleScheme":          tr.ID.ModuleScheme,
		"ModuleVariantHash":     tr.ID.ModuleVariantHash,
		"CoarseName":            tr.ID.CoarseName,
		"FineName":              tr.ID.FineName,
		"CaseName":              tr.ID.CaseName,
		"WorkUnitID":            tr.ID.WorkUnitID,
		"ResultId":              tr.ID.ResultID,
		"ModuleVariant":         tr.ModuleVariant,
		"CreateTime":            tr.CreateTime,
		"Realm":                 tr.Realm,
		"StatusV2":              int64(tr.StatusV2),
		"SummaryHTML":           spanutil.Compressed(tr.SummaryHTML),
		"StartTime":             tr.StartTime,
		"RunDurationNanos":      tr.RunDurationNanos,
		"Tags":                  tr.Tags,
		"TestMetadata":          spanutil.Compressed(pbutil.MustMarshal(tr.TestMetadata)),
		"FailureReason":         spanutil.Compressed(pbutil.MustMarshal(NormaliseFailureReason(tr.FailureReason))),
		"Properties":            spanutil.Compressed(pbutil.MustMarshal(tr.Properties)),
		"SkipReason":            int64(tr.SkipReason),
		"SkippedReason":         spanutil.Compressed(pbutil.MustMarshal(tr.SkippedReason)),
		"FrameworkExtensions":   spanutil.Compressed(pbutil.MustMarshal(tr.FrameworkExtensions)),
	}
	return spanutil.InsertMap("TestResultsV2", row)
}
