// Copyright 2026 The LUCI Authors.
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

package exporter

import (
	"context"
	"encoding/hex"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	rdbpbutil "go.chromium.org/luci/resultdb/pbutil"
	rdbpb "go.chromium.org/luci/resultdb/proto/v1"

	"go.chromium.org/luci/analysis/internal/bqutil"
	"go.chromium.org/luci/analysis/internal/perms"
	"go.chromium.org/luci/analysis/pbutil"
	bqpb "go.chromium.org/luci/analysis/proto/bq"
	pb "go.chromium.org/luci/analysis/proto/v1"
)

// LegacyOptions captures context which will be exported
// alongside the test results.
type LegacyOptions struct {
	// ExportRootInvocationID is the ID of the legacy invocation ID under
	// which the test results are being exported.
	ExportRootInvocationID string
	// RootRealm is the realm of the export root invocation.
	RootRealm     string
	PartitionTime time.Time
	// The immediate parent invocation.
	Parent  *rdbpb.Invocation
	Sources *pb.Sources
}

// ExportLegacy exports the test results in the given run verdicts to BigQuery.
func (e *Exporter) ExportLegacy(ctx context.Context, verdicts []*rdbpb.RunTestVerdict, dest ExportDestination, opts LegacyOptions) error {
	// Use the same timestamp for all rows exported in the same batch.
	insertTime := clock.Now(ctx)

	rows, err := prepareExportRowsLegacy(verdicts, opts, insertTime)
	if err != nil {
		return errors.Fmt("prepare rows: %w", err)
	}

	err = e.client.Insert(ctx, rows, dest)
	if err != nil {
		return errors.Fmt("insert rows: %w", err)
	}
	return nil
}

// prepareExportRowsLegacy prepares BigQuery export rows for a
// ResultDB run verdicts.
func prepareExportRowsLegacy(verdicts []*rdbpb.RunTestVerdict, opts LegacyOptions, insertTime time.Time) ([]*bqpb.TestResultRow, error) {
	rootProject, _, err := perms.SplitRealm(opts.RootRealm)
	if err != nil {
		return nil, errors.Fmt("invalid root realm: %w", err)
	}

	sources := opts.Sources
	var sourceRef *pb.SourceRef
	var sourceRefHash string
	if sources != nil {
		sourceRef = pbutil.SourceRefFromSources(sources)
		if sourceRef != nil {
			sourceRefHash = hex.EncodeToString(pbutil.SourceRefHash(sourceRef))
		}
	}

	parent, err := parentFromInvocation(opts.Parent)
	if err != nil {
		return nil, errors.Fmt("parent invocation: %w", err)
	}

	// Initially allocate enough space for 2 result per run verdict,
	// slice will be re-sized if necessary.
	results := make([]*bqpb.TestResultRow, 0, len(verdicts)*2)

	for _, tv := range verdicts {
		variant, err := bqutil.VariantJSON(tv.Variant)
		if err != nil {
			return nil, errors.Fmt("variant: %w", err)
		}

		testIDStructured, err := bqutil.StructuredTestIdentifierRDB(tv.TestId, tv.Variant)
		if err != nil {
			return nil, errors.Fmt("parse structured test id: %w", err)
		}

		for _, tr := range tv.Results {
			var skipReasonString string
			if tr.Result.SkipReason != rdbpb.SkipReason_SKIP_REASON_UNSPECIFIED {
				skipReasonString = tr.Result.SkipReason.String()
			}

			propertiesJSON, err := bqutil.MarshalStructPB(tr.Result.Properties)
			if err != nil {
				return nil, errors.Fmt("marshal properties: %w", err)
			}

			tmd, err := bqutil.TestMetadata(tv.TestMetadata)
			if err != nil {
				return nil, errors.Fmt("prepare test metadata: %w", err)
			}

			results = append(results, &bqpb.TestResultRow{
				Project:          rootProject,
				TestIdStructured: testIDStructured,
				TestId:           tv.TestId,
				Variant:          variant,
				VariantHash:      tv.VariantHash,
				Invocation: &bqpb.TestResultRow_InvocationRecord{
					Id:    opts.ExportRootInvocationID,
					Realm: opts.RootRealm,
				},
				PartitionTime:       timestamppb.New(opts.PartitionTime),
				Parent:              parent,
				Name:                tr.Result.Name,
				ResultId:            tr.Result.ResultId,
				Expected:            tr.Result.Expected,
				Status:              pbutil.LegacyTestStatusFromResultDB(tr.Result.Status),
				StatusV2:            pbutil.TestStatusV2FromResultDB(tr.Result.StatusV2),
				SummaryHtml:         tr.Result.SummaryHtml,
				StartTime:           tr.Result.StartTime,
				DurationSecs:        tr.Result.Duration.AsDuration().Seconds(),
				Tags:                pbutil.StringPairFromResultDB(tr.Result.Tags),
				FailureReason:       tr.Result.FailureReason,
				SkipReason:          skipReasonString,
				Properties:          propertiesJSON,
				Sources:             sources,
				SourceRef:           sourceRef,
				SourceRefHash:       sourceRefHash,
				TestMetadata:        tmd,
				SkippedReason:       tr.Result.SkippedReason,
				FrameworkExtensions: tr.Result.FrameworkExtensions,
				InsertTime:          timestamppb.New(insertTime),
			})
		}
	}
	return results, nil
}

func parentFromInvocation(parent *rdbpb.Invocation) (*bqpb.TestResultRow_ParentRecord, error) {
	invocationID, err := rdbpbutil.ParseInvocationName(parent.Name)
	if err != nil {
		return nil, errors.Fmt("invalid invocation name %q: %w", invocationID, err)
	}
	propertiesJSON, err := bqutil.MarshalStructPB(parent.Properties)
	if err != nil {
		return nil, errors.Fmt("marshal properties: %w", err)
	}

	return &bqpb.TestResultRow_ParentRecord{
		Id:         invocationID,
		Tags:       pbutil.StringPairFromResultDB(parent.Tags),
		Realm:      parent.Realm,
		Properties: propertiesJSON,
	}, nil
}
