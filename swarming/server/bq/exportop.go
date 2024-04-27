// Copyright 2024 The LUCI Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//  http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package bq

import (
	"context"
	"io"
	"time"

	"cloud.google.com/go/bigquery/storage/apiv1/storagepb"
	"cloud.google.com/go/bigquery/storage/managedwriter"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/tq"
)

const (
	// How many raw BigQuery serialized bytes to buffer before flushing them.
	//
	// BigQuery's hard limit is 10MB.
	bqFlushThresholdDefault = 5 * 1024 * 1024
)

// ExportOp can fetch data from datastore and write it to BigQuery.
//
// It can fail with a transient error and be retried. On a retry it will attempt
// to finish the previous export (if possible).
type ExportOp struct {
	Client      *managedwriter.Client // the BigQuery client to use
	OperationID string                // ID of this particular export operation
	TableID     string                // full table name to write results into
	Fetcher     AbstractFetcher       // fetches data and coverts it to [][]byte

	bqFlushThreshold int
	stream           *managedwriter.ManagedStream
}

// Execute performs the export operation.
func (p *ExportOp) Execute(ctx context.Context, start time.Time, duration time.Duration) error {
	// Check if this operation already ran.
	state := &ExportState{ID: p.OperationID}
	switch err := datastore.Get(ctx, state); {
	case errors.Is(err, datastore.ErrNoSuchEntity):
		// This operation never ran.
	case err != nil:
		return errors.Annotate(err, "fetching ExportState").Tag(transient.Tag).Err()
	default:
		// This operation already ran. Make sure it actually committed writes.
		if err := p.ensureCommitted(ctx, state.WriteStreamName); err != nil {
			return errors.Annotate(err, "ensuring stream is committed").Err()
		}
		return nil
	}

	var flushers []*Flusher

	// Flusher that sends rows to BQ via a lazily opened write stream.
	total := 0
	bqFlushThreshold := bqFlushThresholdDefault
	if p.bqFlushThreshold != 0 {
		bqFlushThreshold = p.bqFlushThreshold
	}
	flushers = append(flushers, &Flusher{
		CountThreshold: 100000,           // ~= unlimited, BQ doesn't care
		ByteThreshold:  bqFlushThreshold, // BQ does care about overall request size
		Marshal:        proto.Marshal,
		Flush: func(rows [][]byte) error {
			total += len(rows)
			return p.appendRows(ctx, rows)
		},
	})

	// TODO: PubSub flusher.

	// Do all fetching and uploads.
	err := p.Fetcher.Fetch(ctx, start, duration, flushers)
	if err != nil {
		return errors.Annotate(err, "exporting rows").Err()
	}

	// It is possible there was no data to export. We are done in that case. No
	// need to store ExportState, since retrying such "empty" task is safe.
	if total == 0 {
		logging.Infof(ctx, "Nothing to commit")
		return nil
	}

	// Prepare the stream for commit.
	logging.Infof(ctx, "Finalizing the stream with %d rows", total)
	if _, err := p.stream.Finalize(ctx); err != nil {
		return wrapBQErr(err, "finalizing the stream")
	}

	// Create an entity representing this commit operation. If we fail to commit
	// the stream, this entity will be used (on a retry) to skip uploading all the
	// rows again.
	err = datastore.Put(ctx, &ExportState{
		ID:              p.OperationID,
		WriteStreamName: p.stream.StreamName(),
		ExpireAt:        clock.Now(ctx).Add(exportStateExpiry).UTC(),
	})
	if err != nil {
		return errors.Annotate(err, "failed to store ExportState").Tag(transient.Tag).Err()
	}

	// Make the exported data actually visible in the BigQuery table.
	return p.commit(ctx, p.stream.StreamName())
}

// Close cleans up resources.
func (p *ExportOp) Close(ctx context.Context) {
	if p.stream != nil {
		if err := p.stream.Close(); err != nil && err != io.EOF {
			logging.Errorf(ctx, "Error closing the write stream: %s", err)
		}
	}
	p.stream = nil
}

// appendRows sends a bunch of rows to BigQuery via the write stream.
func (p *ExportOp) appendRows(ctx context.Context, rows [][]byte) error {
	logging.Infof(ctx, "Appending %d rows...", len(rows))
	stream, err := p.getStream(ctx)
	if err != nil {
		return err
	}
	res, err := stream.AppendRows(ctx, rows)
	if err != nil {
		return wrapBQErr(err, "appending rows")
	}
	_, err = res.FullResponse(ctx)
	if err != nil {
		return wrapBQErr(err, "appending rows")
	}
	return nil
}

// getStream creates the write stream on demand the first time it is needed.
func (p *ExportOp) getStream(ctx context.Context) (*managedwriter.ManagedStream, error) {
	if p.stream != nil {
		return p.stream, nil
	}
	var err error
	p.stream, err = p.Client.NewManagedStream(ctx,
		managedwriter.WithDestinationTable(p.TableID),
		managedwriter.WithType(managedwriter.PendingStream),
		managedwriter.WithSchemaDescriptor(p.Fetcher.Descriptor()),
	)
	if err != nil {
		return nil, wrapBQErr(err, "creating BigQuery write stream")
	}
	logging.Infof(ctx, "Opened write stream %s", p.stream.StreamName())
	return p.stream, nil
}

// ensureCommitted commits the write stream if it is not committed yet.
func (p *ExportOp) ensureCommitted(ctx context.Context, streamName string) error {
	logging.Infof(ctx, "Checking commit status of stream %s", streamName)
	stream, err := p.Client.GetWriteStream(ctx, &storagepb.GetWriteStreamRequest{
		Name: streamName,
	})
	switch {
	case err != nil:
		return wrapBQErr(err, "checking stream status")
	case stream.CommitTime != nil:
		logging.Infof(ctx, "Stream was already committed at %s", stream.CommitTime.AsTime())
		return nil
	default:
		// The stream exists, but wasn't committed yet. Commit it.
		return p.commit(ctx, streamName)
	}
}

// commit commits the write stream into BigQuery, handling errors.
func (p *ExportOp) commit(ctx context.Context, streamName string) error {
	logging.Infof(ctx, "Committing stream %s", streamName)
	resp, err := p.Client.BatchCommitWriteStreams(ctx, &storagepb.BatchCommitWriteStreamsRequest{
		Parent:       p.TableID,
		WriteStreams: []string{streamName},
	})
	switch {
	case err != nil:
		return wrapBQErr(err, "committing the stream")
	case resp.CommitTime == nil:
		// Something is misconfigured. Treat such errors as fatal to avoid infinite
		// retries.
		logging.Errorf(ctx, "Commit failed for %s", streamName)
		for _, serr := range resp.StreamErrors {
			logging.Errorf(ctx, "%s: %s", serr.Code, serr.ErrorMessage)
		}
		return errors.Reason("commit unsuccessful, see logs").Tag(tq.Fatal).Err()
	default:
		logging.Infof(ctx, "Stream was successfully committed at %s", resp.CommitTime.AsTime())
		return nil
	}
}

// wrapBQErr tags a gRPC error based on whether it is fatal or not.
func wrapBQErr(err error, op string) error {
	if err == nil {
		return nil
	}
	switch status.Code(err) {
	case codes.Internal,
		codes.Unknown,
		codes.Unavailable,
		codes.Canceled,
		codes.DeadlineExceeded:
		return errors.Annotate(err, "transient error %s", op).Tag(transient.Tag).Err()
	default:
		return errors.Annotate(err, "fatal error %s", op).Tag(tq.Fatal).Err()
	}
}
