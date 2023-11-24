// Copyright 2020 The LUCI Authors.
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

package bqexporter

import (
	"context"

	"cloud.google.com/go/bigquery"
	"cloud.google.com/go/spanner"
	"github.com/golang/protobuf/descriptor"
	desc "github.com/golang/protobuf/protoc-gen-go/descriptor"
	"golang.org/x/sync/errgroup"
	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/bq"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/resultdb/internal/invocations"
	"go.chromium.org/luci/resultdb/internal/invocations/graph"
	"go.chromium.org/luci/resultdb/internal/spanutil"
	"go.chromium.org/luci/resultdb/internal/testresults"
	"go.chromium.org/luci/resultdb/pbutil"
	bqpb "go.chromium.org/luci/resultdb/proto/bq"
	pb "go.chromium.org/luci/resultdb/proto/v1"
	"go.chromium.org/luci/server/span"
)

var testResultRowSchema bigquery.Schema

const testResultRowMessage = "luci.resultdb.bq.TestResultRow"

func init() {
	var err error
	if testResultRowSchema, err = generateTestResultRowSchema(); err != nil {
		panic(err)
	}
}

func generateTestResultRowSchema() (schema bigquery.Schema, err error) {
	fd, _ := descriptor.MessageDescriptorProto(&bqpb.TestResultRow{})
	// We also need to get FileDescriptorProto for StringPair, TestMetadata, Sources and FailureReason
	// because they are defined in different files.
	fdsp, _ := descriptor.MessageDescriptorProto(&pb.StringPair{})
	fdtmd, _ := descriptor.MessageDescriptorProto(&pb.TestMetadata{})
	fds, _ := descriptor.MessageDescriptorProto(&pb.Sources{})
	fdfr, _ := descriptor.MessageDescriptorProto(&pb.FailureReason{})
	fdinv, _ := descriptor.MessageDescriptorProto(&bqpb.InvocationRecord{})
	fdset := &desc.FileDescriptorSet{File: []*desc.FileDescriptorProto{fd, fdsp, fdtmd, fds, fdfr, fdinv}}
	return generateSchema(fdset, testResultRowMessage)
}

// Row size limit is 5MB according to
// https://cloud.google.com/bigquery/quotas#streaming_inserts
// Cap the summaryHTML's length to 4MB to ensure the row size is under
// limit.
const maxSummaryLength = 4e6

func invocationProtoToRecord(inv *pb.Invocation) *bqpb.InvocationRecord {
	return &bqpb.InvocationRecord{
		Id:         string(invocations.MustParseName(inv.Name)),
		Tags:       inv.Tags,
		Properties: inv.Properties,
		Realm:      inv.Realm,
	}
}

// testResultRowInput is information required to generate a TestResult BigQuery row.
type testResultRowInput struct {
	exported   *pb.Invocation
	parent     *pb.Invocation
	tr         *pb.TestResult
	sources    *pb.Sources
	exonerated bool
}

func (i *testResultRowInput) row() proto.Message {
	tr := i.tr

	ret := &bqpb.TestResultRow{
		Exported:      invocationProtoToRecord(i.exported),
		Parent:        invocationProtoToRecord(i.parent),
		Name:          tr.Name,
		TestId:        tr.TestId,
		ResultId:      tr.ResultId,
		Variant:       pbutil.VariantToStringPairs(tr.Variant),
		VariantHash:   tr.VariantHash,
		Expected:      tr.Expected,
		Status:        tr.Status.String(),
		SummaryHtml:   tr.SummaryHtml,
		StartTime:     tr.StartTime,
		Duration:      tr.Duration,
		Tags:          tr.Tags,
		Exonerated:    i.exonerated,
		Sources:       i.sources,
		PartitionTime: i.exported.CreateTime,
		TestMetadata:  tr.TestMetadata,
		FailureReason: tr.FailureReason,
		Properties:    tr.Properties,
	}

	if tr.Status == pb.TestStatus_SKIP {
		ret.SkipReason = tr.SkipReason.String()
	}

	if len(ret.SummaryHtml) > maxSummaryLength {
		ret.SummaryHtml = "[Trimmed] " + ret.SummaryHtml[:maxSummaryLength]
	}

	return ret
}

func (i *testResultRowInput) id() []byte {
	return []byte(i.tr.Name)
}

type testVariantKey struct {
	testID      string
	variantHash string
}

