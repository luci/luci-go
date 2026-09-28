// Copyright 2025 The LUCI Authors.
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
	"fmt"
	"strings"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/server/auth/realms"
	"go.chromium.org/luci/server/span"
	"go.chromium.org/luci/server/tq"

	"go.chromium.org/luci/resultdb/internal/artifacts"
	"go.chromium.org/luci/resultdb/internal/checkpoints"
	"go.chromium.org/luci/resultdb/internal/config"
	"go.chromium.org/luci/resultdb/internal/invocations"
	"go.chromium.org/luci/resultdb/internal/masking"
	"go.chromium.org/luci/resultdb/internal/permissions"
	"go.chromium.org/luci/resultdb/internal/rootinvocations"
	"go.chromium.org/luci/resultdb/internal/tasks"
	"go.chromium.org/luci/resultdb/internal/tasks/taskspb"
	"go.chromium.org/luci/resultdb/internal/tracing"
	"go.chromium.org/luci/resultdb/internal/workunits"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

const (
	// WorkUnitsTaskStateProcessID is the process ID for tracking the execution state
	// of a specific PublishWorkUnitsTask pagination step to ensure idempotency.
	WorkUnitsTaskStateProcessID = "wu-publisher-task"

	// workUnitFetchChunkSize is the maximum number of work units to fetch from Spanner
	// in a single chunk when building a notification batch.
	workUnitFetchChunkSize = 200
)

var maxWorkUnitPubSubMessageSize = maxPubSubMessageSize

// workUnitPublisher is a helper struct for publishing work units.
type workUnitPublisher struct {
	// task is the task payload.
	task *taskspb.PublishWorkUnitsTask

	// resultDBHostname is the hostname of the ResultDB service.
	resultDBHostname string
}

// handleWorkUnitPublisher handles the work unit publisher task.
func (p *workUnitPublisher) handleWorkUnitPublisher(ctx context.Context) (err error) {
	ctx, s := tracing.Start(ctx, "go.chromium.org/luci/resultdb/internal/services/pubsub.handleWorkUnitPublisher")
	defer func() { tracing.End(s, err) }()

	task := p.task
	if len(task.WorkUnitIds) == 0 {
		logging.Infof(ctx, "No work units to process for root invocation %q", task.RootInvocationId)
		return nil
	}

	if task.CurrentWorkUnitIndex < 0 || int(task.CurrentWorkUnitIndex) >= len(task.WorkUnitIds) {
		return errors.Fmt("CurrentWorkUnitIndex %d is out of bounds for WorkUnitIds list of size %d", task.CurrentWorkUnitIndex, len(task.WorkUnitIds))
	}

	rootInvID := rootinvocations.ID(task.RootInvocationId)

	// 1. Reads Root Invocation metadata.
	rootInv, err := rootinvocations.Read(span.Single(ctx), rootInvID)
	if err != nil {
		return errors.Fmt("read root invocation %q: %w", rootInvID.Name(), err)
	}

	// 2. Fetch service config for URL generation.
	cfg, err := config.Service(ctx)
	if err != nil {
		return errors.Fmt("fetch service config: %w", err)
	}

	// 3. Check for existing Task State Checkpoint.
	project, _ := realms.Split(rootInv.Realm)
	checkpointKey := p.taskStateCheckpointKey(project, rootInvID)
	exists, err := checkpoints.Exists(span.Single(ctx), checkpointKey)
	if err != nil {
		return errors.Fmt("check task state checkpoint %q: %w", checkpointKey, err)
	}
	if exists {
		logging.Infof(ctx, "Task state checkpoint already exists for root invocation %q and index %d, skipping", rootInvID.Name(), task.CurrentWorkUnitIndex)
		return nil
	}

	// 4. Collect work units up to maxWorkUnitPubSubMessageSize in chunks.
	collectedDetails, nextIndex, hasMore, err := p.collectWorkUnits(ctx, rootInvID, cfg, rootInv)
	if err != nil {
		return err
	}

	// 5. Publish notification (if any work units were collected in this batch).
	// Published before the transaction because NotifyWorkUnits is non-transactional.
	// If the worker crashes before the transaction commits, Cloud Tasks retries this
	// task and downstream subscribers deduplicate via DeduplicationKey.
	if len(collectedDetails) > 0 {
		attrs := generateAttributes(rootInv)
		notification := &pb.WorkUnitsNotification{
			ResultdbHost:           p.resultDBHostname,
			RootInvocationMetadata: masking.RootInvocationMetadata(rootInv, cfg),
			WorkUnits:              collectedDetails,
			DeduplicationKey:       generateWorkUnitsDeduplicationKey(collectedDetails),
		}
		tasks.NotifyWorkUnits(ctx, notification, attrs)
	} else {
		logging.Infof(ctx, "No work units collected in this batch for root invocation %s", rootInvID.Name())
	}

	// 6. Commit Task State Checkpoint and Continuation Task atomically in a single Spanner transaction.
	return p.commitCheckpointAndContinuation(ctx, rootInvID, checkpointKey, nextIndex, hasMore)
}

