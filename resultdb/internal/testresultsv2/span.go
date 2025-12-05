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

	"go.chromium.org/luci/resultdb/internal/spanutil"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

// TestResultRow represents a row in the TestResultsV2 table.
type TestResultRow struct {
	ID                  ID
	ModuleVariant       *pb.Variant
	CreateTime          time.Time
	Realm               string
	StatusV2            pb.TestResult_Status
	SummaryHTML         string
	StartTime           spanner.NullTime
	RunDurationNanos    spanner.NullInt64
	Tags                []*pb.StringPair
	TestMetadata        *pb.TestMetadata
	FailureReason       *pb.FailureReason
	Properties          *structpb.Struct
	SkipReason          pb.SkipReason // Deprecated
	SkippedReason       *pb.SkippedReason
	FrameworkExtensions *pb.FrameworkExtensions
}

// All columns in the TestResult table.
var testResultColumns = []string{
	"RootInvocationShardId",
	"ModuleName",
	"ModuleScheme",
	"ModuleVariantHash",
	"T1CoarseName",
	"T2FineName",
	"T3CaseName",
	"WorkUnitID",
	"ResultId",
	"ModuleVariant",
	"CreateTime",
	"Realm",
	"StatusV2",
	"SummaryHTML",
	"StartTime",
	"RunDurationNanos",
	"Tags",
	"TestMetadata",
	"TestMetadataName",
	"TestMetadataLocationRepo",
	"TestMetadataLocationFileName",
	"FailureReason",
	"Properties",
	"SkipReason",
	"SkippedReason",
	"FrameworkExtensions",
}

// Create creates a mutation to insert a TestResultRow.
func Create(tr *TestResultRow) *spanner.Mutation {
	if err := tr.ID.Validate(); err != nil {
		panic(err)
	}
	if tr.ModuleVariant == nil {
		panic("ModuleVariant is required")
	}
	if tr.Realm == "" {
		panic("Realm is required")
	}

	fr := RemoveOutputOnlyFailureReasonFields(tr.FailureReason)
	tmd := decomposeTestMetadataForWrite(tr.TestMetadata)

	// Rather than use spanutil.InsertMap, use spanner.Insert with
	// cols and vals. This is somewhat less readable but previous profiling
	// work indicates this is noticeably more CPU efficient for row types with
	// high insert volumes.
	vals := []any{
		tr.ID.RootInvocationShardID.RowID(),
		tr.ID.ModuleName,
		tr.ID.ModuleScheme,
		tr.ID.ModuleVariantHash,
		tr.ID.CoarseName,
		tr.ID.FineName,
		tr.ID.CaseName,
		tr.ID.WorkUnitID,
		tr.ID.ResultID,
		pbutil.VariantToStrings(tr.ModuleVariant),
		spanner.CommitTimestamp,
		tr.Realm,
		int64(tr.StatusV2),
		spanutil.Compressed(tr.SummaryHTML).ToSpanner(),
		tr.StartTime,
		tr.RunDurationNanos,
		pbutil.StringPairsToStrings(tr.Tags...),
		spanutil.Compressed(pbutil.MustMarshal(tmd.Remainder)).ToSpanner(),
		tmd.Name,
		tmd.LocationRepo,
		tmd.LocationFileName,
		spanutil.Compressed(pbutil.MustMarshal(fr)).ToSpanner(),
		spanutil.Compressed(pbutil.MustMarshal(tr.Properties)).ToSpanner(),
		int64(tr.SkipReason),
		spanutil.Compressed(pbutil.MustMarshal(tr.SkippedReason)).ToSpanner(),
		spanutil.Compressed(pbutil.MustMarshal(tr.FrameworkExtensions)).ToSpanner(),
	}
	return spanner.InsertOrUpdate("TestResultsV2", testResultColumns, vals)
}

// RemoveOutputOnlyFailureReasonFields removes any fields from the FailureReason
// that are OUTPUT_ONLY. This is necessary for storage.
func RemoveOutputOnlyFailureReasonFields(fr *pb.FailureReason) *pb.FailureReason {
	if fr == nil {
		return nil
	}
	result := proto.Clone(fr).(*pb.FailureReason)
	// Clear the PrimaryErrorMessage field, it is supposed to be output only.
	result.PrimaryErrorMessage = ""
	return result
}

type testMetadataFields struct {
	// The Remainder proto with Name, Location.Repo and Location.FileName cleared.
	Remainder *pb.TestMetadata
	// The Name field of the TestMetadata proto.
	Name spanner.NullString
	// The Location.Repo field of the TestMetadata proto.
	LocationRepo spanner.NullString
	// The Location.FileName field of the TestMetadata proto.
	LocationFileName spanner.NullString
}

// decomposeTestMetadataForWrite decomposes the TestMetadata proto into a set of
// fields that can be stored in the TestResultsV2 table.
func decomposeTestMetadataForWrite(tmd *pb.TestMetadata) testMetadataFields {
	var testMetadataName spanner.NullString
	var testMetadataLocationRepo spanner.NullString
	var testMetadataLocationFileName spanner.NullString

	// Clone the proto to avoid changes propagating back to the caller.
	remainder := proto.Clone(tmd).(*pb.TestMetadata)
	if remainder.GetName() != "" {
		testMetadataName = spanner.NullString{Valid: true, StringVal: remainder.Name}
		// For space efficiency, clear the value in the proto to avoid storing the same value twice.
		remainder.Name = ""
	}
	if remainder.GetLocation() != nil {
		testMetadataLocationRepo = spanner.NullString{Valid: true, StringVal: remainder.Location.Repo}
		testMetadataLocationFileName = spanner.NullString{Valid: true, StringVal: remainder.Location.FileName}
		// For space efficiency, clear the value in the proto to avoid storing the same value twice.
		remainder.Location.Repo = ""
		remainder.Location.FileName = ""
	}
	return testMetadataFields{
		Remainder:        remainder,
		Name:             testMetadataName,
		LocationRepo:     testMetadataLocationRepo,
		LocationFileName: testMetadataLocationFileName,
	}
}
