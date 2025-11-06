// Copyright 2021 The LUCI Authors.
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

// Package bq handles sending rows to BigQuery.
package bq

import (
	"context"
	"net/http"

	"cloud.google.com/go/bigquery"
	"google.golang.org/api/option"
	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/auth/scopes"
	lucibq "go.chromium.org/luci/common/bq"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/server/auth"
)

// Row encapsulates destination and actual row to send.
//
// Exists to avoid confusion over multiple string arguments to SendRow.
type Row struct {
	// CloudProject allows sending rows to other projects.
	//
	// Optional. Defaults to the one in the scope of which this process is running
	// (e.g. "luci-change-verifier-dev").
	CloudProject string
	Dataset      string
	Table        string
	// OperationID is used for de-duplication, but over just 1 minute window :(
	OperationID string
	Payload     proto.Message
}

type Client interface {
	// SendRow appends a row to a BigQuery table synchronously.
	SendRow(ctx context.Context, row Row) error
}

// NewProdClient creates new production client.
//
// The specified cloud project should be the one, in the scope of which this
// code is running, e.g. "luci-change-verifier-dev".
func NewProdClient(ctx context.Context, cloudProject string) (*prodClient, error) {
	t, err := auth.GetRPCTransport(ctx, auth.AsSelf, auth.WithScopes(scopes.CloudScopeSet()...))
	if err != nil {
		return nil, err
	}
	b, err := bigquery.NewClient(ctx, cloudProject, option.WithHTTPClient(&http.Client{Transport: t}))
	if err != nil {
		return nil, errors.Fmt("failed to create BQ client: %w", err)
	}
	return &prodClient{b}, nil
}

// prodClient implements a BigQuery Client for production.
type prodClient struct {
	b *bigquery.Client
}

// SendRow sends a row to a real BigQuery table.
func (c *prodClient) SendRow(ctx context.Context, row Row) error {
	var table *bigquery.Table
	if row.CloudProject == "" {
		table = c.b.Dataset(row.Dataset).Table(row.Table)
	} else {
		table = c.b.DatasetInProject(row.CloudProject, row.Dataset).Table(row.Table)
	}
	r := &lucibq.Row{
		Message:  row.Payload,
		InsertID: row.OperationID,
	}
	if err := table.Inserter().Put(ctx, r); err != nil {
		if pme, _ := err.(bigquery.PutMultiError); len(pme) != 0 {
			return errors.Fmt("bad row: %w", err)
		}
		return transient.Tag.Apply(errors.Fmt("unknown error sending row: %w", err))
	}
	return nil
}
