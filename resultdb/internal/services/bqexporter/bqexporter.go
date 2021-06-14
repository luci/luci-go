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
	"bufio"
	"context"
	"crypto/sha512"
	"encoding/hex"
	"fmt"
	"net/http"
	"time"

	"cloud.google.com/go/bigquery"
	"golang.org/x/sync/errgroup"
	"golang.org/x/sync/semaphore"
	"golang.org/x/time/rate"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/option"
	"google.golang.org/genproto/googleapis/bytestream"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/runtime/protoiface"

	"go.chromium.org/luci/common/bq"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/server"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/realms"
	"go.chromium.org/luci/server/caching"
	"go.chromium.org/luci/server/span"
	"go.chromium.org/luci/server/tq"

	"go.chromium.org/luci/resultdb/internal/artifactcontent"
	"go.chromium.org/luci/resultdb/internal/invocations"
	"go.chromium.org/luci/resultdb/internal/tasks/taskspb"
	pb "go.chromium.org/luci/resultdb/proto/v1"

	// Add support for Spanner transactions in TQ.
	_ "go.chromium.org/luci/server/tq/txn/spanner"
)

const partitionExpirationTime = 540 * 24 * time.Hour // ~1.5y

var bqTableCache = caching.RegisterLRUCache(50)

// Options is bqexporter configuration.
type Options struct {
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

	// ArtifactRBEInstance is the name of the RBE instance to use for artifact
	// storage. Example: "projects/luci-resultdb/instances/artifacts".
	ArtifactRBEInstance string
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

	// Client to read from RBE-CAS.
	rbecasClient bytestream.ByteStreamClient

	// Max size of a token the scanner can buffer when reading artifact content.
	maxTokenSize int
}

// TestResultTasks describes how to route bq test result export tasks.
var TestResultTasks = tq.RegisterTaskClass(tq.TaskClass{
	ID:            "bq-test-result-export",
	Prototype:     &taskspb.ExportInvocationTestResultsToBQ{},
	Kind:          tq.Transactional,
	Queue:         "bqtestresultexports",
	RoutingPrefix: "/internal/tasks/bqexporter",
})

// ArtifactTasks describes how to route bq artifact export tasks.
var ArtifactTasks = tq.RegisterTaskClass(tq.TaskClass{
	ID:            "bq-artifact-export",
	Prototype:     &taskspb.ExportInvocationArtifactsToBQ{},
	Kind:          tq.Transactional,
	Queue:         "bqartifactexports",
	RoutingPrefix: "/internal/tasks/bqexporter",
})

