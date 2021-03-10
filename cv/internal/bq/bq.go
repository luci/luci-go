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

// XXX: based on tokenserver/appengine/impl/utils/bq/bq.go.

// batchSize is total number of items to pass to PutMulti or DeleteMulti RPCs.
const batchSize = 500

// Inserter sends BQ rows.
type Inserter struct {
	bq *bigquery.Client
}

// NewInserter constructs an instance of Inserter.
// and perhaps install into context (which is typical approach we use in CV, but not used in Token Server)
func NewInserter(ctx context.Context, projectID string) (*Inserter, error) {
	tr, err := auth.GetRPCTransport(ctx, auth.AsSelf, auth.WithScopes(auth.CloudOAuthScopes...))
	if err != nil {
		return nil, err
	}
	bq, err := bigquery.NewClient(ctx, projectID, option.WithHTTPClient(&http.Client{Transport: tr}))
	if err != nil {
		return nil, err
	}
	return &Inserter{bq: bq}, nil
}

// For use in tests.
func InstallFakeInserter(ctx context.Context) (*Inserter, error) {
}

// Send a row to a table synchronously.
//
// This waits until the operation either succeeds or fails then returns.
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