// queryExoneratedTestVariants reads exonerated test variants matching the predicate.
func queryExoneratedTestVariants(ctx context.Context, invs invocations.IDSet) (map[testVariantKey]struct{}, error) {
	st := spanner.NewStatement(`
		SELECT DISTINCT TestId, VariantHash,
		FROM TestExonerations
		WHERE InvocationId IN UNNEST(@invIDs)
	`)
	st.Params["invIDs"] = invs
	tvs := map[testVariantKey]struct{}{}
	var b spanutil.Buffer
	err := spanutil.Query(ctx, st, func(row *spanner.Row) error {
		var key testVariantKey
		if err := b.FromSpanner(row, &key.testID, &key.variantHash); err != nil {
			return err
		}
		tvs[key] = struct{}{}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return tvs, nil
}

func (b *bqExporter) queryTestResults(
	ctx context.Context,
	reachableInvs graph.ReachableInvocations,
	exported *pb.Invocation,
	predicate *pb.TestResultPredicate,
	exoneratedTestVariants map[testVariantKey]struct{},
	batchC chan []rowInput) error {
	invocationIds, err := reachableInvs.WithTestResultsIDSet()
	if err != nil {
		return err
	}
	q := testresults.Query{
		Predicate:     predicate,
		InvocationIDs: invocationIds,
		Mask:          testresults.AllFields,
	}

	invs, err := invocations.ReadBatch(ctx, invocationIds)
	if err != nil {
		return err
	}

	rows := make([]rowInput, 0, b.MaxBatchRowCount)
	batchSize := 0 // Estimated size of rows in bytes.
	rowCount := 0
	err = q.Run(ctx, func(tr *pb.TestResult) error {
		_, exonerated := exoneratedTestVariants[testVariantKey{testID: tr.TestId, variantHash: tr.VariantHash}]
		parentID, _, _ := testresults.MustParseName(tr.Name)
		sourceHash := reachableInvs.Invocations[parentID].SourceHash
		var sources *pb.Sources
		if sourceHash != graph.EmptySourceHash {
			sources = reachableInvs.Sources[sourceHash]
		}

		rows = append(rows, &testResultRowInput{
			exported:   exported,
			parent:     invs[parentID],
			tr:         tr,
			sources:    sources,
			exonerated: exonerated,
		})
		batchSize += proto.Size(tr)
		rowCount++
		if len(rows) >= b.MaxBatchRowCount || batchSize >= b.MaxBatchSizeApprox {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case batchC <- rows:
			}
			rows = make([]rowInput, 0, b.MaxBatchRowCount)
			batchSize = 0
		}
		return nil
	})

	if err != nil {
		return err
	}

	if len(rows) > 0 {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case batchC <- rows:
		}
	}

	// Log the number of fetched rows so that later we can compare it to
	// the value in QueryTestResultsStatistics. This is to help debugging
	// crbug.com/1090671.
	logging.Debugf(ctx, "fetched %d rows for invocations %q", rowCount, q.InvocationIDs)
	return nil
}

// exportTestResultsToBigQuery queries test results in Spanner then exports them to BigQuery.
func (b *bqExporter) exportTestResultsToBigQuery(ctx context.Context, ins inserter, invID invocations.ID, bqExport *pb.BigQueryExport) error {
	ctx, cancel := span.ReadOnlyTransaction(ctx)
	defer cancel()

	exported, err := invocations.Read(ctx, invID)
	if err != nil {
		return err
	}
	if exported.State != pb.Invocation_FINALIZED {
		return errors.Reason("%s is not finalized yet", invID.Name()).Err()
	}

	invs, err := graph.Reachable(ctx, invocations.NewIDSet(invID))
	if err != nil {
		return errors.Annotate(err, "querying reachable invocations").Err()
	}

	exonerationInvocationIds, err := invs.WithExonerationsIDSet()
	if err != nil {
		return err
	}
	exoneratedTestVariants, err := queryExoneratedTestVariants(ctx, exonerationInvocationIds)
	if err != nil {
		return errors.Annotate(err, "query exoneration").Err()
	}

	// Query test results in batches of invocations.
	for _, batch := range invs.Batches() {
		// Within each batch of invocations, batch the querying of
		// test results and export to BigQuery.
		batchC := make(chan []rowInput)

		// Batch exports rows to BigQuery.
		eg, ctx := errgroup.WithContext(ctx)

		eg.Go(func() error {
			return b.batchExportRows(ctx, ins, batchC, func(ctx context.Context, err bigquery.PutMultiError, rows []*bq.Row) {
				// Print up to 10 errors.
				for i := 0; i < 10 && i < len(err); i++ {
					tr := rows[err[i].RowIndex].Message.(*bqpb.TestResultRow)
					logging.Errorf(ctx, "failed to insert row for %s: %s", pbutil.TestResultName(tr.Parent.Id, tr.TestId, tr.ResultId), err[i].Error())
				}
				if len(err) > 10 {
					logging.Errorf(ctx, "%d more row insertions failed", len(err)-10)
				}
			})
		})

		eg.Go(func() error {
			defer close(batchC)
			predicate := bqExport.GetTestResults().GetPredicate()
			return b.queryTestResults(ctx, batch, exported, predicate, exoneratedTestVariants, batchC)
		})

		if err := eg.Wait(); err != nil {
			return errors.Annotate(err, "exporting batch").Err()
		}
	}
	return nil
}
