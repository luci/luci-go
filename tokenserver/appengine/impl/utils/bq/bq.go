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

// Package bq contains helpers for uploading rows to BigQuery.
package bq

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"cloud.google.com/go/bigquery"
	protov1 "github.com/golang/protobuf/proto"
	"google.golang.org/api/option"
	"google.golang.org/protobuf/encoding/prototext"
	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/bq"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/tsmon/field"
	"go.chromium.org/luci/common/tsmon/metric"
	"go.chromium.org/luci/server/auth"
)

var (
	bigQueryInserts = metric.NewCounter(
		"luci/tokenserver/bigquery_inserts",
		"Number of insertAll BQ calls.",
		nil,
		field.String("table"),   // "<projID>/<datasetID>/<tableID>"
		field.String("outcome")) // "ok, "bad_row", "deadline", "error"
)

// NewClient constructs an instance of BigQuery API client with proper auth.
func NewClient(ctx context.Context, projectID string) (*bigquery.Client, error) {
	tr, err := auth.GetRPCTransport(ctx, auth.AsSelf, auth.WithScopes(auth.CloudOAuthScopes...))
	if err != nil {
		return nil, err
	}
	return bigquery.NewClient(ctx, projectID, option.WithHTTPClient(&http.Client{Transport: tr}))
}

// Inserter can insert BQ rows into some particular BigQuery table.
type Inserter struct {
	Table  *bigquery.Table // the table to upload rows to
	DryRun bool            // if true, only log rows locally, do not upload them

	// Insert ID generation.
	initID sync.Once
	nextID int64
	prefix string
}

var marshalOpts = prototext.MarshalOptions{
	Indent:       " ",
	AllowPartial: true,
}

// Insert inserts a single row into the table.
func (ins *Inserter) Insert(ctx context.Context, row proto.Message) error {
	if logging.IsLogging(ctx, logging.Debug) {
		blob, err := marshalOpts.Marshal(row)
		if err != nil {
			logging.Errorf(ctx, "Failed to marshal the row to proto text: %s", err)
		} else {
			logging.Debugf(ctx, "BigQuery row for %s/%s:\n%s", ins.Table.DatasetID, ins.Table.TableID, blob)
		}
	}

	if ins.DryRun {
		return nil
	}

	ctx, cancel := clock.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	err := ins.Table.Inserter().Put(ctx, &bq.Row{
		Message:  protov1.MessageV1(row),
		InsertID: ins.nextInsertID(), // for dedup when retrying on transient errors
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

	bigQueryInserts.Add(ctx, 1,
		fmt.Sprintf("%s/%s/%s", ins.Table.ProjectID, ins.Table.DatasetID, ins.Table.TableID),
		outcome)

	return err
}

// nextInsertID returns a unique insert ID.
func (ins *Inserter) nextInsertID() string {
	ins.initID.Do(func() {
		buf := make([]byte, 16)
		if _, err := rand.Read(buf); err != nil {
			panic(fmt.Errorf("failed to read from crypto/rand: %s", err))
		}
		ins.prefix = base64.RawStdEncoding.EncodeToString(buf)
	})
	return fmt.Sprintf("%s:%d", ins.prefix, atomic.AddInt64(&ins.nextID, 1))
}