// taskStateCheckpointKey constructs the checkpoint key for the current pagination index.
func (p *workUnitPublisher) taskStateCheckpointKey(project string, rootInvID rootinvocations.ID) checkpoints.Key {
	wuID := p.task.WorkUnitIds[p.task.CurrentWorkUnitIndex]
	uniquifier := fmt.Sprintf("workUnits/%s/indices/%d", wuID, p.task.CurrentWorkUnitIndex)
	return checkpoints.Key{
		Project:    project,
		ResourceID: string(rootInvID),
		ProcessID:  WorkUnitsTaskStateProcessID,
		Uniquifier: uniquifier,
	}
}

// collectWorkUnits collects work units starting from CurrentWorkUnitIndex in bounded
// chunks, ensuring total message size does not exceed maxWorkUnitPubSubMessageSize.
func (p *workUnitPublisher) collectWorkUnits(
	ctx context.Context,
	rootInvID rootinvocations.ID,
	cfg *config.CompiledServiceConfig,
	rootInv *rootinvocations.RootInvocationRow,
) (collectedDetails []*pb.WorkUnitsNotification_WorkUnitDetails, nextIndex int32, hasMore bool, err error) {
	ctx, s := tracing.Start(ctx, "go.chromium.org/luci/resultdb/internal/services/pubsub.collectWorkUnits")
	defer func() { tracing.End(s, err) }()

	propertyCache := make(map[workunits.ID]*structpb.Struct)

	// Include a 64-char placeholder to account for the exact wire size of the
	// hex-encoded SHA-256 DeduplicationKey when checking message size limits.
	baseNotification := &pb.WorkUnitsNotification{
		ResultdbHost:           p.resultDBHostname,
		RootInvocationMetadata: masking.RootInvocationMetadata(rootInv, cfg),
		DeduplicationKey:       strings.Repeat("0", 64),
	}
	baseSize := proto.Size(baseNotification)
	currentSize := 0

	// Fetch work units in bounded chunks rather than reading all remaining work
	// units upfront. Because the task yields as soon as the batch reaches
	// maxWorkUnitPubSubMessageSize, reading all remaining work units upfront
	// would repeatedly fetch and discard unprocessed tail work units across
	// continuation tasks.
	total := len(p.task.WorkUnitIds)
	for chunkStart := int(p.task.CurrentWorkUnitIndex); chunkStart < total; chunkStart += workUnitFetchChunkSize {
		chunkEnd := chunkStart + workUnitFetchChunkSize
		if chunkEnd > total {
			chunkEnd = total
		}
		chunkWUIDs := p.task.WorkUnitIds[chunkStart:chunkEnd]

		startIDs := make([]workunits.ID, 0, len(chunkWUIDs))
		invIDs := make(invocations.IDSet, len(chunkWUIDs))
		for _, wuID := range chunkWUIDs {
			wuInternalID := workunits.ID{RootInvocationID: rootInvID, WorkUnitID: wuID}
			startIDs = append(startIDs, wuInternalID)
			invIDs.Add(wuInternalID.LegacyInvocationID())
		}

		// Batch fetch ancestors for work units in this chunk.
		chunkFetched, err := func() (map[workunits.ID]*workunits.WorkUnitRow, error) {
			roCtx, cancel := span.ReadOnlyTransaction(ctx)
			defer cancel()
			return workunits.ReadAncestorsBatch(roCtx, startIDs, workunits.ExcludeExtendedProperties)
		}()
		if err != nil {
			return nil, 0, false, errors.Fmt("batch read ancestors for work units: %w", err)
		}

		// Batch check for artifacts.
		hasArtifactsSet, err := artifacts.FilterHasArtifacts(span.Single(ctx), invIDs)
		if err != nil {
			return nil, 0, false, errors.Fmt("batch check artifacts for work units: %w", err)
		}

		for i := chunkStart; i < chunkEnd; i++ {
			wuID := p.task.WorkUnitIds[i]
			wuInternalID := workunits.ID{RootInvocationID: rootInvID, WorkUnitID: wuID}
			wuInvID := wuInternalID.LegacyInvocationID()

			mergedProps, err := p.calculateMergedPropertiesLocal(wuInternalID, chunkFetched, propertyCache)
			if err != nil {
				return nil, 0, false, err
			}

			row, ok := chunkFetched[wuInternalID]
			if !ok {
				return nil, 0, false, errors.Fmt("work unit %s not found in fetched map", wuInternalID.Name())
			}

			wuDetail := &pb.WorkUnitsNotification_WorkUnitDetails{
				WorkUnitName:              pbutil.WorkUnitName(string(rootInvID), wuID),
				HasArtifacts:              hasArtifactsSet.Has(wuInvID),
				MergedInheritedProperties: mergedProps,
				WorkUnit:                  masking.WorkUnit(row, permissions.FullAccess, pb.WorkUnitView_WORK_UNIT_VIEW_BASIC, cfg),
			}

			wuSize := proto.Size(wuDetail)
			if wuSize+baseSize > maxWorkUnitPubSubMessageSize {
				return nil, 0, false, errors.Fmt("work unit %s is too large (%d bytes) to fit in a Pub/Sub message", wuDetail.WorkUnitName, wuSize)
			}

			if currentSize+wuSize+baseSize > maxWorkUnitPubSubMessageSize {
				// Current batch is full; yield and resume at index i in continuation task.
				return collectedDetails, int32(i), true, nil
			}

			collectedDetails = append(collectedDetails, wuDetail)
			currentSize += wuSize
		}
	}

	return collectedDetails, int32(total), false, nil
}

