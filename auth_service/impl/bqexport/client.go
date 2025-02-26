// Copyright 2025 The LUCI Authors.
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

package bqexport

import (
	"context"
	"fmt"

	"cloud.google.com/go/bigquery"
	"cloud.google.com/go/bigquery/storage/managedwriter"
	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/bq"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/gae/service/info"

	"go.chromium.org/luci/auth_service/api/bqpb"
)

const (
	// The name of the dataset to export authorization data.
	datasetID = "luci_auth_service"

	// The names of tables to export authorization data.
	groupsTableName = "groups"
	realmsTableName = "realms"
)

// Client provides methods to export authorization data to BQ.
type Client struct {
	projectID string
	bqClient  *bigquery.Client
	mwClient  *managedwriter.Client
}

func NewClient(ctx context.Context) (*Client, error) {
	projectID := info.AppID(ctx)
	bqClient, err := bq.NewClient(ctx, projectID)
	if err != nil {
		return nil, errors.Annotate(err,
			"failed to create BQ client for project %q", projectID).Err()
	}

	mwClient, err := bq.NewWriterClient(ctx, projectID)
	if err != nil {
		return nil, errors.Annotate(err,
			"failed to create BQ managed writer client").Err()
	}

	return &Client{
		projectID: projectID,
		bqClient:  bqClient,
		mwClient:  mwClient,
	}, nil
}

// Close releases resources held by the client.
func (client *Client) Close() (reterr error) {
	// Ensure both bqClient and mwClient Close() methods
	// are called, even if one panics or fails.
	defer func() {
		err := client.mwClient.Close()
		if reterr == nil {
			reterr = err
		}
	}()
	return client.bqClient.Close()
}

// ensureGroupsSchema ensures the groups table's schema has been applied.
func (client *Client) ensureGroupsSchema(ctx context.Context) error {
	table := client.bqClient.Dataset(datasetID).Table(groupsTableName)
	if err := schemaApplyer.EnsureTable(ctx, table, groupsTableMetadata); err != nil {
		return errors.Annotate(err, "failed to ensure groups table and schema").Err()
	}
	return nil
}

// InsertGroups inserts the given groups in BQ.
func (client *Client) InsertGroups(ctx context.Context, rows []*bqpb.GroupRow) error {
	if err := client.ensureGroupsSchema(ctx); err != nil {
		return err
	}

	if len(rows) == 0 {
		// Nothing to insert.
		return nil
	}

	tableName := fmt.Sprintf("projects/%s/datasets/%s/tables/%s",
		client.projectID, datasetID, groupsTableName)
	writer := bq.NewWriter(client.mwClient, tableName, groupsTableSchemaDescriptor)

	payload := make([]proto.Message, len(rows))
	for i, row := range rows {
		payload[i] = row
	}

	// Insert the groups with all-or-nothing semantics.
	return writer.AppendRowsWithPendingStream(ctx, payload)
}
