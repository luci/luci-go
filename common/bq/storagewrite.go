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

package bq

import (
	"context"
	"fmt"
	"time"

	"cloud.google.com/go/bigquery/storage/apiv1/storagepb"
	"cloud.google.com/go/bigquery/storage/managedwriter"
	"google.golang.org/api/option"
	"google.golang.org/grpc"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/descriptorpb"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/errors/errtag"
	"go.chromium.org/luci/grpc/grpcmon"
	"go.chromium.org/luci/server/auth"
)

// RowMaxBytes is the maximum number of row bytes to send in one
// BigQuery Storage Write API - AppendRows request. As at writing, the
// request size limit for this RPC is 10 MB:
// https://cloud.google.com/bigquery/quotas#write-api-limits.
// The maximum size of rows must be less than this as there are
// some overheads in each request.
// This also includes the overhead (16 bytes) for each row when calculating the row size.
// Please see RowSize for details.
const RowMaxBytes = 9 * 1000 * 1000 // 9 MB

// RowSizeOverhead is the overhead for each row.
// It got added to proto.Size for the overhead not being captured.
const RowSizeOverhead = 16 // 16 bytes

// InvalidRowTag will be attached to error when appending rows if
// the rows are invalid (e.g. not UTF-8 compliance, or too large).
var InvalidRowTag = errtag.Make("InvalidRow", true)

// NewWriterClient returns a new BigQuery managedwriter client for use with the
// given GCP project, that authenticates as the project itself.
func NewWriterClient(ctx context.Context, gcpProject string) (*managedwriter.Client, error) {
	// Create shared client for all writes.
	// This will ensure a shared connection pool is used for all writes,
	// as recommended by:
	// https://cloud.google.com/bigquery/docs/write-api-best-practices#limit_the_number_of_concurrent_connections
	creds, err := auth.GetPerRPCCredentials(ctx, auth.AsSelf, auth.WithScopes(auth.CloudOAuthScopes...))
	if err != nil {
		return nil, errors.Annotate(err, "failed to initialize credentials").Err()
	}
	return managedwriter.NewClient(ctx, gcpProject,
		option.WithGRPCDialOption(grpc.WithStatsHandler(&grpcmon.ClientRPCStatsMonitor{})),
		option.WithGRPCDialOption(grpc.WithPerRPCCredentials(creds)),
		option.WithGRPCDialOption(grpc.WithKeepaliveParams(keepalive.ClientParameters{
			Time: time.Minute,
		})))
}

// Writer is used to export rows to BigQuery table.
type Writer struct {
	client                *managedwriter.Client
	tableName             string
	tableSchemaDescriptor *descriptorpb.DescriptorProto
}

// NewWriter creates a writer for exporting rows to the provided BigQuery table
// via the provided managedWriter client.
func NewWriter(
	client *managedwriter.Client,
	tableName string,
	tableSchemaDescriptor *descriptorpb.DescriptorProto,
) *Writer {
	return &Writer{
		client:                client,
		tableName:             tableName,
		tableSchemaDescriptor: tableSchemaDescriptor,
	}
}

// AppendRowsWithDefaultStream write to the default stream. This does not provide exactly-once
// semantics (it provides at least once). The at least once semantic is similar to the
// legacy streaming API.
func (s *Writer) AppendRowsWithDefaultStream(ctx context.Context, rows []proto.Message, extraOpts ...managedwriter.WriterOption) error {
	opts := []managedwriter.WriterOption{
		managedwriter.WithType(managedwriter.DefaultStream),
		managedwriter.WithSchemaDescriptor(s.tableSchemaDescriptor),
		managedwriter.WithDestinationTable(s.tableName),
		managedwriter.EnableWriteRetries(true)}
	for _, opt := range extraOpts {
		opts = append(opts, opt)
	}
	ms, err := s.client.NewManagedStream(ctx, opts...)
	if err != nil {
		return err
	}
	defer ms.Close()

	return s.batchAppendRows(ctx, ms, rows)
}

