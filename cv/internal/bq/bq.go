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
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"

	"cloud.google.com/go/bigquery"
	protov1 "github.com/golang/protobuf/proto"

	"google.golang.org/api/option"
	"google.golang.org/protobuf/encoding/protojson"
	//"google.golang.org/protobuf/encoding/prototext"
	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/bq"
	"go.chromium.org/luci/common/errors"
	//"go.chromium.org/luci/common/logging"
	//"go.chromium.org/luci/common/tsmon/field"
	//"go.chromium.org/luci/common/tsmon/metric"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/tq"

	cvbqpb "go.chromium.org/luci/cv/api/bigquery/v1"
)

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
	// ReadAttempt
	// ConvertToMessage
	// SendRow

	// Failure anywhere above results in transient error.
}

func ReadAttempt(ctx context.Context, int64 runID) (*cvbqpb.Attempt, error) {
	// Read things from datastore
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

func (ins *Inserter) ConvertToMessage(ctx context.Context) error {
	//
	//
	// Deserialize the row into a corresponding proto type.
	cls := tq.Default.TaskClassRef(table)
	if cls == nil {
		return errors.Annotate(err, "unrecognized task class %q", table).Err()
	}
	row := cls.Definition().Prototype.ProtoReflect().New().Interface()
	if err := protojson.Unmarshal(msg.Message.Data, row); err != nil {
		return errors.Annotate(err, "failed to unmarshal the row for %q", table).Err()
	}
	return ins.insert(ctx, table, row, msg.Message.MessageID)
}

func (ins *Inserter) insert(ctx context.Context /*bq *bigquery.Client,*/, table string, row proto.Message, messageID string) error {
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

	//bigQueryInserts.Add(ctx, 1,
	//fmt.Sprintf("%s/%s/%s", tab.ProjectID, tab.DatasetID, tab.TableID),
	//outcome)

	return err
}
