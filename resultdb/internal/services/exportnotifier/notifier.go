// Copyright 2024 The LUCI Authors.
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

// Package exportnotifier is responsible for dispatching
// "invocation ready for export" notifications, used to trigger low-latency
// exports from ResultDB. Notifications are dispatched when all of the
// following criteria is met:
//   - The invocation is included by an export root, AND
//   - The invocation is locally immutable - signified by the
//     invocation being in FINALIZING (or FINALIZED) state, AND
//   - The sources the invocation are final. This could be because
//     the sources were specified concretely and invocation is final
//     (see above) or the invocation is inheriting sources, and those
//     sources are available and final.
package exportnotifier

import (
	"context"

	"cloud.google.com/go/spanner"
	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/server"
	"go.chromium.org/luci/server/span"
	"go.chromium.org/luci/server/tq"

	"go.chromium.org/luci/resultdb/internal/exportroots"
	"go.chromium.org/luci/resultdb/internal/invocations"
	"go.chromium.org/luci/resultdb/internal/tasks/taskspb"
	pb "go.chromium.org/luci/resultdb/proto/v1"

	// Add support for Spanner transactions in TQ.
	_ "go.chromium.org/luci/server/tq/txn/spanner"
)

const (
	// The number of export root rows to propagate to in one transaction.
	// This should balance Spanner mutation limits, efficiency of using
	// larger transactions and the risk of contention.
	BatchSizeInExportRootRows = 1000
)

// RunExportNotificationsTasks describes how to route
// run export notification tasks.
var RunExportNotificationsTasks = tq.RegisterTaskClass(tq.TaskClass{
	ID:            "propagate-export-roots",
	Prototype:     &taskspb.RunExportNotifications{},
	Kind:          tq.Transactional,
	Queue:         "exportnotifier",
	RoutingPrefix: "/internal/tasks/exportnotifier",
})

type Options struct {
	// Hostname of the luci.resultdb.v1.ResultDB service which can be
	// queried to fetch the details of invocations being exported.
	// E.g. "results.api.cr.dev".
	ResultDBHostname string
}

// InitServer initializes a exportnotifier server.
func InitServer(srv *server.Server, opts Options) error {
	RunExportNotificationsTasks.AttachHandler(func(ctx context.Context, msg proto.Message) error {
		task := msg.(*taskspb.RunExportNotifications)
		// Propogate export roots and send `invocation ready for export`
		// notifications as needed.
		return propagate(ctx, task, opts)
	})
	return nil
}

