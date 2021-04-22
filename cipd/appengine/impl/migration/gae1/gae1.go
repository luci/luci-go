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

package gae1

import (
	"context"

	"cloud.google.com/go/bigquery"
	"google.golang.org/appengine"

	"go.chromium.org/luci/appengine/bqlog"
	"go.chromium.org/luci/cipd/appengine/impl/model"
)

var eventsLog = bqlog.Log{
	QueueName:           "bqlog-events", // see queue.yaml
	DatasetID:           "cipd",         // see push_bq_schema.sh
	TableID:             "events",
	DumpEntriesToLogger: true,
	DryRun:              appengine.IsDevAppServer(),
}

// Activate installs implementations that use GAEv1 APIs.
func Activate() {
	model.EnqueueEventsImpl = func(ctx context.Context, rows []bigquery.ValueSaver) error {
		return eventsLog.Insert(ctx, rows...)
	}
}

// FlushEventsToBQ sends all buffered events to BigQuery.
//
// It is fine to call FlushEventsToBQ concurrently from multiple request
// handlers, if necessary (it will effectively parallelize the flush).
func FlushEventsToBQ(c context.Context) error {
	_, err := eventsLog.Flush(c)
	return err
}
