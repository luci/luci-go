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
	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"golang.org/x/sync/errgroup"
	"golang.org/x/sync/semaphore"
	"golang.org/x/time/rate"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/option"
	"google.golang.org/genproto/googleapis/bytestream"
	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/bq"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/server"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/realms"
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

// schemaApplyer ensures BQ schema matches the row proto definitons.
var schemaApplyer = bq.NewSchemaApplyer(bq.RegisterSchemaApplyerCache(50))

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

// InvocationTasks describes how to route bq invocation export tasks.
var InvocationTasks = tq.RegisterTaskClass(tq.TaskClass{
	ID:            "bq-invocation-export",
	Prototype:     &taskspb.ExportInvocationToBQ{},
	Kind:          tq.Transactional,
	Queue:         "bqinvocationexports",
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

	invClient, err := NewInvClient(srv.Context, srv.Options.CloudProject)
	if err != nil {
		return errors.Annotate(err, "create invocation export client").Err()
	}

	srv.RegisterCleanup(func(ctx context.Context) {
		err := conn.Close()
		if err != nil {
			logging.Errorf(ctx, "Cleaning up artifact RBE connection: %s", err)
		}
		err = invClient.Close()
		if err != nil {
			logging.Errorf(ctx, "Cleaning up invocation export client: %s", err)
		}
	})

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
	InvocationTasks.AttachHandler(func(ctx context.Context, msg proto.Message) error {
		task := msg.(*taskspb.ExportInvocationToBQ)
		return b.exportInvocationToBigQuery(ctx, invocations.ID(task.InvocationId), invClient)
	})
	return nil
}

// inserter is implemented by bigquery.Inserter.
type inserter interface {
	// Put uploads one or more rows to the BigQuery service.
	Put(ctx context.Context, src any) error
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
	row() proto.Message

	// id returns an identifier for the row.
	id() []byte
}

func (b *bqExporter) batchExportRows(ctx context.Context, ins inserter, batchC chan []rowInput, errorLogger func(ctx context.Context, err bigquery.PutMultiError, rows []*bq.Row)) error {
	eg, ctx := errgroup.WithContext(ctx)

	for rows := range batchC {
		if err := b.batchSem.Acquire(ctx, 1); err != nil {
			// This can happen only if errgroup context is canceled, which usually
			// happens on errors. Grab the error from the errgroup.
			return eg.Wait()
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
	ctx = span.ModifyRequestOptions(ctx, func(opts *span.RequestOptions) {
		opts.Priority = sppb.RequestOptions_PRIORITY_MEDIUM
		opts.Tag = "bqexporter"
	})

	luciProject, err := getLUCIProject(ctx, invID)
	if err != nil {
		return err
	}
	ctx = span.ModifyRequestOptions(ctx, func(opts *span.RequestOptions) {
		opts.Tag = "bqexporter,proj=" + luciProject
	})

	client, err := getBQClient(ctx, luciProject, bqExport)
	if err != nil {
		return errors.Annotate(err, "new bq client").Err()
	}
	defer client.Close()

	table := client.Dataset(bqExport.Dataset).Table(bqExport.Table)
	ins := table.Inserter()

	// Both test results and test artifacts tables are partitioned by partition_time.
	tableMetadata := &bigquery.TableMetadata{
		TimePartitioning: &bigquery.TimePartitioning{
			Field:      "partition_time",
			Expiration: partitionExpirationTime,
		},
	}

	switch bqExport.ResultType.(type) {
	case *pb.BigQueryExport_TestResults_:
		tableMetadata.Schema = testResultRowSchema.Relax()
		if err := schemaApplyer.EnsureTable(ctx, table, tableMetadata); err != nil {
			if !transient.Tag.In(err) {
				err = tq.Fatal.Apply(err)
			}
			return errors.Annotate(err, "ensure test results bq table").Err()
		}
		return errors.Annotate(b.exportTestResultsToBigQuery(ctx, ins, invID, bqExport), "export test results").Err()
	case *pb.BigQueryExport_TextArtifacts_:
		tableMetadata.Schema = textArtifactRowSchema.Relax()
		if err := schemaApplyer.EnsureTable(ctx, table, tableMetadata); err != nil {
			if !transient.Tag.In(err) {
				err = tq.Fatal.Apply(err)
			}
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
	if err := invocations.ReadColumns(ctx, invID, map[string]any{"BigqueryExports": &bqExports}); err != nil {
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
	tq.MustAddTask(ctx, &tq.Task{
		Payload: &taskspb.ExportInvocationToBQ{
			InvocationId: string(invID),
		},
		Title: string(invID),
	})
	return nil
}
