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

// clientCtxKey is the key used to install the BQ client in the context.
var clientCtxKey = "go.chromium.org/luci/cv/internal/bq.Client"

type Client interface {
	// SendRow appends a row to a BigQuery table synchronously.
	//
	// Dataset and table specify the destination table; operationID can be used
	// to avoid sending duplicate rows.
	SendRow(ctx context.Context, dataset, table, operationID string, row proto.Message) error
}

// Install puts the given `Client` implementation into the context.
func Install(ctx context.Context, c Client) context.Context {
	return context.WithValue(ctx, &clientCtxKey, c)
}

// InstallProd puts a production `Client` implementation in the context.
//
// This is typically called in the app's main function.
func InstallProd(ctx context.Context, cloudProject string) (context.Context, error) {
	t, err := auth.GetRPCTransport(ctx, auth.AsSelf, auth.WithScopes(auth.CloudOAuthScopes...))
	if err != nil {
		return nil, err
	}
	bq, err := bigquery.NewClient(ctx, cloudProject, option.WithHTTPClient(&http.Client{Transport: t}))
	if err != nil {
		return nil, err
	}
	return Install(ctx, &inserter{bq}), nil
}

// mustClient returns the `Client` implementation stored in the context.
//
// Panics if not found.
func mustClient(ctx context.Context) Client {
	c := ctx.Value(&clientCtxKey)
	if c == nil {
		panic("BQ Client not found in the context")
	}
	return c.(Client)
}

// SendRow sends a row using the client in the context.
func SendRow(ctx context.Context, dataset, table, operationID string, msg proto.Message) error {
	return mustClient(ctx).SendRow(ctx, dataset, table, operationID, msg)
}

// inserter implements a BigQuery client for production.
type inserter struct {
	bq *bigquery.Client
}

// Implements the interface, sending real rows using the bigquery.Client in the
// inserter.
func (ins *inserter) SendRow(ctx context.Context, dataset, table, operationID string, msg proto.Message) error {
	tab := ins.bq.Dataset(dataset).Table(table)
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
