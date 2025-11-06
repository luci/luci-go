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
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"cloud.google.com/go/bigquery"
	"google.golang.org/api/option"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/encoding/prototext"
	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/auth/scopes"
	"go.chromium.org/luci/common/bq"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/tsmon/field"
	"go.chromium.org/luci/common/tsmon/metric"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/tq"
)

var (
	bigQueryInserts = metric.NewCounter(
		"luci/tokenserver/bigquery_inserts",
		"Number of insertAll BQ calls.",
		nil,
		field.String("table"),   // "<projID>/<datasetID>/<tableID>"
		field.String("outcome")) // "ok, "bad_row", "deadline", "error"
)

// RegisterTokenKind registers a TQ class to log a particular token kind into
// a particular BigQuery table.
func RegisterTokenKind(table string, prototype proto.Message) {
	tq.RegisterTaskClass(tq.TaskClass{
		ID:        table,
		Prototype: prototype,
		Kind:      tq.NonTransactional,
		Topic:     "bigquery-log",
		Custom: func(ctx context.Context, m proto.Message) (*tq.CustomPayload, error) {
			blob, err := (protojson.MarshalOptions{Indent: "\t"}).Marshal(m)
			if err != nil {
				return nil, err
			}
			return &tq.CustomPayload{
				Meta: map[string]string{"table": table},
				Body: blob,
			}, nil
		},
	})
}

// LogToken emits a PubSub task to record the token to the BigQuery log.
//
// If `dryRun` is true, will just log the token to the local text log.
func LogToken(ctx context.Context, tok proto.Message, dryRun bool) error {
	if logging.IsLogging(ctx, logging.Debug) {
		blob, err := (prototext.MarshalOptions{Indent: "\t"}).Marshal(tok)
		if err != nil {
			logging.Errorf(ctx, "Failed to marshal the row to proto text: %s", err)
		} else {
			logging.Debugf(ctx, "BigQuery row:\n%s", blob)
		}
	}
	if dryRun {
		return nil
	}
	return tq.AddTask(ctx, &tq.Task{Payload: tok})
}

// Inserter receives PubSub push messages and converts them to BQ inserts.
type Inserter struct {
	bq *bigquery.Client
}

// NewInserter constructs an instance of Inserter.
func NewInserter(ctx context.Context, projectID string) (*Inserter, error) {
	tr, err := auth.GetRPCTransport(ctx, auth.AsSelf, auth.WithScopes(scopes.CloudScopeSet()...))
	if err != nil {
		return nil, err
	}
	bq, err := bigquery.NewClient(ctx, projectID, option.WithHTTPClient(&http.Client{Transport: tr}))
	if err != nil {
		return nil, err
	}
	return &Inserter{bq: bq}, nil
}

// HandlePubSubPush handles incoming PubSub push request.
func (ins *Inserter) HandlePubSubPush(ctx context.Context, body io.Reader) error {
	blob, err := io.ReadAll(body)
	if err != nil {
		return errors.Fmt("failed to read the request body: %w", err)
	}

	// See https://cloud.google.com/pubsub/docs/push#receiving_messages
	var msg struct {
		Message struct {
			Attributes map[string]string `json:"attributes"`
			Data       []byte            `json:"data"`
			MessageID  string            `json:"messageId"`
		} `json:"message"`
	}
	if json.Unmarshal(blob, &msg); err != nil {
		return errors.Fmt("failed to unmarshal PubSub message: %w", err)
	}

	// "table" metadata defines both the destination table and the TQ task class
	// used to push this message, see RegisterTokenKind.
	table := msg.Message.Attributes["table"]

	// Deserialize the row into a corresponding proto type.
	cls := tq.Default.TaskClassRef(table)
	if cls == nil {
		return errors.Fmt("unrecognized task class %q", table)
	}
	row := cls.Definition().Prototype.ProtoReflect().New().Interface()
	if err := protojson.Unmarshal(msg.Message.Data, row); err != nil {
		return errors.Fmt("failed to unmarshal the row for %q: %w", table, err)
	}
	return ins.insert(ctx, table, row, msg.Message.MessageID)
}

func (ins *Inserter) insert(ctx context.Context, table string, row proto.Message, messageID string) error {
	tab := ins.bq.Dataset("tokens").Table(table)

	err := tab.Inserter().Put(ctx, &bq.Row{
		Message:  row,
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

	bigQueryInserts.Add(ctx, 1,
		fmt.Sprintf("%s/%s/%s", tab.ProjectID, tab.DatasetID, tab.TableID),
		outcome)

	return err
}
