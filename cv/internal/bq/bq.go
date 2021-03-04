// Copyright 2017 The LUCI Authors.
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

// NOTE: This is WIP;
// client creation based on tokenserver/appengine/impl/utils/bq/bq.go.

// batchSize is total number of items to pass to PutMulti or DeleteMulti RPCs.
const batchSize = 500

// Inserter receives send requests and attempts to send BQ rows.
type Inserter struct {
	bq *bigquery.Client
}

// Fetches an attempt and its relevant info from datastore,
// Tries to send the BQ row.
// On failure returns a transient error.
func Send(ctx context.Context, int64 runID) error {
	// TODO: Implement.
	//  1. Read Attempt data from datastore.
	//  2. Do any necessary conversion.
	//  3. Create BQ client, and try to send row.
	// Failure anywhere above results in transient error.
}

func ReadAttempt(ctx context.Context, int64 runID) (*cvbqpb.Attempt, error) {
	// TODO: Read Attempt (and anything else required) from datastore based on run ID.
}

func SendRow(ctx context.Context, attempt *cvbqpb.Attempt) error {
	tr, err := auth.GetRPCTransport(ctx, auth.AsSelf, auth.WithScopes(auth.CloudOAuthScopes...))
	if err != nil {
		return nil, err
	}
	//var bq *bigQuery.Client
	bq, err := bigquery.NewClient(ctx, projectID, option.WithHTTPClient(&http.Client{Transport: tr}))
	if err != nil {
		return nil, err
	}
	return &Inserter{bq: bq}, nil

}

func (ins *Inserter) insert(ctx context.Context, table string, row proto.Message, messageID string) error {
	table = "raw_new"
	tab := ins.bq.Dataset("attemtps").Table(table)

	err := tab.Inserter().Put(ctx, &bq.Row{
		Message:  protov1.MessageV1(row),
		InsertID: fmt.Sprintf("v1:%s", messageID),
	})

	var outcome string
	if err == nil {
		outcome = "ok"
	} else if pme, _ := err.(bigquery.PutMultiError); len(pme) != 0 {
		outcome = "bad_row"
	} else if ctx.Err() != nil {
		outcome = "deadline"
	} else {
		outcome = "error"
	}

	// NOTE: Consider making a tsmon metric for BQ inserts?
	return err
}
