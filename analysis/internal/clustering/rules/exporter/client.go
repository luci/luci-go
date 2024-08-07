// Copyright 2023 The LUCI Authors.
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

// Package exporter provides methods to interact with the failure_assocation_rules BigQuery table.
package exporter

import (
	"context"
	"fmt"

	"cloud.google.com/go/bigquery"
	"cloud.google.com/go/bigquery/storage/managedwriter"
	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/bq"
	"go.chromium.org/luci/common/errors"

	"go.chromium.org/luci/analysis/internal/bqutil"
	bqpb "go.chromium.org/luci/analysis/proto/bq"
)

// Client provides methods to export and read failure_association_rules_history BigQuery tale
type Client struct {
	// projectID is the name of the GCP project that contains LUCI Analysis datasets.
	projectID string
	// bqClient is the BigQuery client.
	bqClient *bigquery.Client
	// mwClient is the BigQuery managed writer.
	mwClient *managedwriter.Client
}

// NewClient creates a new client for exporting failure association rules
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
			// This method failed for some reason, clean up the
			// BigQuery client. Swallow any error returned by the Close()
			// call.
			bqClient.Close()
		}
	}()

	mwClient, err := bq.NewWriterClient(ctx, projectID)
	if err != nil {
		return nil, errors.Annotate(err, "creating managed writer client").Err()
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

// Insert inserts the given rows in BigQuery.
func (c *Client) Insert(ctx context.Context, rows []*bqpb.FailureAssociationRulesHistoryRow) error {
	if err := c.ensureSchema(ctx); err != nil {
		return errors.Annotate(err, "ensure schema").Err()
	}
	tableName := fmt.Sprintf("projects/%s/datasets/%s/tables/%s", c.projectID, bqutil.InternalDatasetID, tableName)
	writer := bq.NewWriter(c.mwClient, tableName, tableSchemaDescriptor)
	payload := make([]proto.Message, len(rows))
	for i, r := range rows {
		payload[i] = r
	}
	return writer.AppendRowsWithPendingStream(ctx, payload)
}

// NewestLastUpdated get the largest value in the lastUpdated field from the BigQuery table.
// The last_updated field is synced from the spanner table which is a spanner commit timestamp.
// This the newest last_updated field indicate newest update of the failureAssociationRules spanner table
// which has been synced to BigQuery.
func (c *Client) NewestLastUpdated(ctx context.Context) (bigquery.NullTimestamp, error) {
	if err := c.ensureSchema(ctx); err != nil {
		return bigquery.NullTimestamp{}, errors.Annotate(err, "ensure schema").Err()
	}
	q := c.bqClient.Query(`
		SELECT MAX(last_update_time) as LastUpdateTime
		FROM failure_association_rules_history
	`)
	q.DefaultDatasetID = bqutil.InternalDatasetID
	it, err := q.Read(ctx)
	if err != nil {
		return bigquery.NullTimestamp{}, errors.Annotate(err, "querying max last update").Err()
	}
	type result struct {
		LastUpdateTime bigquery.NullTimestamp
	}
	var lastUpdatedResult result
	err = it.Next(&lastUpdatedResult)
	if err != nil {
		return bigquery.NullTimestamp{}, errors.Annotate(err, "obtain next row").Err()
	}
	return lastUpdatedResult.LastUpdateTime, nil
}

// schemaApplier ensures BQ schema matches the row proto definitions.
var schemaApplier = bq.NewSchemaApplyer(bq.RegisterSchemaApplyerCache(1))

func (c *Client) ensureSchema(ctx context.Context) error {
	// Dataset for the project may have to be manually created.
	table := c.bqClient.Dataset(bqutil.InternalDatasetID).Table(tableName)
	if err := schemaApplier.EnsureTable(ctx, table, tableMetadata); err != nil {
		return errors.Annotate(err, "ensuring %s table", tableName).Err()
	}
	return nil
}
