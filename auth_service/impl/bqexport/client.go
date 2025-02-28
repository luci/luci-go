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
	"bytes"
	"context"
	"fmt"
	"text/template"
	"time"

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

	// The name of the key column used in the query for a view of the latest
	// groups or realms.
	latestKeyColumn = "exported_at"

	// The names of views for the latest export in which both groups and realms
	// were successfully exported.
	latestGroupsViewName = "latest_groups"
	latestRealmsViewName = "latest_realms"

	// Template to construct the query for a view of the latest exported data.
	latestView = `
	WITH these_keys AS (
		SELECT DISTINCT {{.keyColumn}} FROM {{.datasetID}}.{{.thisTableName}}
	),

	other_keys AS (
		SELECT DISTINCT {{.keyColumn}} FROM {{.datasetID}}.{{.otherTableName}}
	),

	latest AS (
		SELECT MAX(these_keys.{{.keyColumn}}) AS key FROM these_keys
		INNER JOIN other_keys
		ON these_keys.{{.keyColumn}} = other_keys.{{.keyColumn}}
	)

	SELECT * FROM {{.datasetID}}.{{.thisTableName}}
	WHERE {{.keyColumn}} = COALESCE(
		(SELECT key FROM latest),
		(SELECT MAX({{.keyColumn}}) FROM {{.datasetID}}.{{.thisTableName}})
	)
	`
)

var (
	latestViewTemplate = template.Must(template.New("latest view").Parse(latestView))
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

// ensureRealmsSchema ensures the realms table's schema has been applied.
func (client *Client) ensureRealmsSchema(ctx context.Context) error {
	table := client.bqClient.Dataset(datasetID).Table(realmsTableName)
	if err := schemaApplyer.EnsureTable(ctx, table, realmsTableMetadata); err != nil {
		return errors.Annotate(err, "failed to ensure realms table and schema").Err()
	}
	return nil
}

// InsertRealms inserts the given realms in BQ.
func (client *Client) InsertRealms(ctx context.Context, rows []*bqpb.RealmRow) error {
	if err := client.ensureRealmsSchema(ctx); err != nil {
		return err
	}

	if len(rows) == 0 {
		// Nothing to insert.
		return nil
	}

	tableName := fmt.Sprintf("projects/%s/datasets/%s/tables/%s",
		client.projectID, datasetID, realmsTableName)
	writer := bq.NewWriter(client.mwClient, tableName, realmsTableSchemaDescriptor)

	payload := make([]proto.Message, len(rows))
	for i, row := range rows {
		payload[i] = row
	}

	// Insert the realms with all-or-nothing semantics.
	return writer.AppendRowsWithPendingStream(ctx, payload)
}

func constructLatestViewQuery(ctx context.Context,
	thisTable, otherTable string) (string, error) {
	// Construct the view query.
	buf := bytes.Buffer{}
	err := latestViewTemplate.Execute(&buf, map[string]string{
		"datasetID":      datasetID,
		"keyColumn":      latestKeyColumn,
		"thisTableName":  thisTable,
		"otherTableName": otherTable,
	})
	if err != nil {
		return "", err
	}

	return buf.String(), nil
}

func (client *Client) ensureLatestView(ctx context.Context,
	viewName, thisTable, otherTable, version string) error {
	viewQuery, err := constructLatestViewQuery(ctx, thisTable, otherTable)
	if err != nil {
		return errors.Annotate(err,
			"failed to construct view query for %q", viewName).Err()
	}

	// Ensure the view to propagate schema updates.
	metadata := &bigquery.TableMetadata{
		ViewQuery: viewQuery,
		Labels:    map[string]string{bq.MetadataVersionKey: version},
	}
	view := client.bqClient.Dataset(datasetID).Table(viewName)
	err = bq.EnsureTable(ctx, view, metadata, bq.UpdateMetadata(),
		bq.RefreshViewInterval(time.Hour))
	if err != nil {
		return errors.Annotate(err, "failed to ensure view %q for version %s",
			viewName, version).Err()
	}

	return nil
}

// EnsureLatestViews ensures the views for the latest groups and latest realms
// have their metadata applied, including schema and query definition updates.
//
// If a view's underlying table has its schema updated, be sure to update the
// metadata version to propagate it to the view as well.
func (client *Client) EnsureLatestViews(ctx context.Context) error {
	// Apply the metadata for the view of the latest groups.
	groupsViewVersion := "1"
	err := client.ensureLatestView(
		ctx, latestGroupsViewName, groupsTableName, realmsTableName,
		groupsViewVersion)
	if err != nil {
		return err
	}

	// Apply the metadata for the view of the latest realms.
	realmsViewVersion := "1"
	err = client.ensureLatestView(
		ctx, latestRealmsViewName, realmsTableName, groupsTableName,
		realmsViewVersion)
	if err != nil {
		return err
	}

	return nil
}
