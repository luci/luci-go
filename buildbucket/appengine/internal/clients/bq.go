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

package clients

import (
	"context"
	"net/http"

	"cloud.google.com/go/bigquery"
	"google.golang.org/api/option"

	"go.chromium.org/luci/auth/scopes"
	lucibq "go.chromium.org/luci/common/bq"
	"go.chromium.org/luci/server/auth"
)

var bqClientCtxKey = "holds the global bigquery client"

type BqClient interface {
	// Insert a row into a BigQuery table
	Insert(ctx context.Context, dataset string, table string, row *lucibq.Row) error
}
type bqClientImpl struct {
	client *bigquery.Client
}

// Ensure bqClientImpl implements BqClient.
var _ BqClient = &bqClientImpl{}

func (b *bqClientImpl) Insert(ctx context.Context, dataset string, table string, row *lucibq.Row) error {
	t := b.client.Dataset(dataset).Table(table)
	return t.Inserter().Put(ctx, row)
}

// NewBqClient creates a new BqClient.
func NewBqClient(ctx context.Context, cloudProject string) (BqClient, error) {
	t, err := auth.GetRPCTransport(ctx, auth.AsSelf, auth.WithScopes(scopes.CloudScopeSet()...))
	if err != nil {
		return nil, err
	}
	b, err := bigquery.NewClient(ctx, cloudProject, option.WithHTTPClient(&http.Client{Transport: t}))
	if err != nil {
		return nil, err
	}
	return &bqClientImpl{client: b}, nil
}

// WithBqClient returns a new context with the given bq client.
func WithBqClient(ctx context.Context, client BqClient) context.Context {
	return context.WithValue(ctx, &bqClientCtxKey, client)
}

// GetBqClient returns the bigquery Client installed in the current context.
// Panics if there isn't one.
func GetBqClient(ctx context.Context) BqClient {
	return ctx.Value(&bqClientCtxKey).(BqClient)
}