// InitServer initializes a bqexporter server.
func InitServer(srv *server.Server, opts Options) error {
	if opts.ArtifactRBEInstance == "" {
		return errors.Reason("opts.ArtifactRBEInstance is required").Err()
	}

	conn, err := artifactcontent.RBEConn(srv.Context)
	if err != nil {
		return err
	}
	b := &bqExporter{
		Options:      &opts,
		putLimiter:   rate.NewLimiter(opts.RateLimit, 1),
		batchSem:     semaphore.NewWeighted(int64(opts.MaxBatchTotalSizeApprox / opts.MaxBatchSizeApprox)),
		rbecasClient: bytestream.NewByteStreamClient(conn),
		maxTokenSize: bufio.MaxScanTokenSize,
	}
	TestResultTasks.AttachHandler(func(ctx context.Context, msg proto.Message) error {
		task := msg.(*taskspb.ExportInvocationTestResultsToBQ)
		return b.exportResultsToBigQuery(ctx, invocations.ID(task.InvocationId), task.BqExport)
	})
	ArtifactTasks.AttachHandler(func(ctx context.Context, msg proto.Message) error {
		task := msg.(*taskspb.ExportInvocationArtifactsToBQ)
		return b.exportResultsToBigQuery(ctx, invocations.ID(task.InvocationId), task.BqExport)
	})
	return nil
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
func ensureBQTable(ctx context.Context, t table, newSchema bigquery.Schema) error {
	// Note: creating/updating the table inside GetOrCreate ensures that different
	// goroutines do not attempt to create/update the same table concurrently.
	_, err := bqTableCache.LRU(ctx).GetOrCreate(ctx, t.FullyQualifiedName(), func() (interface{}, time.Duration, error) {
		_, err := t.Metadata(ctx)
		apiErr, ok := err.(*googleapi.Error)
		switch {
		case ok && apiErr.Code == http.StatusNotFound:
			// Table doesn't exist. Create it and cache its existence for 5 minutes.
			return nil, 5 * time.Minute, errors.Annotate(createBQTable(ctx, t, newSchema), "create bq table").Err()

		case ok && apiErr.Code == http.StatusForbidden:
			// No read table permission.
			return tq.Fatal.Apply(err), time.Minute, nil

		case err != nil:
			return nil, 0, err
		}

		// Table exists and is accessible.
		// Ensure its schema is up to date and remember that for 5 minutes.
		err = ensureBQTableFields(ctx, t, newSchema)
		return nil, 5 * time.Minute, errors.Annotate(err, "ensure bq table fields").Err()
	})

	return err
}

func createBQTable(ctx context.Context, t table, newSchema bigquery.Schema) error {
	err := t.Create(ctx, &bigquery.TableMetadata{
		TimePartitioning: &bigquery.TimePartitioning{
			Field:      "partition_time",
			Expiration: partitionExpirationTime,
		},
		Schema: newSchema,
	})
	apiErr, ok := err.(*googleapi.Error)
	switch {
	case ok && apiErr.Code == http.StatusConflict:
		// Table just got created. This is fine.
		return nil
	case ok && apiErr.Code == http.StatusForbidden:
		// No create table permission.
		return tq.Fatal.Apply(err)
	case err != nil:
		return err
	default:
		logging.Infof(ctx, "Created BigQuery table %s", t.FullyQualifiedName())
		return nil
	}
}

// ensureBQTableFields adds missing fields to t.
func ensureBQTableFields(ctx context.Context, t table, newSchema bigquery.Schema) error {
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
		combinedSchema = appendMissing(combinedSchema, newSchema)
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

func hasReason(apiErr *googleapi.Error, reason string) bool {
	for _, e := range apiErr.Errors {
		if e.Reason == reason {
			return true
		}
	}
	return false
}

// rowInput is information required to generate a BigQuery row.
type rowInput interface {
	// row returns a BigQuery row.
	row() protoiface.MessageV1

	// id returns an identifier for the row.
	id() []byte
}

func (b *bqExporter) batchExportRows(ctx context.Context, ins inserter, batchC chan []rowInput, errorLogger func(ctx context.Context, err bigquery.PutMultiError, rows []*bq.Row)) error {
	eg, ctx := errgroup.WithContext(ctx)
	defer eg.Wait()

	for rows := range batchC {
		rows := rows
		if err := b.batchSem.Acquire(ctx, 1); err != nil {
			return err
		}

		eg.Go(func() error {
			defer b.batchSem.Release(1)
			err := b.insertRowsWithRetries(ctx, ins, rows, errorLogger)
			if apiErr, ok := err.(*googleapi.Error); ok && apiErr.Code == http.StatusForbidden && hasReason(apiErr, "accessDenied") {
				err = tq.Fatal.Apply(err)
			}
			return err
		})
	}

	return eg.Wait()
}

// insertRowsWithRetries inserts rows into BigQuery.
// Retries on quotaExceeded errors.
func (b *bqExporter) insertRowsWithRetries(ctx context.Context, ins inserter, rowInputs []rowInput, errorLogger func(ctx context.Context, err bigquery.PutMultiError, rows []*bq.Row)) error {
	if err := b.putLimiter.Wait(ctx); err != nil {
		return err
	}

	rows := make([]*bq.Row, 0, len(rowInputs))
	for _, ri := range rowInputs {
		row := &bq.Row{Message: ri.row()}

		if b.UseInsertIDs {
			// InsertID cannot exceed 128 bytes.
			// https://cloud.google.com/bigquery/quotas#streaming_inserts
			// Use SHA512 which is exactly 128 bytes in hex.
			hash := sha512.Sum512(ri.id())
			row.InsertID = hex.EncodeToString(hash[:])
		} else {
			row.InsertID = bigquery.NoDedupeID
		}
		rows = append(rows, row)
	}

	return retry.Retry(ctx, quotaErrorIteratorFactory(), func() error {
		err := ins.Put(ctx, rows)

		if bqErr, ok := err.(bigquery.PutMultiError); ok {
			// TODO(nodir): increment a counter.
			errorLogger(ctx, bqErr, rows)
		}

		return err
	}, retry.LogCallback(ctx, "bigquery_put"))
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
		return errors.Annotate(err, "new bq client").Err()
	}
	defer client.Close()

	table := client.Dataset(bqExport.Dataset).Table(bqExport.Table)
	ins := table.Inserter()

	switch bqExport.ResultType.(type) {
	case *pb.BigQueryExport_TestResults_:
		if err := ensureBQTable(ctx, table, testResultRowSchema.Relax()); err != nil {
			return errors.Annotate(err, "ensure test results bq table").Err()
		}
		return errors.Annotate(b.exportTestResultsToBigQuery(ctx, ins, invID, bqExport), "export test results").Err()
	case *pb.BigQueryExport_TextArtifacts_:
		if err := ensureBQTable(ctx, table, textArtifactRowSchema.Relax()); err != nil {
			return errors.Annotate(err, "ensure text artifacts bq table").Err()
		}
		return errors.Annotate(b.exportTextArtifactsToBigQuery(ctx, ins, invID, bqExport), "export text artifacts").Err()
	case nil:
		return fmt.Errorf("bqExport.ResultType is unspecified")
	default:
		panic("impossible")
	}
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
		switch bqx.ResultType.(type) {
		case *pb.BigQueryExport_TestResults_:
			tq.MustAddTask(ctx, &tq.Task{
				Payload: &taskspb.ExportInvocationTestResultsToBQ{
					BqExport:     bqx,
					InvocationId: string(invID),
				},
				Title: fmt.Sprintf("%s:%d", invID, i),
			})
		case *pb.BigQueryExport_TextArtifacts_:
			tq.MustAddTask(ctx, &tq.Task{
				Payload: &taskspb.ExportInvocationArtifactsToBQ{
					BqExport:     bqx,
					InvocationId: string(invID),
				},
				Title: fmt.Sprintf("%s:%d", invID, i),
			})
		default:
			return errors.Reason("bqexport.ResultType is required").Err()
		}

	}
	return nil
}
