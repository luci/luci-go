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
	"cloud.google.com/go/spanner"
	"golang.org/x/sync/errgroup"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/option"

	"go.chromium.org/luci/common/bq"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/sync/parallel"
	"go.chromium.org/luci/server/auth"

	"go.chromium.org/luci/resultdb/internal"
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

func getLUCIProject(ctx context.Context, invID span.InvocationID) (string, error) {
	realm, err := span.ReadInvocationRealm(ctx, span.Client(ctx).Single(), invID)
	if err != nil {
		return "", err
	}

	project, _, err := internal.ParseRealm(realm)
	if err != nil {
		return "", errors.Annotate(err, "invocation %q", invID.Name()).Err()
	}
	return project, nil
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
	apiErr, ok := err.(*googleapi.Error)
	switch {
	case ok && apiErr.Code == http.StatusNotFound:
		// Table doesn't exist.
		break
	case ok && apiErr.Code == http.StatusForbidden:
		// No read table permission.
		return permanentInvocationTaskErrTag.Apply(err)
	default:
		// Either no err or the err is not special cases above, simply return.
		return err
	}

	// Table doesn't exist. Create one.
	err = t.Create(ctx, nil)
	apiErr, ok = err.(*googleapi.Error)
	switch {
	case err == nil:
		logging.Infof(ctx, "Created BigQuery table %s.%s.%s", bqExport.Project, bqExport.Dataset, bqExport.Table)
		return nil
	case ok && apiErr.Code == http.StatusConflict:
		// Table just got created. This is fine.
		return nil
	case ok && apiErr.Code == http.StatusForbidden:
		// No create table permission.
		return permanentInvocationTaskErrTag.Apply(err)
	default:
		return err
	}
}

type testVariantKey struct {
	testID      string
	variantHash string
}

// queryExoneratedTestVariants reads exonerated test variants matching the predicate.
func queryExoneratedTestVariants(ctx context.Context, txn *spanner.ReadOnlyTransaction, invIDs span.InvocationIDSet) (map[testVariantKey]struct{}, error) {
	st := spanner.NewStatement(`
		SELECT DISTINCT TestId, VariantHash,
		FROM TestExonerations
		WHERE InvocationId IN UNNEST(@invIDs)
	`)
	st.Params["invIDs"] = invIDs

	tvs := map[testVariantKey]struct{}{}
	var b span.Buffer
	err := span.Query(ctx, "exonerated test variants", txn, st, func(row *spanner.Row) error {
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

func generateBQRow(inv *pb.Invocation, tr *pb.TestResult, exonerated bool) *bq.Row {
	return &bq.Row{
		Message: &bqpb.TestResultRow{
			Invocation: &bqpb.TestResultRow_Invocation{
				Id:          string(span.MustParseInvocationName(inv.Name)),
				Interrupted: inv.Interrupted,
				Tags:        inv.Tags,
			},
			Result: tr,
			Exoneration: &bqpb.TestResultRow_TestExoneration{
				Exonerated: exonerated,
			},
		},
		InsertID: tr.Name,
	}
}

func queryTestResultsStreaming(ctx context.Context, txn *spanner.ReadOnlyTransaction, inv *pb.Invocation, q span.TestResultQuery, exoneratedTestVariants map[testVariantKey]struct{}, maxBatchSize int, batchC chan []*bq.Row) error {
	rows := make([]*bq.Row, 0, maxBatchSize)
	err := span.QueryTestResultsStreaming(ctx, txn, q, func(tr *pb.TestResult, variantHash string) error {
		trimTestResultForBigQuery(tr)

		_, exonerated := exoneratedTestVariants[testVariantKey{testID: tr.TestId, variantHash: variantHash}]
		rows = append(rows, generateBQRow(inv, tr, exonerated))
		if len(rows) >= maxBatchSize {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case batchC <- rows:
			}
			rows = make([]*bq.Row, 0, maxBatchSize)
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

	return nil
}

func batchExportRows(ctx context.Context, ins inserter, batchC chan []*bq.Row) error {
	return parallel.WorkPool(10, func(workC chan<- func() error) {
		for rows := range batchC {
			rows := rows
			workC <- func() error {
				err := ins.Put(ctx, rows)
				if apiErr, ok := err.(*googleapi.Error); ok && apiErr.Code == http.StatusForbidden {
					err = permanentInvocationTaskErrTag.Apply(err)
				}
				return err
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
	if inv.State != pb.Invocation_FINALIZED {
		return errors.Reason("%s is not finalized yet", invID.Name()).Err()
	}

	// Get the invocation set.
	invIDs, err := span.ReadReachableInvocations(ctx, txn, maxInvocationGraphSize, span.NewInvocationIDSet(invID))
	if err != nil {
		if span.TooManyInvocationsTag.In(err) {
			err = permanentInvocationTaskErrTag.Apply(err)
		}
		return err
	}

	exoneratedTestVariants, err := queryExoneratedTestVariants(ctx, txn, invIDs)
	if err != nil {
		return err
	}

	// Query test results and export to BigQuery.
	batchC := make(chan []*bq.Row)

	// Batch exports rows to BigQuery.
	eg, ctx := errgroup.WithContext(ctx)

	eg.Go(func() error {
		return batchExportRows(ctx, ins, batchC)
	})

	q := span.TestResultQuery{
		Predicate:         bqExport.GetTestResults().GetPredicate(),
		InvocationIDs:     invIDs,
		SelectVariantHash: true,
	}
	eg.Go(func() error {
		defer close(batchC)
		return queryTestResultsStreaming(ctx, txn, inv, q, exoneratedTestVariants, maxBatchSize, batchC)
	})

	return eg.Wait()
}

// exportResultsToBigQuery exports results of an invocation to a BigQuery table.
func exportResultsToBigQuery(ctx context.Context, invID span.InvocationID, bqExport *pb.BigQueryExport) error {
	luciProject, err := getLUCIProject(ctx, invID)
	if err != nil {
		return err
	}

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

// trimTestResultForBigQuery trims fields that should not go to BigQuery.
func trimTestResultForBigQuery(tr *pb.TestResult) {
	trimArts := func(arts []*pb.Artifact) {
		for _, a := range arts {
			a.FetchUrl = ""
		}
	}
	trimArts(tr.InputArtifacts)
	trimArts(tr.OutputArtifacts)
}
