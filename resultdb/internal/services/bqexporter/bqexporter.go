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

package bqexporter

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"cloud.google.com/go/bigquery"
	"cloud.google.com/go/spanner"
	"golang.org/x/sync/errgroup"
	"golang.org/x/sync/semaphore"
	"golang.org/x/time/rate"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/option"
	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/server"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/realms"
	"go.chromium.org/luci/server/caching"
	"go.chromium.org/luci/server/experiments"
	"go.chromium.org/luci/server/span"
	"go.chromium.org/luci/server/tq"

	"go.chromium.org/luci/resultdb/internal/invocations"
	"go.chromium.org/luci/resultdb/internal/spanutil"
	"go.chromium.org/luci/resultdb/internal/tasks"
	"go.chromium.org/luci/resultdb/internal/tasks/taskspb"
	"go.chromium.org/luci/resultdb/internal/testresults"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

const partitionExpirationTime = 540 * 24 * time.Hour // ~1.5y

var bqTableCache = caching.RegisterLRUCache(50)

// Options is bqexporter configuration.
type Options struct {
	// How often to query for tasks.
	TaskQueryInterval time.Duration

	// How long to lease a task for.
	TaskLeaseDuration time.Duration

	// Whether to use InsertIDs in BigQuery Streaming Inserts.
	UseInsertIDs bool

	// Maximum number of rows in a batch.
	MaxBatchRowCount int

	// Maximum size of a batch in bytes, approximate.
	MaxBatchSizeApprox int

	// Maximum size of all batches held in memory, approximate.
	MaxBatchTotalSizeApprox int

	// Maximum rate for BigQuery Streaming Inserts.
	RateLimit rate.Limit

	// Number of invocations to export concurrently.
	// This number should be small (e.g. 10) if this ResultDB instance mostly
	// exports huge invocations (10k-100k results per invocation), and it should
	// be large (e.g. 100) if exports small invocations (1000 results per
	// invocation).
	TaskWorkers int
}

// DefaultOptions returns Options with default values.
func DefaultOptions() Options {
	return Options{
		// 500 is recommended
		// https://cloud.google.com/bigquery/quotas#streaming_inserts
		MaxBatchRowCount: 500,
		// HTTP request size limit is 10 MiB according to
		// https://cloud.google.com/bigquery/quotas#streaming_inserts
		// Use a smaller size as the limit since we are only using the size of
		// test results to estimate the whole payload size.
		MaxBatchSizeApprox:      6 * 1024 * 1024,        // 6 MiB
		MaxBatchTotalSizeApprox: 2 * 1024 * 1024 * 1024, // 2 GiB
		RateLimit:               100,
		TaskLeaseDuration:       10 * time.Minute,
		TaskQueryInterval:       5 * time.Second,
		TaskWorkers:             10,
	}
}

type bqExporter struct {
	*Options

	// putLimiter limits the rate of bigquery.Inserter.Put calls.
	putLimiter *rate.Limiter

	// batchSem limits the number of batches we hold in memory at a time.
	//
	// Strictly speaking, this is not the exact number of batches.
	// The exact number is batchSemWeight + taskWorkers*2,
	// but this is good enough.
	batchSem *semaphore.Weighted
}

// Tasks describes how to route bq export tasks.
var Tasks = tq.RegisterTaskClass(tq.TaskClass{
	ID:            "bq-export",
	Prototype:     &taskspb.ExportInvocationToBQ{},
	Kind:          tq.Transactional,
	Queue:         "bqexporter",                 // use a dedicated queue
	RoutingPrefix: "/internal/tasks/bqexporter", // for routing to "bqexporter" service
})

// UseTQ experiment enables using server/tq for bq export tasks.
var UseTQ = experiments.Register("rdb-use-tq-bq-export")

// InitServer initializes a bqexporter server.
func InitServer(srv *server.Server, opts Options) {
	b := &bqExporter{
		Options:    &opts,
		putLimiter: rate.NewLimiter(opts.RateLimit, 1),
		batchSem:   semaphore.NewWeighted(int64(opts.MaxBatchTotalSizeApprox / opts.MaxBatchSizeApprox)),
	}

	d := tasks.Dispatcher{
		Workers:       opts.TaskWorkers,
		LeaseDuration: opts.TaskLeaseDuration,
		QueryInterval: opts.TaskQueryInterval,
	}
	srv.RunInBackground("bqexport", func(ctx context.Context) {
		d.Run(ctx, tasks.BQExport, func(ctx context.Context, invID invocations.ID, payload []byte) error {
			bqExport := &pb.BigQueryExport{}
			if err := proto.Unmarshal(payload, bqExport); err != nil {
				return err
			}
			return b.exportResultsToBigQuery(ctx, invID, bqExport)
		})
	})

	Tasks.AttachHandler(func(ctx context.Context, msg proto.Message) error {
		task := msg.(*taskspb.ExportInvocationToBQ)
		return b.exportResultsToBigQuery(ctx, invocations.ID(task.InvocationId), task.BqExport)
	})
}

