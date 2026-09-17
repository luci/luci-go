// Copyright 2026 The LUCI Authors.
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

package pubsub

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/server/span"
	"go.chromium.org/luci/server/tq"

	"go.chromium.org/luci/resultdb/internal/rootinvocations"
	"go.chromium.org/luci/resultdb/internal/tasks/taskspb"
	"go.chromium.org/luci/resultdb/internal/tracing"
	"go.chromium.org/luci/resultdb/internal/workunits"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

const (
	// defaultCatchUpPageSize is the default number of work units to fetch per page
	// for catch-up.
	// It is limited by standard TQ payload size limits (~100KB).
	// Carrying standard list of WorkUnit IDs downstream requires keeping it approx
	// ~1000 to remain safe in enqueued task payloads.
	defaultCatchUpPageSize = 1000
)

// workUnitsCatchUpPublisher is a helper struct for catching up on publishing work units and test results.
type workUnitsCatchUpPublisher struct {
	// task is the task payload.
	task *taskspb.PublishWorkUnitsCatchUpTask

	// pageSize is the number of work units to query per page.
	pageSize int
}

// tokenHash generates a safe, short hex string from a page token for Cloud Tasks title naming.
func tokenHash(token string) string {
	if token == "" {
		return "start"
	}
	h := sha256.Sum256([]byte(token))
	return hex.EncodeToString(h[:8])
}

// handleWorkUnitsCatchUpPublisher handles the work units catch-up task.
func (p *workUnitsCatchUpPublisher) handleWorkUnitsCatchUpPublisher(ctx context.Context) (err error) {
	ctx, s := tracing.Start(ctx, "go.chromium.org/luci/resultdb/internal/services/pubsub.handleWorkUnitsCatchUpPublisher")
	defer func() { tracing.End(s, err) }()

	task := p.task
	rootInvID := rootinvocations.ID(task.RootInvocationId)

	// 1. Reads Root Invocation to check its state.
	rootInv, err := rootinvocations.Read(span.Single(ctx), rootInvID)
	if err != nil {
		return errors.Fmt("read root invocation %q: %w", rootInvID.Name(), err)
	}

	// 2. Validate state and determine cutoff filter for catch-up.
	if rootInv.StreamingExportState != pb.RootInvocation_METADATA_FINAL && rootInv.FinalizationState != pb.RootInvocation_FINALIZED {
		return tq.Fatal.Apply(errors.Fmt("root invocation %q is not ready for catch-up (export state: %s, finalization state: %s)",
			rootInvID.Name(), rootInv.StreamingExportState, rootInv.FinalizationState))
	}

	q := &workunits.Query{
		RootInvocationID: rootInvID,
		Mask:             workunits.ExcludeExtendedProperties,
		PageSize:         p.pageSize,
		PageToken:        task.PageToken,
		OnlyFinalized:    true,
	}

	if rootInv.StreamingExportState == pb.RootInvocation_METADATA_FINAL {
		// Gap 2: Late transition to METADATA_FINAL. Only catch up work units finalized strictly before
		// MetadataFinalizedTime. Work units finalized at or after MetadataFinalizedTime are handled by direct flow.
		if !rootInv.MetadataFinalizedTime.Valid {
			return tq.Fatal.Apply(errors.Fmt("root invocation %q is in METADATA_FINAL state but MetadataFinalizedTime is not set", rootInvID.Name()))
		}
		q.MaxFinalizeTimeStrictlyBefore = rootInv.MetadataFinalizedTime.Time
	} else {
		// Gap 1: Root invocation finalized without ever transitioning to METADATA_FINAL.
		//
		// Note: A root invocation transitions to FINALIZING from UpdateWorkUnit,
		// FinalizeWorkUnit (when finalizing the root work unit), or deadlineenforcer.
		// While triggering the catch-up task directly from those three locations could
		// slightly reduce export latency, keeping the trigger point centralized in
		// workunitfinalizer simplifies maintenance. Hence, we handle Gap 1 catch-up
		// here when root invocation is FINALIZED without METADATA_FINAL.
		//
		// For Gap 1, Direct Flow never ran for any work units because StreamingExportState
		// remained WAIT_FOR_METADATA throughout. Therefore, all finalized work units under
		// the root invocation need to be caught up, without needing a FinalizeTime cutoff.
	}

	// 3. Query work units.
	var wuIDs []string
	roCtx, cancel := span.ReadOnlyTransaction(ctx)
	nextPageToken, err := q.Query(roCtx, func(wu *workunits.WorkUnitRow) error {
		wuIDs = append(wuIDs, wu.ID.WorkUnitID)
		return nil
	})
	cancel()
	if err != nil {
		return errors.Fmt("query work units for catch-up: %w", err)
	}

	if len(wuIDs) == 0 && nextPageToken == "" {
		return nil
	}

	// 4. Enqueue tasks in a transaction.
	_, err = span.ReadWriteTransaction(ctx, func(ctx context.Context) error {
		pageTokenStr := tokenHash(task.PageToken)

		if len(wuIDs) > 0 {
			// Enqueue PublishWorkUnitsTask
			tq.MustAddTask(ctx, &tq.Task{
				Payload: &taskspb.PublishWorkUnitsTask{
					RootInvocationId: task.RootInvocationId,
					WorkUnitIds:      wuIDs,
				},
				Title: fmt.Sprintf("wu-pubsub-catchup-%s-%s", task.RootInvocationId, pageTokenStr),
			})

			// Enqueue PublishTestResultsTask
			tq.MustAddTask(ctx, &tq.Task{
				Payload: &taskspb.PublishTestResultsTask{
					RootInvocationId: task.RootInvocationId,
					WorkUnitIds:      wuIDs,
				},
				Title: fmt.Sprintf("tr-pubsub-catchup-%s-%s", task.RootInvocationId, pageTokenStr),
			})
		}

		// 5. Schedule continuation if necessary.
		if nextPageToken != "" {
			tq.MustAddTask(ctx, &tq.Task{
				Payload: &taskspb.PublishWorkUnitsCatchUpTask{
					RootInvocationId: task.RootInvocationId,
					PageToken:        nextPageToken,
				},
				Title: fmt.Sprintf("wu-catch-up-cont-%s-%s", task.RootInvocationId, tokenHash(nextPageToken)),
			})
		}
		return nil
	})

	if err != nil {
		return errors.Fmt("enqueue catch-up child tasks: %w", err)
	}

	return nil
}