// commitCheckpointAndContinuation commits the task state checkpoint and
// enqueues the continuation task in a single transaction.
func (p *workUnitPublisher) commitCheckpointAndContinuation(
	ctx context.Context,
	rootInvID rootinvocations.ID,
	checkpointKey checkpoints.Key,
	nextIndex int32,
	hasMore bool,
) error {
	_, err := span.ReadWriteTransaction(ctx, func(ctx context.Context) error {
		// 1. Re-check task state checkpoint within transaction.
		exists, err := checkpoints.Exists(ctx, checkpointKey)
		if err != nil {
			return errors.Fmt("check task state checkpoint in transaction: %w", err)
		}
		if exists {
			return nil
		}

		// 2. Insert task state checkpoint.
		span.BufferWrite(ctx, checkpoints.Insert(ctx, checkpointKey, CheckpointTTL))

		// 3. Schedule continuation task if necessary.
		if hasMore {
			nextWUID := p.task.WorkUnitIds[nextIndex]
			payload := &taskspb.PublishWorkUnitsTask{
				RootInvocationId:     p.task.RootInvocationId,
				WorkUnitIds:          p.task.WorkUnitIds,
				CurrentWorkUnitIndex: nextIndex,
			}
			if err := tq.AddTask(ctx, &tq.Task{
				Title:   fmt.Sprintf("wu-pubsub-cont-%s-%s-%d", rootInvID, nextWUID, nextIndex),
				Payload: payload,
			}); err != nil {
				return errors.Fmt("schedule continuation task for work unit index %d: %w", nextIndex, err)
			}
			logging.Infof(ctx, "Scheduled continuation for work unit %q, index %d", nextWUID, nextIndex)
		}
		return nil
	})
	if err != nil {
		return errors.Fmt("commit checkpoint and continuation for root invocation %q at index %d: %w", rootInvID.Name(), nextIndex, err)
	}
	return nil
}

// generateWorkUnitsDeduplicationKey creates a unique key for task deduplication.
// The key is a SHA-256 hash of all work unit names in the batch.
func generateWorkUnitsDeduplicationKey(workUnits []*pb.WorkUnitsNotification_WorkUnitDetails) string {
	if len(workUnits) == 0 {
		return ""
	}
	h := sha256.New()
	for _, wu := range workUnits {
		h.Write([]byte(wu.WorkUnitName))
		// Null-byte separator prevents boundary collisions between adjacent names.
		h.Write([]byte{0})
	}
	return fmt.Sprintf("%x", h.Sum(nil))
}

// calculateMergedPropertiesLocal calculates the fully merged inherited properties
// using the fetched work units map completely in memory.
func (p *workUnitPublisher) calculateMergedPropertiesLocal(wuID workunits.ID, fetched map[workunits.ID]*workunits.WorkUnitRow, cache map[workunits.ID]*structpb.Struct) (*structpb.Struct, error) {
	if merged, ok := cache[wuID]; ok {
		return merged, nil
	}

	row, ok := fetched[wuID]
	if !ok {
		return nil, errors.Fmt("work unit %s not found in fetched map", wuID.Name())
	}

	merged := &structpb.Struct{Fields: make(map[string]*structpb.Value)}

	if row.ParentWorkUnitID.Valid {
		parentID := workunits.ID{
			RootInvocationID: wuID.RootInvocationID,
			WorkUnitID:       row.ParentWorkUnitID.StringVal,
		}
		parentMerged, err := p.calculateMergedPropertiesLocal(parentID, fetched, cache)
		if err != nil {
			return nil, err
		}
		if parentMerged != nil {
			for k, v := range parentMerged.Fields {
				merged.Fields[k] = proto.Clone(v).(*structpb.Value)
			}
		}
	}

	if row.InheritedProperties != nil {
		for k, v := range row.InheritedProperties.Fields {
			merged.Fields[k] = proto.Clone(v).(*structpb.Value)
		}
	}

	if len(merged.Fields) == 0 {
		merged = nil
	}

	cache[wuID] = merged
	return merged, nil
}
