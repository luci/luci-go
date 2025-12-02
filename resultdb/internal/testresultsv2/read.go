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
	"context"
	"time"

	"cloud.google.com/go/spanner"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/resultdb/internal/spanutil"
	"go.chromium.org/luci/resultdb/internal/tracing"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

// ReadAllForTesting reads all test results from Spanner.
// This function is intended for testing purposes only.
// This method must be called in a transactional Spanner context, e.g. span.Single().
func ReadAllForTesting(ctx context.Context) (rows []*TestResultRow, err error) {
	ctx, ts := tracing.Start(ctx, "go.chromium.org/luci/resultdb/internal/testresultsv2.ReadAll")
	defer func() { tracing.End(ts, err) }()

	stmt := spanner.NewStatement(`
		SELECT
			RootInvocationShardId,
			ModuleName,
			ModuleScheme,
			ModuleVariantHash,
			T1CoarseName,
			T2FineName,
			T3CaseName,
			WorkUnitId,
			ResultId,
			ModuleVariant,
			CreateTime,
			Realm,
			StatusV2,
			SummaryHTML,
			StartTime,
			RunDurationNanos,
			Tags,
			TestMetadata,
			FailureReason,
			Properties,
			SkipReason,
			SkippedReason,
			FrameworkExtensions
		FROM TestResultsV2
		ORDER BY RootInvocationShardId, ModuleName, ModuleScheme, ModuleVariantHash, T1CoarseName, T2FineName, T3CaseName, WorkUnitId, ResultId
	`)

	var b spanutil.Buffer
	err = span.Query(ctx, stmt).Do(func(r *spanner.Row) error {
		var row TestResultRow
		var summaryHTML, testMetadata, failureReason, properties, skippedReason, frameworkExtensions spanutil.Compressed
		var skipReason int64
		var statusV2 int64

		err := b.FromSpanner(r,
			&row.ID.RootInvocationShardID,
			&row.ID.ModuleName,
			&row.ID.ModuleScheme,
			&row.ID.ModuleVariantHash,
			&row.ID.CoarseName,
			&row.ID.FineName,
			&row.ID.CaseName,
			&row.ID.WorkUnitID,
			&row.ID.ResultID,
			&row.ModuleVariant,
			&row.CreateTime,
			&row.Realm,
			&statusV2,
			&summaryHTML,
			&row.StartTime,
			&row.RunDurationNanos,
			&row.Tags,
			&testMetadata,
			&failureReason,
			&properties,
			&skipReason,
			&skippedReason,
			&frameworkExtensions,
		)
		if err != nil {
			return err
		}
		row.StatusV2 = pb.TestResult_Status(statusV2)
		row.SkipReason = pb.SkipReason(skipReason)
		row.SummaryHTML = string(summaryHTML)

		if len(testMetadata) > 0 {
			row.TestMetadata = &pb.TestMetadata{}
			if err := proto.Unmarshal(testMetadata, row.TestMetadata); err != nil {
				return errors.Fmt("unmarshal TestMetadata: %w", err)
			}
		}
		if len(failureReason) > 0 {
			row.FailureReason = &pb.FailureReason{}
			if err := proto.Unmarshal(failureReason, row.FailureReason); err != nil {
				return errors.Fmt("unmarshal FailureReason: %w", err)
			}
			PopulateFailureReasonOutputOnlyFields(row.FailureReason)
		}
		if len(properties) > 0 {
			row.Properties = &structpb.Struct{}
			if err := proto.Unmarshal(properties, row.Properties); err != nil {
				return errors.Fmt("unmarshal Properties: %w", err)
			}
		}
		if len(skippedReason) > 0 {
			row.SkippedReason = &pb.SkippedReason{}
			if err := proto.Unmarshal(skippedReason, row.SkippedReason); err != nil {
				return errors.Fmt("unmarshal SkippedReason: %w", err)
			}
		}
		if len(frameworkExtensions) > 0 {
			row.FrameworkExtensions = &pb.FrameworkExtensions{}
			if err := proto.Unmarshal(frameworkExtensions, row.FrameworkExtensions); err != nil {
				return errors.Fmt("unmarshal FrameworkExtensions: %w", err)
			}
		}

		rows = append(rows, &row)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return rows, nil
}

// PopulateFailureReasonOutputOnlyFields populates output only fields
// for a normalised test result.
func PopulateFailureReasonOutputOnlyFields(fr *pb.FailureReason) {
	if len(fr.Errors) > 0 {
		// Populate PrimaryErrorMessage from Errors collection.
		fr.PrimaryErrorMessage = fr.Errors[0].Message
	} else {
		fr.PrimaryErrorMessage = ""
	}
}

// ToProto converts the TestResultRow to a TestResult proto.
// Note that some internal-only fields are lost in this translation (e.g. CreateTime, Realm, Root Invocation Shard).
func (r *TestResultRow) ToProto() *pb.TestResult {
	testID := pbutil.EncodeTestID(pbutil.BaseTestIdentifier{
		ModuleName:   r.ID.ModuleName,
		ModuleScheme: r.ID.ModuleScheme,
		CoarseName:   r.ID.CoarseName,
		FineName:     r.ID.FineName,
		CaseName:     r.ID.CaseName,
	})

	var startTime *timestamppb.Timestamp
	if r.StartTime.Valid {
		startTime = timestamppb.New(r.StartTime.Time)
	}

	var duration *durationpb.Duration
	if r.RunDurationNanos.Valid {
		duration = durationpb.New(time.Duration(r.RunDurationNanos.Int64))
	}

	statusV1, expected := pbutil.TestStatusV1FromV2(r.StatusV2, r.FailureReason.GetKind(), r.FrameworkExtensions.GetWebTest())

	return &pb.TestResult{
		Name: pbutil.TestResultName(string(r.ID.RootInvocationShardID.RootInvocationID), r.ID.WorkUnitID, testID, r.ID.ResultID),
		TestIdStructured: &pb.TestIdentifier{
			ModuleName:        r.ID.ModuleName,
			ModuleScheme:      r.ID.ModuleScheme,
			ModuleVariant:     r.ModuleVariant,
			ModuleVariantHash: r.ID.ModuleVariantHash,
			CoarseName:        r.ID.CoarseName,
			FineName:          r.ID.FineName,
			CaseName:          r.ID.CaseName,
		},
		TestId:              testID,
		ResultId:            r.ID.ResultID,
		Variant:             r.ModuleVariant,
		Expected:            expected,
		Status:              statusV1,
		StatusV2:            r.StatusV2,
		SummaryHtml:         r.SummaryHTML,
		StartTime:           startTime,
		Duration:            duration,
		Tags:                r.Tags,
		VariantHash:         r.ID.ModuleVariantHash,
		TestMetadata:        r.TestMetadata,
		FailureReason:       r.FailureReason,
		Properties:          r.Properties,
		SkipReason:          r.SkipReason,
		SkippedReason:       r.SkippedReason,
		FrameworkExtensions: r.FrameworkExtensions,
	}
}
