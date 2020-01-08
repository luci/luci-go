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
	"google.golang.org/api/googleapi"
	"google.golang.org/api/option"

	"go.chromium.org/luci/common/bq"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/sync/parallel"
	"go.chromium.org/luci/server/auth"

	"go.chromium.org/luci/resultdb/internal/span"
	bqpb "go.chromium.org/luci/resultdb/proto/bq/v1"
	pb "go.chromium.org/luci/resultdb/proto/rpc/v1"
)

const (
	maxInvocationGraphSize = 1000
	maxBatchSize           = 500
)

// inserter is implemented by bigquery.Inserter.
type inserter interface {
	// Put uploads one or more rows to the BigQuery service.
	Put(ctx context.Context, src interface{}) error
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

// ensureBQTable creates a BQ table if it doesn't exist.
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

func generateBQRow(invID span.InvocationID, inv *pb.Invocation, tr *pb.TestResult) *bq.Row {
	return &bq.Row{
		Message: &bqpb.TestResultRow{
			Invocation: &bqpb.TestResultRow_Invocation{
				Id:          string(invID),
				Interrupted: inv.Interrupted,
				Tags:        inv.Tags,
			},
			Result: tr,
			Exoneration: &bqpb.TestResultRow_TestExoneration{
				Exonerated: false,
			},
		},
		InsertID: tr.Name,
	}
}

func batchExportRows(ctx context.Context, ins inserter, batchC chan []*bq.Row) error {
	return parallel.WorkPool(10, func(workC chan<- func() error) {
		for rows := range batchC {
			rows := rows
			workC <- func() error {
				return ins.Put(ctx, rows)
			}
		}
	})
}

// exportTestResultsToBigQuery queries test results in Spanner then exports them to BigQuery.
func exportTestResultsToBigQuery(ctx context.Context, ins inserter, invID span.InvocationID, bqExport *pb.BigQueryExport, maxBatchSize int) error {
	txn := span.Client(ctx).ReadOnlyTransaction()
	defer txn.Close()

	inv, err := span.ReadInvocationFull(ctx, txn, invID)
	if err != nil {
		return err
	}
	if inv.State != pb.Invocation_COMPLETED {
		return errors.Reason("%s is not finalized yet", invID.Name()).Err()
	}

	// Get the invocation set.
	invIDs, err := span.ReadReachableInvocations(ctx, txn, maxInvocationGraphSize, span.NewInvocationIDSet(invID))
	if err != nil {
		return err
	}

	// TODO(chanli): Query test exonerations.

	// Query test results and export to BigQuery.
	errC := make(chan error, 1)
	batchC := make(chan []*bq.Row, 20)

	// Batch exports rows to BigQuery.
	ctxStreaming, cancelStreaming := context.WithCancel(ctx)
	go func() {
		err := batchExportRows(ctx, ins, batchC)
		errC <- err
		cancelStreaming()
	}()

	q := span.TestResultQuery{
		Predicate:     bqExport.GetTestResults().GetPredicate(),
		InvocationIDs: invIDs,
	}

	rows := make([]*bq.Row, 0, maxBatchSize)
	err = span.QueryTestResultsStreaming(ctxStreaming, txn, q, func(tr *pb.TestResult) error {
		rows = append(rows, generateBQRow(invID, inv, tr))
		if len(rows) >= maxBatchSize {
			select {
			case <-ctxStreaming.Done():
				return <-errC
			case batchC <- rows:
			}
			rows = make([]*bq.Row, 0, maxBatchSize)
		}
		return nil
	})

	if len(rows) > 0 {
		select {
		case <-ctxStreaming.Done():
		case batchC <- rows:
		}
	}

	close(batchC)

	if err != nil {
		return err
	}

	return <-errC
}

// exportResultsToBigQuery exports results of an invocation to a BigQuery table.
func exportResultsToBigQuery(ctx context.Context, luciProject string, invID span.InvocationID, bqExport *pb.BigQueryExport) error {
	client, err := getBQClient(ctx, luciProject, bqExport)
	if err != nil {
		return err
	}
	defer client.Close()

	if err := ensureBQTable(ctx, client, bqExport); err != nil {
		return err
	}

	ins := client.Dataset(bqExport.Dataset).Table(bqExport.Table).Inserter()
	return exportTestResultsToBigQuery(ctx, ins, invID, bqExport, maxBatchSize)
}
