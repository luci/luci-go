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

package failureattributes

import (
	"context"

	"cloud.google.com/go/bigquery"

	"go.chromium.org/luci/common/errors"

	"go.chromium.org/luci/analysis/internal/bqutil"
)

// NewClient creates a new client for updating BigQuery failure_attributes
// table.
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

	return &Client{
		projectID: projectID,
		bqClient:  bqClient,
	}, nil
}

// Close releases resources held by the client.
func (s *Client) Close() (reterr error) {
	return s.bqClient.Close()
}

// Client provides methods to update BigQuery failure_attributes table.
type Client struct {
	// projectID is the name of the GCP project that contains LUCI Analysis datasets.
	projectID string
	bqClient  *bigquery.Client
}

func (s *Client) ensureSchema(ctx context.Context) error {
	table := s.bqClient.Dataset(bqutil.InternalDatasetID).Table(tableName)
	if err := schemaApplyer.EnsureTable(ctx, table, tableMetadata); err != nil {
		return errors.Annotate(err, "ensuring failure attributes table").Err()
	}
	return nil
}
