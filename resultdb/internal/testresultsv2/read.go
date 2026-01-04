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

	"cloud.google.com/go/spanner"
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
			TestMetadataName,
			TestMetadataLocationRepo,
			TestMetadataLocationFileName,
			FailureReason,
			Properties,
			SkipReason,
			SkippedReason,
			FrameworkExtensions
		FROM TestResultsV2
		ORDER BY RootInvocationShardId, ModuleName, ModuleScheme, ModuleVariantHash, T1CoarseName, T2FineName, T3CaseName, WorkUnitId, ResultId
	`)

	var b spanutil.Buffer
	decoder := &Decoder{}
	err = span.Query(ctx, stmt).Do(func(r *spanner.Row) error {
		var row TestResultRow
		var summaryHTML, testMetadata, failureReason, properties, skippedReason, frameworkExtensions []byte
		var testMetadataName, testMetadataLocationRepo, testMetadataLocationFileName spanner.NullString
		var skipReason spanner.NullInt64
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
			&testMetadataName,
			&testMetadataLocationRepo,
			&testMetadataLocationFileName,
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
		row.SkipReason = DecodeSkipReason(skipReason)

		if row.SummaryHTML, err = decoder.DecompressText(summaryHTML); err != nil {
			return errors.Fmt("decompress SummaryHTML: %w", err)
		}

		if row.TestMetadata, err = decoder.DecodeTestMetadata(testMetadata, testMetadataName, testMetadataLocationRepo, testMetadataLocationFileName); err != nil {
			return errors.Fmt("decode TestMetadata: %w", err)
		}

		if row.FailureReason, err = decoder.DecodeFailureReason(failureReason); err != nil {
			return errors.Fmt("decode FailureReason: %w", err)
		}

		if row.Properties, err = decoder.DecodeProperties(properties); err != nil {
			return errors.Fmt("decode Properties: %w", err)
		}

		if row.SkippedReason, err = decoder.DecodeSkippedReason(skippedReason); err != nil {
			return errors.Fmt("decode SkippedReason: %w", err)
		}

		if row.FrameworkExtensions, err = decoder.DecodeFrameworkExtensions(frameworkExtensions); err != nil {
			return errors.Fmt("decode FrameworkExtensions: %w", err)
		}

		rows = append(rows, &row)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return rows, nil
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
		Duration:            ToProtoDuration(r.RunDurationNanos),
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