// inserter is implemented by bigquery.Inserter.
type inserter interface {
	// Put uploads one or more rows to the BigQuery service.
	Put(ctx context.Context, src interface{}) error
}

// table is implemented by *bigquery.Table.
// See its documentation for description of the methods below.
type table interface {
	FullyQualifiedName() string
	Metadata(ctx context.Context) (md *bigquery.TableMetadata, err error)
	Create(ctx context.Context, md *bigquery.TableMetadata) error
	Update(ctx context.Context, md bigquery.TableMetadataToUpdate, etag string) (*bigquery.TableMetadata, error)
}

// bqInserter is an implementation of inserter.
// It's a wrapper around bigquery.Inserter to retry transient errors that are
// not currently retried by bigquery.Inserter.
type bqInserter struct {
	inserter *bigquery.Inserter
}

// Put implements inserter.
func (i *bqInserter) Put(ctx context.Context, src interface{}) error {
	return retry.Retry(ctx, transient.Only(retry.Default), func() error {
		err := i.inserter.Put(ctx, src)

		// ins.Put has retries for most errors, but it does not retry
		// "http2: stream closed" error. Retry only on that.
		// TODO(nodir): remove this code when https://github.com/googleapis/google-api-go-client/issues/450
		// is fixed.
		if err != nil && strings.Contains(err.Error(), "http2: stream closed") {
			err = transient.Tag.Apply(err)
		}
		return err
	}, retry.LogCallback(ctx, "bigquery_put"))
}

