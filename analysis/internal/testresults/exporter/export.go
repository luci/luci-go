// Copyright 2024 The LUCI Authors.
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

// Package exporter contains methods to export test results to BigQuery.
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

// InsertClient defines an interface for inserting rows into BigQuery.
type InsertClient interface {
	// Insert inserts the given rows into BigQuery.
	Insert(ctx context.Context, rows []*bqpb.TestResultRow, dest ExportDestination) error
}

// Exporter provides methods to stream test results to BigQuery.
type Exporter struct {
	client InsertClient
}

// NewExporter instantiates a new Exporter. The given client is used
// to insert rows into BigQuery.
func NewExporter(client InsertClient) *Exporter {
	return &Exporter{client: client}
}

// Options captures context which will be exported
// alongside the test results.
type Options struct {
	RootInvocationID string
	RootRealm        string
	PartitionTime    time.Time
	Parent           *rdbpb.Invocation
	Sources          *pb.Sources
}

// Export exports the test results in the given run verdicts to BigQuery.
func (e *Exporter) Export(ctx context.Context, verdicts []*rdbpb.RunTestVerdict, dest ExportDestination, opts Options) error {
	// Use the same timestamp for all rows exported in the same batch.
	insertTime := clock.Now(ctx)

	rows, err := prepareExportRows(verdicts, opts, insertTime)
	if err != nil {
		return errors.Annotate(err, "prepare rows").Err()
	}

	err = e.client.Insert(ctx, rows, dest)
	if err != nil {
		return errors.Annotate(err, "insert rows").Err()
	}
	return nil
}

// prepareExportRows prepares BigQuery export rows for a
// ResultDB run verdicts.
func prepareExportRows(verdicts []*rdbpb.RunTestVerdict, opts Options, insertTime time.Time) ([]*bqpb.TestResultRow, error) {
	rootProject, _, err := perms.SplitRealm(opts.RootRealm)
	if err != nil {
		return nil, errors.Annotate(err, "invalid root realm").Err()
	}

	sources := opts.Sources
	var sourceRef *pb.SourceRef
	var sourceRefHash string
	if sources != nil {
		sourceRef = pbutil.SourceRefFromSources(sources)
		sourceRefHash = hex.EncodeToString(pbutil.SourceRefHash(sourceRef))
	}

	parent, err := parent(opts.Parent)
	if err != nil {
		return nil, errors.Annotate(err, "parent invocation").Err()
	}

	// Initially allocate enough space for 2 result per run verdict,
	// slice will be re-sized if necessary.
	results := make([]*bqpb.TestResultRow, 0, len(verdicts)*2)

	for _, tv := range verdicts {
		var metadata *pb.TestMetadata
		if tv.TestMetadata != nil {
			metadata = pbutil.TestMetadataFromResultDB(tv.TestMetadata)
		}

		variant, err := bqutil.VariantJSON(tv.Variant)
		if err != nil {
			return nil, errors.Annotate(err, "variant").Err()
		}

		for _, tr := range tv.Results {
			var skipReasonString string
			skipReason := pbutil.SkipReasonFromResultDB(tr.Result.SkipReason)
			if skipReason != pb.SkipReason_SKIP_REASON_UNSPECIFIED {
				skipReasonString = skipReason.String()
			}

			propertiesJSON, err := bqutil.MarshalStructPB(tr.Result.Properties)
			if err != nil {
				return nil, errors.Annotate(err, "marshal properties").Err()
			}

			results = append(results, &bqpb.TestResultRow{
				Project:     rootProject,
				TestId:      tv.TestId,
				Variant:     variant,
				VariantHash: tv.VariantHash,
				Invocation: &bqpb.TestResultRow_InvocationRecord{
					Id:    opts.RootInvocationID,
					Realm: opts.RootRealm,
				},
				PartitionTime: timestamppb.New(opts.PartitionTime),
				Parent:        parent,
				Name:          tr.Result.Name,
				ResultId:      tr.Result.ResultId,
				Expected:      tr.Result.Expected,
				Status:        pbutil.TestResultStatusFromResultDB(tr.Result.Status),
				SummaryHtml:   tr.Result.SummaryHtml,
				StartTime:     tr.Result.StartTime,
				DurationSecs:  tr.Result.Duration.AsDuration().Seconds(),
				Tags:          pbutil.StringPairFromResultDB(tr.Result.Tags),
				FailureReason: pbutil.FailureReasonFromResultDB(tr.Result.FailureReason),
				SkipReason:    skipReasonString,
				Properties:    propertiesJSON,
				Sources:       sources,
				SourceRef:     sourceRef,
				SourceRefHash: sourceRefHash,
				TestMetadata:  metadata,
				InsertTime:    timestamppb.New(insertTime),
			})
		}
	}
	return results, nil
}

func parent(parent *rdbpb.Invocation) (*bqpb.TestResultRow_ParentInvocationRecord, error) {
	invocationID, err := rdbpbutil.ParseInvocationName(parent.Name)
	if err != nil {
		return nil, errors.Annotate(err, "invalid invocation name %q", invocationID).Err()
	}
	propertiesJSON, err := bqutil.MarshalStructPB(parent.Properties)
	if err != nil {
		return nil, errors.Annotate(err, "marshal properties").Err()
	}

	return &bqpb.TestResultRow_ParentInvocationRecord{
		Id:         invocationID,
		Tags:       pbutil.StringPairFromResultDB(parent.Tags),
		Realm:      parent.Realm,
		Properties: propertiesJSON,
	}, nil
}
