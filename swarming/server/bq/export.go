// Copyright 2023 The LUCI Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package bq

import (
	"context"
	"fmt"
	"os"
	"time"

	"cloud.google.com/go/bigquery/storage/managedwriter"
	pubsub "cloud.google.com/go/pubsub/apiv1"
	"golang.org/x/sync/errgroup"
	"google.golang.org/api/option"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/grpc/grpcmon"
	"go.chromium.org/luci/server"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/cron"
	"go.chromium.org/luci/server/tq"

	"go.chromium.org/luci/swarming/server/bq/taskspb"
)

const (
	// exportDuration is a time interval processed by a single export task.
	//
	// Must be an integer number of seconds (important for deriving TQ task
	// deduplication key).
	exportDuration = 15 * time.Second

	// maxTasksToSchedule is the maximum number of tasks to schedule at once.
	//
	// Each cron job "tick" will schedule processing of at most 15*20 = 5 min
	// worth of events. It means if the cron is ticking slower than once per 5 min
	// we'll have a backlog and will need to change some parameters.
	maxTasksToSchedule = 20

	// minEventAge is an age of an event when it becomes "eligible" for export.
	//
	// All events we want to export are associated with timestamps of when they
	// happen. Events younger than minEventAge are not touched by the BQ exporter.
	//
	// This is a precaution against exporting inconsistent state. Our "events" are
	// not really events, but mutable datastore entities. Very young entities are
	// still in flux (e.g. TaskRequest appears slightly before TaskResultSummary).
	//
	// In an ideal world where everything happens via transactions this won't be
	// needed.
	minEventAge = 2 * time.Minute
)

// exportGlobals is constructed on server start and shared with TQ handlers.
type exportGlobals struct {
	bqClient          *managedwriter.Client
	psClient          *pubsub.PublisherClient
	exportToPubSub    stringset.Set
	pubSubTopicPrefix string
}

// Register registers TQ tasks and cron handlers that implement BigQuery export.
func Register(
	srv *server.Server,
	disp *tq.Dispatcher,
	cron *cron.Dispatcher,
	dataset, onlyOneTable string,
	exportToPubSub stringset.Set,
	pubSubTopicPrefix string,
) error {
	ts, err := auth.GetTokenSource(srv.Context, auth.AsSelf, auth.WithScopes(auth.CloudOAuthScopes...))
	if err != nil {
		return errors.Fmt("failed to create TokenSource: %w", err)
	}

	// Client for exports to BigQuery.
	bqClient, err := managedwriter.NewClient(srv.Context, srv.Options.CloudProject,
		option.WithTokenSource(ts),
		option.WithGRPCDialOption(grpc.WithStatsHandler(&grpcmon.ClientRPCStatsMonitor{})),
	)
	if err != nil {
		return errors.Fmt("failed to create BQ client: %w", err)
	}
	srv.RegisterCleanup(func(ctx context.Context) {
		if err := bqClient.Close(); err != nil {
			logging.Errorf(ctx, "Error closing BQ client: %s", err)
		}
	})

	// Client for exports to PubSub.
	psClient, err := pubsub.NewPublisherClient(srv.Context,
		option.WithTokenSource(ts),
		option.WithGRPCDialOption(grpc.WithStatsHandler(&grpcmon.ClientRPCStatsMonitor{})),
	)
	if err != nil {
		return errors.Fmt("failed to create PubSub client: %w", err)
	}
	srv.RegisterCleanup(func(ctx context.Context) {
		if err := psClient.Close(); err != nil {
			logging.Errorf(ctx, "Error closing PubSub client: %s", err)
		}
	})

	// When running locally use a custom entity key prefix to avoid colliding
	// with entities used by staging Swarming. Also don't schedule a lot of tasks
	// at once (they will all run at the same time, making local testing harder).
	keyPrefix := ""
	maxTasks := maxTasksToSchedule
	if !srv.Options.Prod {
		keyPrefix = fmt.Sprintf("dev:%s:", os.Getenv("USER"))
		maxTasks = 1
	}

	cron.RegisterHandler("bq-export", func(ctx context.Context) error {
		if dataset == "none" {
			return nil
		}
		// All tables to export. See also `exportTask` where corresponding Fetchers
		// are constructed.
		tables := []string{
			TaskRequests,
			BotEvents,
			TaskRunResults,
			TaskResultSummaries,
		}
		eg, _ := errgroup.WithContext(ctx)
		for _, table := range tables {
			if onlyOneTable != "" && table != onlyOneTable {
				continue
			}
			eg.Go(func() error {
				return scheduleExportTasks(
					ctx, disp, keyPrefix, maxTasks,
					srv.Options.CloudProject, dataset, table,
				)
			})
		}
		return eg.Wait()
	})

	registerTQTasks(disp, &exportGlobals{
		bqClient:          bqClient,
		psClient:          psClient,
		exportToPubSub:    exportToPubSub,
		pubSubTopicPrefix: pubSubTopicPrefix,
	})
	return nil
}

// registerTQTasks registers TQ tasks used for BQ and PubSub export.
func registerTQTasks(disp *tq.Dispatcher, globals *exportGlobals) {
	disp.RegisterTaskClass(tq.TaskClass{
		ID:        "bq-export-interval",
		Kind:      tq.NonTransactional,
		Prototype: &taskspb.ExportInterval{},
		Queue:     "bq-export-interval",
		Handler: func(ctx context.Context, payload proto.Message) error {
			return exportTask(ctx, globals, payload.(*taskspb.ExportInterval))
		},
	})
}