func getLUCIProject(ctx context.Context, invID invocations.ID) (string, error) {
	realm, err := invocations.ReadRealm(span.Single(ctx), invID)
	if err != nil {
		return "", err
	}

	project, _ := realms.Split(realm)
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

// ensureBQTable creates a BQ table if it doesn't exist and updates its schema
// if it is stale.
func ensureBQTable(ctx context.Context, t table) error {
	// Note: creating/updating the table inside GetOrCreate ensures that different
	// goroutines do not attempt to create/update the same table concurrently.
	_, err := bqTableCache.LRU(ctx).GetOrCreate(ctx, t.FullyQualifiedName(), func() (interface{}, time.Duration, error) {
		_, err := t.Metadata(ctx)
		apiErr, ok := err.(*googleapi.Error)
		switch {
		case ok && apiErr.Code == http.StatusNotFound:
			// Table doesn't exist. Create it and cache its existence for 5 minutes.
			return nil, 5 * time.Minute, createBQTable(ctx, t)

		case ok && apiErr.Code == http.StatusForbidden:
			// No read table permission.
			return tasks.PermanentFailure.Apply(err), time.Minute, nil

		case err != nil:
			return nil, 0, err
		}

		// Table exists and is accessible.
		// Ensure its schema is up to date and remember that for 5 minutes.
		return nil, 5 * time.Minute, ensureBQTableFields(ctx, t)
	})

	return err
}

func createBQTable(ctx context.Context, t table) error {
	err := t.Create(ctx, &bigquery.TableMetadata{
		TimePartitioning: &bigquery.TimePartitioning{
			Field:      "partition_time",
			Expiration: partitionExpirationTime,
		},
		Schema: testResultRowSchema,
	})
	apiErr, ok := err.(*googleapi.Error)
	switch {
	case ok && apiErr.Code == http.StatusConflict:
		// Table just got created. This is fine.
		return nil
	case ok && apiErr.Code == http.StatusForbidden:
		// No create table permission.
		return tasks.PermanentFailure.Apply(err)
	case err != nil:
		return err
	default:
		logging.Infof(ctx, "Created BigQuery table %s", t.FullyQualifiedName())
		return nil
	}
}

// ensureBQTableFields adds missing fields to t.
func ensureBQTableFields(ctx context.Context, t table) error {
	return retry.Retry(ctx, transient.Only(retry.Default), func() error {
		// We should retrieve Metadata in a retry loop because of the ETag check
		// below.
		md, err := t.Metadata(ctx)
		if err != nil {
			return err
		}

		combinedSchema := md.Schema

		// Append fields missing in the actual schema.
		mutated := false
		var appendMissing func(schema, newSchema bigquery.Schema) bigquery.Schema
		appendMissing = func(schema, newFields bigquery.Schema) bigquery.Schema {
			indexed := make(map[string]*bigquery.FieldSchema, len(schema))
			for _, c := range schema {
				indexed[c.Name] = c
			}

			for _, newField := range newFields {
				if existingField := indexed[newField.Name]; existingField == nil {
					// The field is missing.
					schema = append(schema, newField)
					mutated = true
				} else {
					existingField.Schema = appendMissing(existingField.Schema, newField.Schema)
				}
			}
			return schema
		}

		// Relax the new fields because we cannot add new required fields.
		combinedSchema = appendMissing(combinedSchema, testResultRowSchema.Relax())
		if !mutated {
			// Nothing to update.
			return nil
		}

		_, err = t.Update(ctx, bigquery.TableMetadataToUpdate{Schema: combinedSchema}, md.ETag)
		apiErr, ok := err.(*googleapi.Error)
		switch {
		case ok && apiErr.Code == http.StatusConflict:
			// ETag became stale since we requested it. Try again.
			return transient.Tag.Apply(err)

		case err != nil:
			return err

		default:
			logging.Infof(ctx, "Updated BigQuery table %s", t.FullyQualifiedName())
			return nil
		}
	}, nil)

}

type testVariantKey struct {
	testID      string
	variantHash string
}

// queryExoneratedTestVariants reads exonerated test variants matching the predicate.
func queryExoneratedTestVariants(ctx context.Context, invIDs invocations.IDSet) (map[testVariantKey]struct{}, error) {
	st := spanner.NewStatement(`
		SELECT DISTINCT TestId, VariantHash,
		FROM TestExonerations
		WHERE InvocationId IN UNNEST(@invIDs)
	`)
	st.Params["invIDs"] = invIDs
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
	exportedID invocations.ID,
	q testresults.Query,
	exoneratedTestVariants map[testVariantKey]struct{},
	batchC chan []*bigquery.StructSaver) error {

	invs, err := invocations.ReadBatch(ctx, q.InvocationIDs)
	if err != nil {
		return err
	}

	rows := make([]*bigquery.StructSaver, 0, b.MaxBatchRowCount)
	batchSize := 0 // Estimated size of rows in bytes.
	rowCount := 0
	err = q.Run(ctx, func(tr *pb.TestResult) error {
		_, exonerated := exoneratedTestVariants[testVariantKey{testID: tr.TestId, variantHash: tr.VariantHash}]
		parentID, _, _ := testresults.MustParseName(tr.Name)
		rows = append(rows, b.generateBQRow(&rowInput{
			exported:   invs[exportedID],
			parent:     invs[parentID],
			tr:         tr,
			exonerated: exonerated,
		}))
		batchSize += proto.Size(tr)
		rowCount++
		if len(rows) >= b.MaxBatchRowCount || batchSize >= b.MaxBatchSizeApprox {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case batchC <- rows:
			}
			rows = make([]*bigquery.StructSaver, 0, b.MaxBatchRowCount)
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

func hasReason(apiErr *googleapi.Error, reason string) bool {
	for _, e := range apiErr.Errors {
		if e.Reason == reason {
			return true
		}
	}
	return false
}

func (b *bqExporter) batchExportRows(ctx context.Context, ins inserter, batchC chan []*bigquery.StructSaver) error {
	eg, ctx := errgroup.WithContext(ctx)
	defer eg.Wait()

	for rows := range batchC {
		rows := rows
		if err := b.batchSem.Acquire(ctx, 1); err != nil {
			return err
		}

		eg.Go(func() error {
			defer b.batchSem.Release(1)
			err := b.insertRowsWithRetries(ctx, ins, rows)
			if apiErr, ok := err.(*googleapi.Error); ok && apiErr.Code == http.StatusForbidden && hasReason(apiErr, "accessDenied") {
				err = tasks.PermanentFailure.Apply(err)
			}
			return err
		})
	}

	return eg.Wait()
}

// insertRowsWithRetries inserts rows into BigQuery.
// Retries on quotaExceeded errors.
func (b *bqExporter) insertRowsWithRetries(ctx context.Context, ins inserter, rows []*bigquery.StructSaver) error {
	if err := b.putLimiter.Wait(ctx); err != nil {
		return err
	}

	return retry.Retry(ctx, quotaErrorIteratorFactory(), func() error {
		err := ins.Put(ctx, rows)

		if bqErr, ok := err.(bigquery.PutMultiError); ok {
			// TODO(nodir): increment a counter.
			logPutMultiError(ctx, bqErr, rows)
		}

		return err
	}, retry.LogCallback(ctx, "bigquery_put"))
}

func logPutMultiError(ctx context.Context, err bigquery.PutMultiError, rows []*bigquery.StructSaver) {
	// Print up to 10 errors.
	for i := 0; i < 10 && i < len(err); i++ {
		tr := rows[err[i].RowIndex].Struct.(*TestResultRow)
		logging.Errorf(ctx, "failed to insert row for %s: %s", tr.Name(), err[i].Error())
	}
	if len(err) > 10 {
		logging.Errorf(ctx, "%d more row insertions failed", len(err)-10)
	}
}

// exportTestResultsToBigQuery queries test results in Spanner then exports them to BigQuery.
func (b *bqExporter) exportTestResultsToBigQuery(ctx context.Context, ins inserter, invID invocations.ID, bqExport *pb.BigQueryExport) error {
	ctx, cancel := span.ReadOnlyTransaction(ctx)
	defer cancel()

	inv, err := invocations.Read(ctx, invID)
	if err != nil {
		return err
	}
	if inv.State != pb.Invocation_FINALIZED {
		return errors.Reason("%s is not finalized yet", invID.Name()).Err()
	}

	// Get the invocation set.
	invIDs, err := invocations.Reachable(ctx, invocations.NewIDSet(invID))
	if err != nil {
		if invocations.TooManyTag.In(err) {
			err = tasks.PermanentFailure.Apply(err)
		}
		return err
	}

	exoneratedTestVariants, err := queryExoneratedTestVariants(ctx, invIDs)
	if err != nil {
		return err
	}

	// Query test results and export to BigQuery.
	batchC := make(chan []*bigquery.StructSaver)

	// Batch exports rows to BigQuery.
	eg, ctx := errgroup.WithContext(ctx)

	eg.Go(func() error {
		return b.batchExportRows(ctx, ins, batchC)
	})

	q := testresults.Query{
		Predicate:     bqExport.GetTestResults().GetPredicate(),
		InvocationIDs: invIDs,
		Mask:          testresults.AllFields,
	}
	eg.Go(func() error {
		defer close(batchC)
		return b.queryTestResults(ctx, invID, q, exoneratedTestVariants, batchC)
	})

	return eg.Wait()
}

// exportResultsToBigQuery exports results of an invocation to a BigQuery table.
func (b *bqExporter) exportResultsToBigQuery(ctx context.Context, invID invocations.ID, bqExport *pb.BigQueryExport) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()

	luciProject, err := getLUCIProject(ctx, invID)
	if err != nil {
		return err
	}

	client, err := getBQClient(ctx, luciProject, bqExport)
	if err != nil {
		return err
	}
	defer client.Close()

	table := client.Dataset(bqExport.Dataset).Table(bqExport.Table)
	if err := ensureBQTable(ctx, table); err != nil {
		return err
	}

	ins := &bqInserter{
		inserter: client.Dataset(bqExport.Dataset).Table(bqExport.Table).Inserter(),
	}
	return b.exportTestResultsToBigQuery(ctx, ins, invID, bqExport)
}

// Schedule schedules tasks for all the given invocation's BigQuery Exports.
func Schedule(ctx context.Context, invID invocations.ID) error {
	var bqExports [][]byte
	if err := invocations.ReadColumns(ctx, invID, map[string]interface{}{"BigqueryExports": &bqExports}); err != nil {
		return err
	}
	for i, buf := range bqExports {
		bqx := &pb.BigQueryExport{}
		if err := proto.Unmarshal(buf, bqx); err != nil {
			return err
		}
		tq.MustAddTask(ctx, &tq.Task{
			Payload: &taskspb.ExportInvocationToBQ{
				BqExport:     bqx,
				InvocationId: string(invID),
			},
			Title: fmt.Sprintf("%s:%d", invID, i),
		})
	}
	return nil
}
