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

// NOTE: This is WIP;
// client creation based on tokenserver/appengine/impl/utils/bq/bq.go.

// batchSize is total number of items to pass to PutMulti or DeleteMulti RPCs.
const batchSize = 500

// Inserter receives send requests and attempts to send BQ rows.
type Inserter struct {
	bq *bigquery.Client
}

// TODO: Add:
// * API to instantiate prod Inserter, and perhaps install into context (which is typical approach we use in CV, but not used in Token Server)
// * API to install fake inserter for use in tests.

// Send a row to a table synchronously.
//
// This waits until the operation either succeeds or fails then returns
func SendRow(ctx context.Context, table string, row proto.Message) error {
	panic("not implemented")
}
