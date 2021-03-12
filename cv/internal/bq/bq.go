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

// REferences
// XXX: based on tokenserver/appengine/impl/utils/bq/bq.go.

// Package bq handles sending rows to BigQuery.
package bq

import (
	"context"
	"fmt"
	"net/http"

	"cloud.google.com/go/bigquery"
	protov1 "github.com/golang/protobuf/proto"

	"google.golang.org/api/option"
	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/bq"
	"go.chromium.org/luci/server/auth"

	cvbqpb "go.chromium.org/luci/cv/api/bigquery/v1"
)

const (
	// batchSize is total number of items to pass to PutMulti or DeleteMulti RPCs.
	batchSize = 500
	// clientCtxKey is the key used to install the BQ client in the context.
	clientCtxKey = "go.chromium.org/luci/cv/internal/bq.Client"
)

type Client interface {
	// SendRow appends a row to a BigQuery table.
	SendRow(ctx context.Context, dataset, table string, row proto.Message)
}

// InstallProd puts a production `Client` implementation in the context.
func InstallProd(ctx context.Context, projectID string) (context.Context, error) {
	t, err := auth.GetRPCTransport(ctx, auth.AsSelf, auth.WithScopes(auth.CloudOAuthScopes...))
	if err != nil {
		return nil, err
	}
	bq, err := bigquery.NewClient(ctx, projectID, option.WithHTTPClient(&http.Client{Transport: t}))
	if err != nil {
		return nil, err
	}
	return Install(ctx, bq), nil
}

// Install puts the given `Client` implementation into the context.
func Install(ctx context.Context, c Client) context.Context {
	return context.WithValue(ctx, &clientCtxKey, c)
}

// mustClient returns the `Client` implementation stored in the context.
//
// Panics if not found.
func mustClient(ctx context.Context) Client {
	c := ctx.Value(&clientCtxKey)
	if c == nil {
		panic("BigQuery Client not found in the context")
	}
	return c.(Client)
}

// Send a row to a table synchronously.
//
// This waits until the operation either succeeds or fails then returns.
// Returns an error if an error occurred, else nil.
func SendRow(ctx context.Context, dataset, table string, row proto.Message) error {
	tab := ins.bq.Dataset(dataset).Table(table)

	err := tab.Inserter().Put(ctx, &bq.Row{
		Message:  protov1.MessageV1(row),
		InsertID: fmt.Sprintf("v1:%s", messageID),
	})

	// XXX handle errors
	if err == nil {
		return nil
	}
	if pme, _ := err.(bigquery.PutMultiError); len(pme) != 0 {
		return errors.New("bad row")
	}
	if ctx.Err() != nil {
		return errors.New("deadline exceeded")
	}
	return errors.New("unknown error")
}
