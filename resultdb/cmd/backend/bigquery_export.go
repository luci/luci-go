// Copyright 2019 The LUCI Authors.
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

package main

import (
	"context"
	"net/http"

	"cloud.google.com/go/bigquery"
	"github.com/golang/protobuf/proto"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/option"

	"go.chromium.org/luci/common/bq"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/server/auth"

	"go.chromium.org/luci/resultdb/internal/span"
	bqpb "go.chromium.org/luci/resultdb/proto/bq/v1"
	pb "go.chromium.org/luci/resultdb/proto/rpc/v1"
)

const (
	maxInvocationGraphSize = 1000
	maxRowsInBatch         = 8000
)

type Uploader interface {
	// Put uploads one or more rows to the BigQuery service.
	Put(ctx context.Context, messages ...proto.Message) error
}

func getBQClient(ctx context.Context, luciProject string, bqExport *pb.BigQueryExport) (*bigquery.Client, error) {
	tr, err := auth.GetRPCTransport(ctx, auth.AsProject, auth.WithProject(luciProject), auth.WithScopes(bigquery.Scope))
	if err != nil {
		return nil, err
	}

	return bigquery.NewClient(ctx, bqExport.Project, option.WithHTTPClient(&http.Client{
		Transport: tr,
	}))
}

// ensureBQTable checks the existence of a BQ table, and create one if it doesn't exist.
func ensureBQTable(ctx context.Context, client *bigquery.Client, bqExport *pb.BigQueryExport) error {
	t := client.Dataset(bqExport.Dataset).Table(bqExport.Table)

	// Check the existence of table.
	// TODO(chanli): Cache the check result.
	_, err := t.Metadata(ctx)
	if apiErr, ok := err.(*googleapi.Error); !(ok && apiErr.Code == http.StatusNotFound) {
		return err
	}

	// Table doesn't exist. Create one.
	err = t.Create(ctx, nil)
	if apiErr, ok := err.(*googleapi.Error); ok && apiErr.Code == http.StatusConflict {
		// Table just got created. This is fine.
		return nil
	}
	if err != nil {
		return err
	}
	logging.Infof(ctx, "Created BigQuery table %s.%s.%s", bqExport.Project, bqExport.Dataset, bqExport.Table)
	return nil
}

func isInvocationFinalized(ctx context.Context, txn span.Txn, invID span.InvocationID) error {
	var state pb.Invocation_State
	err := span.ReadInvocation(ctx, txn, invID, map[string]interface{}{
		"State": &state,
	})

	if err != nil {
		return err
	}

	if state != pb.Invocation_COMPLETED {
		return errors.Reason("%s is not finalized yet", invID).Err()
	}
	return nil
}

func generateBQRow(invID span.InvocationID, inv *pb.Invocation, tr *pb.TestResult) *bqpb.TestResultRow {
	return &bqpb.TestResultRow{
		Invocation: &bqpb.TestResultRow_Invocation{
			Id:          string(invID),
			Interrupted: inv.Interrupted,
			Tags:        inv.Tags,
		},
		Result: tr,
		Exoneration: &bqpb.TestResultRow_TestExoneration{
			Exonerated: false,
		},
	}
}

func batchExportRows(ctx context.Context, up Uploader, maxRowsInBatch int, buffResults chan *bqpb.TestResultRow, queryDone chan bool, upDone chan bool, errc chan error) {
	errMulti := errors.MultiError(nil)
	done := false
	for {
		select {
		case d := <-queryDone:
			done = d
		default:
		}
		var rows []proto.Message
		// Block buffResults channel.
		rows = append(rows, <-buffResults)

	InnerLoop:
		for i := 1; i < maxRowsInBatch; i++ {
			select {
			case row := <-buffResults:
				rows = append(rows, row)
			default:
				break InnerLoop
			}
		}

		// Export this batch of rows.
		err := up.Put(ctx, rows...)
		if err != nil {
			errMulti = append(errMulti, err)
		}

		if done && len(buffResults) <= 0 {
			upDone <- true
			errc <- errMulti
			close(errc)
		}
	}
}

// exportTestResultsToBigQuery queries test results on Spanner then exports them to BigQuery.
func exportTestResultsToBigQuery(ctx context.Context, up Uploader, invID span.InvocationID, bqExport *pb.BigQueryExport, maxRowsInBatch int) error {
	txn := span.Client(ctx).ReadOnlyTransaction()
	defer txn.Close()

	if err := isInvocationFinalized(ctx, txn, invID); err != nil {
		return err
	}

	// Get the invocations set.
	invIDs, err := span.ReadReachableInvocations(ctx, txn, maxInvocationGraphSize, span.NewInvocationIDSet(invID))
	if err != nil {
		return err
	}

	invs, err := span.ReadInvocationsFull(ctx, txn, invIDs)
	if err != nil {
		return err
	}

	// TODO(chanli): Query test exonerations.

	// Query test results and export to BigQuery.
	buffResults := make(chan *bqpb.TestResultRow, maxRowsInBatch)
	queryDone := make(chan bool)
	errc := make(chan error)
	upDone := make(chan bool)

	// Batch exports rows to BigQuery.
	go batchExportRows(ctx, up, maxRowsInBatch, buffResults, queryDone, upDone, errc)

	q := span.TestResultQuery{
		Predicate:     bqExport.GetTestResults().GetPredicate(),
		InvocationIDs: invIDs,
	}

	err = span.QueryTestResultsStreaming(ctx, txn, q, func(tr *pb.TestResult) error {
		buffResults <- generateBQRow(invID, invs[invID], tr)
		return nil
	})

	queryDone <- true
	close(buffResults)

	if err != nil {
		return err
	}

	<-upDone
	if err = <-errc; err != nil {
		return err
	}

	return nil
}

// ExportTestResultsToBigQuery export test results of an invocation to a BigQuery table.
func ExportTestResultsToBigQuery(ctx context.Context, luciProject string, invID span.InvocationID, bqExport *pb.BigQueryExport) error {
	client, err := getBQClient(ctx, luciProject, bqExport)
	if err != nil {
		return err
	}
	defer client.Close()

	if err := ensureBQTable(ctx, client, bqExport); err != nil {
		return err
	}

	up := bq.NewUploader(ctx, client, bqExport.Dataset, bqExport.Table)
	up.SkipInvalidRows = true
	up.IgnoreUnknownValues = true

	return exportTestResultsToBigQuery(ctx, up, invID, bqExport, maxRowsInBatch)
}