// AppendRowsWithPendingStream append rows to BigQuery table via the pending stream.
// This provides all-or-nothing semantics for insertion.
func (s *Writer) AppendRowsWithPendingStream(ctx context.Context, rows []proto.Message) error {
	ms, err := s.client.NewManagedStream(ctx,
		managedwriter.WithType(managedwriter.PendingStream),
		managedwriter.WithSchemaDescriptor(s.tableSchemaDescriptor),
		managedwriter.WithDestinationTable(s.tableName))
	if err != nil {
		return err
	}
	defer ms.Close()

	err = s.batchAppendRows(ctx, ms, rows)
	if err != nil {
		return err
	}
	_, err = ms.Finalize(ctx)
	if err != nil {
		return err
	}
	req := &storagepb.BatchCommitWriteStreamsRequest{
		Parent:       s.tableName,
		WriteStreams: []string{ms.StreamName()},
	}
	// Commit data atomically.
	resp, err := s.client.BatchCommitWriteStreams(ctx, req)
	if err != nil {
		return err
	}
	if len(resp.StreamErrors) > 0 {
		return errors.New(fmt.Sprintf("batchCommitWriteStreams error %s", resp.StreamErrors))
	}
	return nil
}

// batchAppendRows chunk rows into batches and append each batch to the provided managedStream.
// If the input rows are invalid, the error returned will be tagged with InvalidRowTag.
func (s *Writer) batchAppendRows(ctx context.Context, ms *managedwriter.ManagedStream, rows []proto.Message) error {
	batches, err := toBatches(rows)
	if err != nil {
		return errors.Annotate(err, "batching rows").Tag(InvalidRowTag).Err()
	}
	results := make([]*managedwriter.AppendResult, 0, len(batches))
	for _, batch := range batches {
		encoded := make([][]byte, 0, len(batch))
		for _, r := range batch {
			b, err := proto.Marshal(r)
			if err != nil {
				return errors.Annotate(err, "marshal proto").Tag(InvalidRowTag).Err()
			}
			encoded = append(encoded, b)
		}
		result, err := ms.AppendRows(ctx, encoded)
		if err != nil {
			return errors.Annotate(err, "start appending rows").Err()
		}
		// Defer waiting on AppendRows until after all batches sent out.
		// https://cloud.google.com/bigquery/docs/write-api-best-practices#do_not_block_on_appendrows_calls
		results = append(results, result)
	}
	for _, result := range results {
		// TODO: In future, we might need to apply some sort of retry
		// logic around batches as we did for legacy streaming writes
		// for quota issues.
		// That said, the client library here should deal with standard
		// BigQuery retries and backoffs.
		_, err := result.GetResult(ctx)
		if err != nil {
			return errors.Annotate(err, "appending rows (context: %s)", ctx.Err()).Err()
		}
	}
	return nil
}

// toBatches divides the rows to be inserted into batches, with each
// batch having an on-the-wire size not exceeding batchMaxBytes.
func toBatches(rows []proto.Message) ([][]proto.Message, error) {
	var result [][]proto.Message

	batchStartIndex := 0
	batchSizeInBytes := 0
	for i, row := range rows {
		rowSize := RowSize(row)
		if (batchSizeInBytes + rowSize) > RowMaxBytes {
			if rowSize > RowMaxBytes {
				return nil, errors.Reason("a single row exceeds the maximum BigQuery AppendRows request size of %v bytes", RowMaxBytes).Err()
			}
			// Output batch from batchStartIndex (inclusive) to i (exclusive).
			result = append(result, rows[batchStartIndex:i])

			// The current row becomes part of the next batch.
			batchStartIndex = i
			batchSizeInBytes = 0
		}
		batchSizeInBytes += rowSize
	}
	lastBatch := rows[batchStartIndex:]
	if len(lastBatch) > 0 {
		result = append(result, lastBatch)
	}
	return result, nil
}

// RowSize return size of row when we do batching.
func RowSize(row proto.Message) int {
	// Assume 16 bytes of overhead per row not captured here.
	return proto.Size(row) + RowSizeOverhead
}
