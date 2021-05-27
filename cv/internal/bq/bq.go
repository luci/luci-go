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
	protov1 "github.com/golang/protobuf/proto"

	"google.golang.org/api/option"
	"google.golang.org/protobuf/proto"

	lucibq "go.chromium.org/luci/common/bq"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/server/auth"
)

type Client interface {
	// SendRow appends a row to a BigQuery table synchronously.
	//
	// Dataset and table specify the destination table; operationID can be used
	// to avoid sending duplicate rows.
	SendRow(ctx context.Context, dataset, table, operationID string, row proto.Message) error
}

// prodClient implements a BigQuery Client for production.
type prodClient struct {
	bq *bigquery.Client
}

func NewProdClient(ctx context.Context, cloudProject string) (Client, error) {
	t, err := auth.GetRPCTransport(ctx, auth.AsSelf, auth.WithScopes(auth.CloudOAuthScopes...))
	if err != nil {
		return nil, err
	}
	bq, err := bigquery.NewClient(ctx, cloudProject, option.WithHTTPClient(&http.Client{Transport: t}))
	if err != nil {
		return nil, err
	}
	return &prodClient{bq}, nil
}

// SendRow sends a row to a real BigQuery table.
func (c *prodClient) SendRow(ctx context.Context, dataset, table, operationID string, msg proto.Message) error {
	tab := c.bq.Dataset(dataset).Table(table)
	row := &lucibq.Row{
		Message:  protov1.MessageV1(msg),
		InsertID: operationID,
	}
	if err := tab.Inserter().Put(ctx, row); err != nil {
		if pme, _ := err.(bigquery.PutMultiError); len(pme) != 0 {
			return errors.Annotate(err, "bad row").Err()
		}
		return errors.Annotate(err, "unknown error sending row").Tag(transient.Tag).Err()
	}
	return nil
}
