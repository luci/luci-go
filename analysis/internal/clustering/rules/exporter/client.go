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

	"go.chromium.org/luci/analysis/internal/bqutil"
	bqpb "go.chromium.org/luci/analysis/proto/bq"
	"go.chromium.org/luci/common/errors"
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
			bqClient.Close()
		}
	}()

	mwClient, err := bqutil.NewWriterClient(ctx, projectID)
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

// Insert inserts the given rows in BigQuery.
func (s *Client) Insert(ctx context.Context, rows []*bqpb.FailureAssociationRulesHistoryRow) error {
	if err := s.ensureSchema(ctx); err != nil {
		return errors.Annotate(err, "ensure schema").Err()
	}
	tableName := fmt.Sprintf("projects/%s/datasets/%s/tables/%s", s.projectID, datasetID, tableName)
	writer := bqutil.NewWriter(s.mwClient, tableName, tableSchemaDescriptor)
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
func (s *Client) NewestLastUpdated(ctx context.Context) (bigquery.NullTimestamp, error) {
	if err := s.ensureSchema(ctx); err != nil {
		return bigquery.NullTimestamp{}, errors.Annotate(err, "ensure schema").Err()
	}
	q := s.bqClient.Query(`
		SELECT MAX(last_updated) as bqLastUpdated
		FROM failure_association_rules_history
	`)
	q.DefaultDatasetID = datasetID
	job, err := q.Run(ctx)
	if err != nil {
		return bigquery.NullTimestamp{}, errors.Annotate(err, "querying max last update").Err()
	}
	it, err := job.Read(ctx)

	if err != nil {
		return bigquery.NullTimestamp{}, err
	}
	var lastUpdate bigquery.NullTimestamp
	err = it.Next(&lastUpdate)
	if err != nil {
		return bigquery.NullTimestamp{}, errors.Annotate(err, "obtain next row").Err()
	}
	return lastUpdate, nil
}

func (s *Client) ensureSchema(ctx context.Context) error {
	// Dataset for the project may have to be manually created.
	table := s.bqClient.Dataset(datasetID).Table(tableName)
	if err := schemaApplyer.EnsureTable(ctx, table, tableMetadata); err != nil {
		return errors.Annotate(err, "ensuring %s table in dataset %q", tableName, datasetID).Err()
	}
	return nil
}
