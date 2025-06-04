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

package artifactexporter

import (
	"context"
	"fmt"

	"cloud.google.com/go/bigquery"
	"cloud.google.com/go/bigquery/storage/managedwriter"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/bq"
	"go.chromium.org/luci/common/errors"

	"go.chromium.org/luci/resultdb/bqutil"
	bqpb "go.chromium.org/luci/resultdb/proto/bq"
)

// NewClient creates a new client for exporting artifacts
// via the BigQuery Write API.
func NewClient(ctx context.Context, projectID string) (s *Client, reterr error) {
	if projectID == "" {
		return nil, errors.New("GCP Project must be specified")
	}

	bqClient, err := bq.NewClient(ctx, projectID)
	if err != nil {
		return nil, errors.Fmt("creating BQ client: %w", err)
	}
	defer func() {
		if reterr != nil {
			// This method failed for some reason, clean up the
			// BigQuery client. Swallow any error returned by the Close()
			// call.
			bqClient.Close()
		}
	}()

	mwClient, err := bq.NewWriterClient(ctx, projectID)
	if err != nil {
		return nil, errors.Fmt("creating managed writer client: %w", err)
	}
	return &Client{
		projectID: projectID,
		bqClient:  bqClient,
		mwClient:  mwClient,
	}, nil
}

// Close releases resources held by the client.
func (c *Client) Close() (reterr error) {
	// Ensure both bqClient and mwClient Close() methods
	// are called, even if one panics or fails.
	defer func() {
		err := c.mwClient.Close()
		if reterr == nil {
			reterr = err
		}
	}()
	return c.bqClient.Close()
}

// Client provides methods to export artifacts to BigQuery
// via the BigQuery Write API.
type Client struct {
	// projectID is the name of the GCP project that contains ResultDB
	// BigQuery datasets.
	projectID string
	bqClient  *bigquery.Client
	mwClient  *managedwriter.Client
}

// schemaApplier ensures BQ schema matches the row proto definitions.
var schemaApplyer = bq.NewSchemaApplyer(bq.RegisterSchemaApplyerCache(1))

func (c *Client) EnsureSchema(ctx context.Context) error {
	table := c.bqClient.Dataset(bqutil.InternalDatasetID).Table(tableName)
	if err := schemaApplyer.EnsureTable(ctx, table, tableMetadata, bq.UpdateMetadata()); err != nil {
		return errors.Fmt("ensuring text artifacts table: %w", err)
	}
	return nil
}

// InsertArtifactRows inserts the given rows in BigQuery.
func (c *Client) InsertArtifactRows(ctx context.Context, rows []*bqpb.TextArtifactRow) error {
	if err := c.EnsureSchema(ctx); err != nil {
		return errors.Fmt("ensure schema: %w", err)
	}
	now := timestamppb.Now()
	tableName := fmt.Sprintf("projects/%s/datasets/%s/tables/%s", c.projectID, bqutil.InternalDatasetID, tableName)
	writer := bq.NewWriter(c.mwClient, tableName, tableSchemaDescriptor)
	payload := make([]proto.Message, len(rows))
	for i, r := range rows {
		r.InsertTime = now
		payload[i] = r
	}
	return writer.AppendRowsWithDefaultStream(ctx, payload)
}