// propagate propagates export roots and their associated sources to
// included invocations. It triggers `invocation ready for export`
// notifications as appropriate.
//
// This task must be called whenever there is any update
// to an invocation that may affect the export root records or
// trigger a notification, e.g:
// - new invocation is included.
// - sources are finalized.
// - the invocation transitions to FINALIZING state.
// - a new export root is defined.
//
// The task must ensure that the changes that were made and for
// which the task was created are propogated to the included
// invocations. It *may* also opportunistically apply other
// changes (for which another task would have been scheduled).
//
// If each task does at least what it needs to and does not roll
// back any other task's work (with which it may be racing), then
// export roots will be eventually consistent with the state of
// invocations in ResultDB, after all pending tasks for that
// invocation graph have completed.
func propagate(ctx context.Context, task *taskspb.RunExportNotifications, opts Options) error {
	// The task body can be split up into multiple transactions, where
	// it does not cause us to violate any of the following rules:
	//   - We read whatever was present at a time after the task
	//     was scheduled.
	//   - We make all updates (i.e. propagate roots and sources and
	//     notify) required by that read.
	//   - We only ever make positive progress, that is, we don't roll
	//     back or overwrite the sources or (Is)Notified fields set
	//     by a concurrently running propagate job.
	//
	// The last point requires use of a Read-Write transaction
	// to update entities, as blind writes risk overwriting.

	invID := invocations.ID(task.InvocationId)
	rootRestriction := toRootRestriction(task.RootInvocationIds)

	inv, err := invocations.ReadExportInfo(span.Single(ctx), invID)
	if err != nil {
		if spanner.ErrCode(err) == codes.NotFound {
			// Invocation was deleted.
			return nil
		}

		return errors.Fmt("read sources: %w", err)
	}

	var roots []exportroots.ExportRoot
	_, err = span.ReadWriteTransaction(ctx, func(ctx context.Context) error {
		var err error
		// Read export roots of the current invocation, we'll need
		// these later when we consider propagating them to included
		// invocations.
		roots, err = exportroots.ReadForInvocation(ctx, invID, rootRestriction)
		if err != nil {
			return err
		}

		// Consider whether we need to send any notifications
		// for the current invocation.

		if !inv.IsInvocationFinal {
			// Invocation is not yet ready for export.
			return nil
		}
		for _, root := range roots {
			if inv.IsInheritingSources && !root.IsInheritedSourcesSet {
				// Wait for inherited sources to be set.
				continue
			}
			if root.IsNotified {
				// We previously notified for this root.
				continue
			}

			var rootRealm string
			var rootCreateTime *timestamppb.Timestamp
			if err := invocations.ReadColumns(ctx, root.RootInvocation, map[string]any{"Realm": &rootRealm, "CreateTime": &rootCreateTime}); err != nil {
				return errors.Fmt("read root invocation: %w", err)
			}

			var sources *pb.Sources
			if inv.IsInheritingSources {
				sources = root.InheritedSources
			} else {
				sources = inv.Sources
			}

			// Create pub/sub message.
			notification := &pb.InvocationReadyForExportNotification{
				ResultdbHost:        opts.ResultDBHostname,
				RootInvocation:      root.RootInvocation.Name(),
				RootInvocationRealm: rootRealm,
				RootCreateTime:      rootCreateTime,
				Invocation:          root.Invocation.Name(),
				InvocationRealm:     inv.Realm,
				Sources:             sources,
			}
			// Transactioncally dispatch it.
			notifyInvocationReadyForExport(ctx, notification)

			// Set notified.
			root.IsNotified = true
			span.BufferWrite(ctx, exportroots.SetNotified(root))
		}
		return nil
	})
	if err != nil {
		return errors.Fmt("read roots and notify: %w", err)
	}

	// Read included invocations.
	included, err := invocations.ReadIncluded(span.Single(ctx), invID)
	if err != nil {
		return errors.Fmt("read included: %w", err)
	}

	if len(task.IncludedInvocationIds) > 0 {
		// Restrict to propagating to nominated invocations, if specified.
		included = included.Intersect(toIDSet(task.IncludedInvocationIds))
	}

	if len(roots) == 0 || len(included) == 0 {
		// Early exit: no export roots to propagate or nowhere to propogate to.
		return nil
	}

	// Compute the batch size in number of included invocations to process at
	// a time.
	batchSizeInInvocations := BatchSizeInExportRootRows / len(roots)
	if batchSizeInInvocations <= 1 {
		batchSizeInInvocations = 1
	}

	// Propogate export roots and sources to included invocations.
	// Batch updates to remain within Spanner transaction mutation limits.
	batches := included.Batch(batchSizeInInvocations)
	for _, includedInvocationIDs := range batches {
		_, err := span.ReadWriteTransaction(ctx, func(ctx context.Context) error {
			rootsToRead := invocations.NewIDSet()
			for _, root := range roots {
				rootsToRead.Add(root.RootInvocation)
			}

			childRoots, err := exportroots.ReadForInvocations(ctx, includedInvocationIDs, rootsToRead)
			if err != nil {
				return errors.Fmt("read roots for included invocations: %w", err)
			}

			// For each included invocation.
			for _, includedInvocationID := range includedInvocationIDs.SortByRowID() {
				rootsUpdated := invocations.NewIDSet()

				// For each export root in the current invocation to propagate.
				for _, root := range roots {
					// Identify the sources that included invocations are eligible
					// to inherit from this invocation for this export root.
					sourcesKnown := false
					var sources *pb.Sources

					// The sources spec on this invocation must final before we
					// can tell included invocations which sources they are inheriting.
					if inv.IsSourceSpecFinalEffective {
						if inv.IsInheritingSources && root.IsInheritedSourcesSet {
							// This invocation is inheriting sources, and the sources
							// assigned to this invocation for this export root are known.
							sources = root.InheritedSources
							sourcesKnown = true
						} else if !inv.IsInheritingSources {
							// This invocation has specified concrete sources.
							sources = inv.Sources
							sourcesKnown = true
						}
					}

					childRoot, ok := childRoots[includedInvocationID][root.RootInvocation]
					if ok {
						// The included invocation already has a record for the export root.
						if !childRoot.IsInheritedSourcesSet && sourcesKnown {
							// Sources were not set, but are available now.
							// Propogate sources to the export root record.
							childRoot.InheritedSources = sources
							childRoot.IsInheritedSourcesSet = true
							span.BufferWrite(ctx, exportroots.SetInheritedSources(childRoot))
							rootsUpdated.Add(root.RootInvocation)
						}
					} else {
						// The included invocation does not have a record of the root.
						// Create a record, setting sources as appropriate.
						newRoot := exportroots.ExportRoot{
							Invocation:            includedInvocationID,
							RootInvocation:        root.RootInvocation,
							IsInheritedSourcesSet: sourcesKnown,
							InheritedSources:      sources,
							IsNotified:            false,
						}
						span.BufferWrite(ctx, exportroots.Create(newRoot))
						rootsUpdated.Add(root.RootInvocation)
					}
				}

				if len(rootsUpdated) > 0 {
					// Updates were made to this included invocation's export roots.
					// Schedule a task to send pub/sub notifications and/or continue
					// propogation of roots and sources on the included invocation
					// as appropriate.
					var rootIDs []string
					for _, rootID := range rootsUpdated.SortByRowID() {
						rootIDs = append(rootIDs, string(rootID))
					}
					task := &taskspb.RunExportNotifications{
						InvocationId:      string(includedInvocationID),
						RootInvocationIds: rootIDs,
					}
					EnqueueTask(ctx, task)
				}
			}
			return nil
		})
		if err != nil {
			return errors.Fmt("propogate to included invocations: %w", err)
		}
	}
	return nil
}

func toIDSet(invocationIDs []string) invocations.IDSet {
	result := invocations.NewIDSet()
	for _, id := range invocationIDs {
		result.Add(invocations.ID(id))
	}
	return result
}

func toRootRestriction(rootInvocationIDs []string) exportroots.RootRestriction {
	if len(rootInvocationIDs) == 0 {
		return exportroots.RootRestriction{
			UseRestriction: false,
		}
	}
	return exportroots.RootRestriction{
		UseRestriction: true,
		InvocationIDs:  toIDSet(rootInvocationIDs),
	}
}

// EnqueueTask transactionally enqueues a RunExportNotifications task.
func EnqueueTask(ctx context.Context, task *taskspb.RunExportNotifications) {
	tq.MustAddTask(ctx, &tq.Task{
		Payload: task,
		Title:   string(task.InvocationId),
	})
}