// scheduleExportTasks creates a series of TQ tasks that export not yet exported
// events.
//
// ExportSchedule entity holds the timestamp of not yet exported events (called
// NextExport). The goal of this function is to advance this timestamp, emitting
// TQ tasks that do actual export along the way.
//
// Each TQ task exports events from a time range of exportDuration seconds. Thus
// NextExport is advanced in increments of exportDuration. It is advanced either
// until reaching maxTasksToSchedule number of emitted tasks, or until it is
// about to cross `now()-minEventAge` cut off point: we don't want to schedule
// too many tasks at once and we don't want to export very new events (they may
// still be touched by the main Swarming logic!).
//
// Either way at the end of this process ExportSchedule entity is updated to
// point NextExport to new not-yet-exported timestamp, to let this process
// resume next time scheduleExportTasks is called.
//
// All emitted TQ tasks have DuplicationKey derived from Unix timestamp of
// the range they are exporting. That way on reties of scheduleExportTasks we
// do not re-export the same subranges again. This relies on range boundaries to
// "line up" across scheduleExportTasks invocations (to make DuplicationKey be
// the same). To ensure that, we round initial NextExport to an integer number
// of seconds and then **always** update it in increments of exportDuration
// (which is also an integer number of seconds). That way all TQ tasks stay
// snapped "to the grid" and DuplicationKey is essentially an integer coordinate
// on this grid.
func scheduleExportTasks(ctx context.Context, disp *tq.Dispatcher, keyPrefix string, maxTasksToSchedule int, cloudProject, dataset, tableName string) error {
	// Get the timestamp to start exporting from (all events before it have been
	// exported already).
	sch := &ExportSchedule{ID: keyPrefix + tableName}
	switch err := datastore.Get(ctx, sch); {
	case errors.Is(err, datastore.ErrNoSuchEntity):
		// Need to truncate at least to seconds precision (see the function doc),
		// but minutes precision is also OK and looks prettier. This code runs once,
		// it doesn't matter.
		sch.NextExport = clock.Now(ctx).Add(-minEventAge).Truncate(time.Minute).UTC()
		logging.Infof(ctx, "Creating initial ExportSchedule: %s", sch.NextExport)
		if err := datastore.Put(ctx, sch); err != nil {
			return transient.Tag.Apply(errors.Fmt("failed to create ExportSchedule: %w", err))
		}
		return nil
	case err != nil:
		return transient.Tag.Apply(errors.Fmt("failed to fetch ExportSchedule: %w", err))
	}

	cutoff := clock.Now(ctx).UTC().Add(-minEventAge)
	tasks := 0
	fail := false

	for sch.NextExport.Add(exportDuration).Before(cutoff) && tasks < maxTasksToSchedule {
		// Note that the initial value of sch.NextExport is a whole number of
		// seconds and we always increment it by exportDuration (also a whole number
		// of seconds). It means sch.NextExport.Unix() doesn't lose any precision
		// and it will be the same even if scheduleExportTasks crashes (before
		// saving sch.NextExport) and then gets retried.
		operationID := fmt.Sprintf("%s%s:%d:%d",
			keyPrefix,
			tableName,
			sch.NextExport.Unix(),
			exportDuration/time.Second,
		)

		logging.Debugf(ctx, "Triggering %s", operationID)
		err := disp.AddTask(ctx, &tq.Task{
			Title:            operationID,
			DeduplicationKey: operationID,
			Payload: &taskspb.ExportInterval{
				OperationId:  operationID,
				Start:        timestamppb.New(sch.NextExport),
				Duration:     durationpb.New(exportDuration),
				CloudProject: cloudProject,
				Dataset:      dataset,
				TableName:    tableName,
			},
		})
		if err != nil {
			logging.Errorf(ctx, "Failed to trigger export task %s: %s", operationID, err)
			fail = true
			break
		}

		sch.NextExport = sch.NextExport.Add(exportDuration)
		tasks += 1
	}

	// Update the state only if something has changed (even on failure).
	if tasks != 0 {
		if err := datastore.Put(ctx, sch); err != nil {
			return transient.Tag.Apply(errors.Fmt("failed to update ExportSchedule: %w", err))
		}
	}

	// If failed to submit a task, ask cron to retry ASAP.
	if fail {
		return transient.Tag.Apply(errors.New("failed to schedule a TQ task"))
	}
	return nil
}

// exportTask is the handler for "bq-export-interval" TQ tasks.
func exportTask(ctx context.Context, globals *exportGlobals, t *taskspb.ExportInterval) error {
	var fetcher AbstractFetcher
	switch t.TableName {
	case TaskRequests:
		fetcher = TaskRequestFetcher()
	case BotEvents:
		fetcher = BotEventsFetcher()
	case TaskRunResults:
		fetcher = TaskRunResultsFetcher()
	case TaskResultSummaries:
		fetcher = TaskResultSummariesFetcher()
	default:
		return tq.Fatal.Apply(errors.
			Fmt("unknown table name %q", t.TableName))
	}

	// Export rows to PubSub if this specific table is requested via server flags.
	psTopic := ""
	if globals.exportToPubSub.Has(t.TableName) {
		pfx := ""
		if globals.pubSubTopicPrefix != "" {
			pfx = globals.pubSubTopicPrefix + "_"
		}
		psTopic = fmt.Sprintf("projects/%s/topics/%s%s", t.CloudProject, pfx, t.TableName)
	}
	op := &ExportOp{
		BQClient:    globals.bqClient,
		PSClient:    globals.psClient,
		OperationID: t.OperationId,
		TableID:     fmt.Sprintf("projects/%s/datasets/%s/tables/%s", t.CloudProject, t.Dataset, t.TableName),
		Topic:       psTopic,
		Fetcher:     fetcher,
	}
	defer op.Close(ctx)
	return op.Execute(ctx, t.Start.AsTime(), t.Duration.AsDuration())
}
