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

package analysis

import (
	"context"

	"cloud.google.com/go/bigquery"
	"go.chromium.org/luci/common/errors"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/iterator"

	"go.chromium.org/luci/analysis/internal/bqutil"
)

// ProjectNotExistsErr is returned if the dataset for the given project
// does not exist.
var ProjectNotExistsErr = errors.New("project does not exist in Weetbix or analysis is not yet available")

// InvalidArgumentTag is used to indicate that one of the query options
// is invalid.
var InvalidArgumentTag = errors.BoolTag{Key: errors.NewTagKey("invalid argument")}

// NewClient creates a new client for reading clusters. Close() MUST
// be called after you have finished using this client.
func NewClient(ctx context.Context, gcpProject string) (*Client, error) {
	client, err := bqutil.Client(ctx, gcpProject)
	if err != nil {
		return nil, err
	}
	return &Client{client: client}, nil
}

// Client may be used to read Weetbix clusters.
type Client struct {
	client *bigquery.Client
}

// Close releases any resources held by the client.
func (c *Client) Close() error {
	return c.client.Close()
}

// ProjectsWithDataset returns the set of LUCI projects which have
// a BigQuery dataset created.
func (c *Client) ProjectsWithDataset(ctx context.Context) (map[string]struct{}, error) {
	result := make(map[string]struct{})
	di := c.client.Datasets(ctx)
	for {
		d, err := di.Next()
		if err == iterator.Done {
			break
		} else if err != nil {
			return nil, err
		}
		project, err := bqutil.ProjectForDataset(d.DatasetID)
		if err != nil {
			return nil, err
		}
		result[project] = struct{}{}
	}
	return result, nil
}

func handleJobReadError(err error) error {
	switch e := err.(type) {
	case *googleapi.Error:
		if e.Code == 404 {
			return ProjectNotExistsErr
		}
	}
	return errors.Annotate(err, "obtain result iterator").Err()
}
