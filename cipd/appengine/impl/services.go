// Copyright 2018 The LUCI Authors.
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

// Package impl instantiates the full implementation of the CIPD backend
// services.
//
// It is imported by GAE's frontend and backend modules that expose appropriate
// bits and pieces over pRPC and HTTP.
package impl

import (
	"context"

	"cloud.google.com/go/bigquery"
	"google.golang.org/appengine"

	"go.chromium.org/luci/appengine/bqlog"
	"go.chromium.org/luci/common/bq"
	"go.chromium.org/luci/server/router"

	"go.chromium.org/luci/cipd/appengine/impl/admin"
	"go.chromium.org/luci/cipd/appengine/impl/cas"
	"go.chromium.org/luci/cipd/appengine/impl/migration"
	"go.chromium.org/luci/cipd/appengine/impl/model"
	"go.chromium.org/luci/cipd/appengine/impl/repo"
	"go.chromium.org/luci/cipd/appengine/impl/settings"

	adminapi "go.chromium.org/luci/cipd/api/admin/v1"
	cipdapi "go.chromium.org/luci/cipd/api/cipd/v1"
)

var (
	// InternalCAS is non-ACLed implementation of cas.StorageService to be used
	// only from within the backend code itself.
	InternalCAS cas.StorageServer

	// PublicCAS is ACL-protected implementation of cas.StorageServer that can be
	// exposed as a public API.
	PublicCAS cipdapi.StorageServer

	// PublicRepo is ACL-protected implementation of cipd.RepositoryServer that
	// can be exposed as a public API.
	PublicRepo repo.Server

	// AdminAPI is ACL-protected implementation of cipd.AdminServer that can be
	// exposed as an external API to be used by administrators.
	AdminAPI adminapi.AdminServer

	// EventLogger can flush events to BigQuery.
	EventLogger *model.BigQueryEventLogger
)

// eventsLog is used only on GAE1
var eventsLog *bqlog.Log

func InitForGAE1(r *router.Router, mw router.MiddlewareChain) {
	tq := migration.NewAppengineTQ()
	InternalCAS = cas.Internal(tq, settings.Get)
	PublicCAS = cas.Public(InternalCAS)
	PublicRepo = repo.Public(InternalCAS, tq)
	AdminAPI = admin.AdminAPI(nil)

	eventsLog = &bqlog.Log{
		QueueName:           "bqlog-events", // see queue.yaml
		DatasetID:           "cipd",         // see push_bq_schema.sh
		TableID:             "events",
		DumpEntriesToLogger: true,
		DryRun:              appengine.IsDevAppServer(),
	}
	model.EnqueueEventsImpl = func(ctx context.Context, ev []*cipdapi.Event) error {
		rows := make([]bigquery.ValueSaver, len(ev))
		for idx, e := range ev {
			rows[idx] = &bq.Row{Message: e}
		}
		return eventsLog.Insert(ctx, rows...)
	}

	// InstallRoutes must be called after all handlers are registered.
	if r != nil {
		tq.TQ.InstallRoutes(r, mw)
	}
}

// FlushEventsToBQGAE1 sends all buffered events to BigQuery.
func FlushEventsToBQGAE1(ctx context.Context) error {
	_, err := eventsLog.Flush(ctx)
	return err
}
