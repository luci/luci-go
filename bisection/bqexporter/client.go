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

package bqexporter

import (
	"context"
	"fmt"

	"cloud.google.com/go/bigquery"
	"cloud.google.com/go/bigquery/storage/managedwriter"
	"google.golang.org/api/iterator"
	"google.golang.org/protobuf/proto"

	bqpb "go.chromium.org/luci/bisection/proto/bq"
	"go.chromium.org/luci/bisection/util/bqutil"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/gae/service/info"
)

// NewClient creates a new client for exporting test analyses
// via the BigQuery Write API.
// projectID is the project ID of the GCP project.
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

// Client provides methods to export test analyses to BigQuery
// via the BigQuery Write API.
type Client struct {
	// projectID is the name of the GCP project that contains LUCI Bisection datasets.
	projectID string
	bqClient  *bigquery.Client
	mwClient  *managedwriter.Client
}

func (client *Client) EnsureSchema(ctx context.Context) error {
	table := client.bqClient.Dataset(bqutil.InternalDatasetID).Table(testFailureAnalysesTableName)
	if err := schemaApplyer.EnsureTable(ctx, table, tableMetadata); err != nil {
		return errors.Annotate(err, "ensuring test_analyses table").Err()
	}
	return nil
}

// Insert inserts the given rows in BigQuery.
func (client *Client) Insert(ctx context.Context, rows []*bqpb.TestAnalysisRow) error {
	if err := client.EnsureSchema(ctx); err != nil {
		return errors.Annotate(err, "ensure schema").Err()
	}
	tableName := fmt.Sprintf("projects/%s/datasets/%s/tables/%s", client.projectID, bqutil.InternalDatasetID, testFailureAnalysesTableName)
	writer := bqutil.NewWriter(client.mwClient, tableName, tableSchemaDescriptor)
	payload := make([]proto.Message, len(rows))
	for i, r := range rows {
		payload[i] = r
	}
	// We use pending stream instead of default stream here because
	// default stream does not offer exactly-once insert.
	return writer.AppendRowsWithPendingStream(ctx, payload)
}

type TestFailureAnalysisRow struct {
	// We only need analysis ID for now.
	AnalysisID int64
}

// ReadTestFailureAnalysisRows returns the Test Failure analysis rows
// in test_failure_analyses table that has created_time within the past 14 days.
func (client *Client) ReadTestFailureAnalysisRows(ctx context.Context) ([]*TestFailureAnalysisRow, error) {
	queryStm := fmt.Sprintf(`
		SELECT DISTINCT
			analysis_id as AnalysisID
		FROM test_failure_analyses
		WHERE created_time >= TIMESTAMP_SUB(CURRENT_TIMESTAMP(), INTERVAL %d DAY)
 	`, daysToLookBack)
	q := client.bqClient.Query(queryStm)
	q.DefaultDatasetID = bqutil.InternalDatasetID
	q.DefaultProjectID = info.AppID(ctx)
	it, err := q.Read(ctx)
	if err != nil {
		return nil, errors.Annotate(err, "querying test failure analyses").Err()
	}
	rows := []*TestFailureAnalysisRow{}
	for {
		row := &TestFailureAnalysisRow{}
		err := it.Next(row)
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, errors.Annotate(err, "obtain next test failure analysis row").Err()
		}
		rows = append(rows, row)
	}
	return rows, nil
}
