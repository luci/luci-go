// Copyright 2022 The LUCI Authors.
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

package clusteredfailures

import (
	"context"
	"fmt"
	"time"

	"go.chromium.org/luci/analysis/internal/bqutil"
	bqpb "go.chromium.org/luci/analysis/proto/bq"

	"cloud.google.com/go/bigquery"
	"cloud.google.com/go/bigquery/storage/managedwriter"
	"google.golang.org/api/option"
	"google.golang.org/grpc"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/grpc/grpcmon"
	"go.chromium.org/luci/server/auth"
)

// batchSize is the number of rows to write to BigQuery in one go.
const batchSize = 1000

// NewClient creates a new client for exporting clustered failures
// via the BigQuery Write API.
func NewClient(ctx context.Context, projectID string) (s *Client, reterr error) {
	if projectID == "" {
		return nil, errors.New("GCP Project must be specified")
	}

	bqClient, err := bqutil.Client(ctx, projectID)
	if err != nil {
		return nil, errors.Annotate(err, "creating BQ client").Err()
	}
	defer func() {
		if reterr != nil {
			bqClient.Close()
		}
	}()

	// Create shared client for all writes.
	// This will ensure a shared connection pool is used for all writes,
	// as recommended by:
	// https://cloud.google.com/bigquery/docs/write-api-best-practices#limit_the_number_of_concurrent_connections
	creds, err := auth.GetPerRPCCredentials(ctx, auth.AsSelf, auth.WithScopes(auth.CloudOAuthScopes...))
	if err != nil {
		return nil, errors.Annotate(err, "failed to initialize credentials").Err()
	}
	mwClient, err := managedwriter.NewClient(ctx, projectID,
		option.WithGRPCDialOption(grpcmon.WithClientRPCStatsMonitor()),
		option.WithGRPCDialOption(grpc.WithPerRPCCredentials(creds)),
		option.WithGRPCDialOption(grpc.WithKeepaliveParams(keepalive.ClientParameters{
			Time: time.Minute,
		})))
	if err != nil {
		return nil, errors.Annotate(err, "create managed writer client").Err()
	}

	return &Client{
		projectID: projectID,
		bqClient:  bqClient,
		mwClient:  mwClient,
	}, nil
}

// Close releases resources held by the client.
func (s *Client) Close() (reterr error) {
	// Ensure all Close() methods are called, even if one panics or fails.
	defer func() {
		err := s.mwClient.Close()
		if reterr == nil {
			reterr = err
		}
	}()
	return s.bqClient.Close()
}

// Client provides methods to export clustered failures to BigQuery
// via the BigQuery Write API.
type Client struct {
	// projectID is the name of the GCP project that contains Weetbix datasets.
	projectID string
	bqClient  *bigquery.Client
	mwClient  *managedwriter.Client
}

func (s *Client) ensureSchema(ctx context.Context, datasetID string) error {
	// Dataset for the project may have to be manually created.
	table := s.bqClient.Dataset(datasetID).Table(tableName)
	if err := schemaApplyer.EnsureTable(ctx, table, tableMetadata); err != nil {
		return errors.Annotate(err, "ensuring clustered failures table in dataset %q", datasetID).Err()
	}
	return nil
}

// Insert inserts the given rows in BigQuery.
func (s *Client) Insert(ctx context.Context, luciProject string, rows []*bqpb.ClusteredFailureRow) error {
	dataset, err := bqutil.DatasetForProject(luciProject)
	if err != nil {
		return errors.Annotate(err, "getting dataset").Err()
	}

	if err := s.ensureSchema(ctx, dataset); err != nil {
		return errors.Annotate(err, "ensure schema").Err()
	}

	tableName := fmt.Sprintf("projects/%s/datasets/%s/tables/%s", s.projectID, dataset, tableName)

	// Write to the default stream. This does not provide exactly-once
	// semantics (it provides at leas once), but this should be generally
	// fine for our needs. The at least once semantic is similar to the
	// legacy streaming API.
	ms, err := s.mwClient.NewManagedStream(ctx,
		managedwriter.WithSchemaDescriptor(tableSchemaDescriptor),
		managedwriter.WithDestinationTable(tableName))
	defer ms.Close()

	batches := batch(rows)
	results := make([]*managedwriter.AppendResult, 0, len(batches))

	for _, batch := range batches {
		encoded := make([][]byte, 0, len(batch))
		for _, r := range batch {
			b, err := proto.Marshal(r)
			if err != nil {
				return errors.Annotate(err, "marshal proto").Err()
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
		_, err = result.GetResult(ctx)
		if err != nil {
			return errors.Annotate(err, "appending rows").Err()
		}
	}
	return nil
}

// batch divides the rows to be inserted into batches of at most batchSize.
func batch(rows []*bqpb.ClusteredFailureRow) [][]*bqpb.ClusteredFailureRow {
	var result [][]*bqpb.ClusteredFailureRow
	pages := (len(rows) + (batchSize - 1)) / batchSize
	for p := 0; p < pages; p++ {
		start := p * batchSize
		end := start + batchSize
		if end > len(rows) {
			end = len(rows)
		}
		page := rows[start:end]
		result = append(result, page)
	}
	return result
}
