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
	"fmt"

	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/server/auth/realms"
	"go.chromium.org/luci/server/span"
	"go.chromium.org/luci/server/tq"

	"go.chromium.org/luci/resultdb/internal/checkpoints"
	"go.chromium.org/luci/resultdb/internal/permissions"
	"go.chromium.org/luci/resultdb/internal/rootinvocations"
	"go.chromium.org/luci/resultdb/internal/tasks"
	"go.chromium.org/luci/resultdb/internal/tasks/taskspb"
	"go.chromium.org/luci/resultdb/internal/testaggregations"
	"go.chromium.org/luci/resultdb/internal/tracing"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

const (
	aggregationLevelFilter = "aggregation_level"

	// maxTestAggregationsCount is the maximum number of test aggregations to
	// include in a single Pub/Sub message.
	maxTestAggregationsCount = 10000

	// testAggregationsPageSize is the number of test aggregations to fetch per
	// page from Spanner.
	testAggregationsPageSize = 5000
)

var aggregationLevels = []pb.AggregationLevel{
	pb.AggregationLevel_INVOCATION,
	pb.AggregationLevel_MODULE,
	pb.AggregationLevel_COARSE,
	pb.AggregationLevel_FINE,
}

// testAggregationsPublisher is a helper struct for publishing test aggregations.
type testAggregationsPublisher struct {
	// task is the task payload.
	task *taskspb.PublishTestAggregationsTask

	// resultDBHostname is the hostname of the ResultDB service.
	resultDBHostname string

	// maxPubSubMessageSize is the maximum size of a Pub/Sub message.
	// Defaults to maxPubSubMessageSize constant if 0.
	maxPubSubMessageSize int
}

// AggregationPageToken represents the state of pagination across aggregation levels.
type AggregationPageToken struct {
	// levelIndex is the index of the current aggregation level.
	levelIndex int32

	// pageToken is the page token for test aggregations in the current level.
	pageToken string
}

// handleTestAggregationsPublisher handles the test aggregation publisher task.
func (p *testAggregationsPublisher) handleTestAggregationsPublisher(ctx context.Context) (err error) {
	ctx, s := tracing.Start(ctx, "go.chromium.org/luci/resultdb/internal/services/pubsub.handleTestAggregationsPublisher")
	defer func() { tracing.End(s, err) }()

	task := p.task
	rootInvID := rootinvocations.ID(task.RootInvocationId)

	if p.maxPubSubMessageSize == 0 {
		p.maxPubSubMessageSize = maxPubSubMessageSize
	}

	// 1. Read Root Invocation to check its state.
	rootInv, err := rootinvocations.Read(span.Single(ctx), rootInvID)
	if err != nil {
		return errors.Fmt("read root invocation %q: %w", rootInvID.Name(), err)
	}

	// 2. Check StreamingExportState: Only publish if metadata is final.
	if rootInv.StreamingExportState != pb.RootInvocation_METADATA_FINAL {
		logging.Infof(ctx, "Root invocation %q is not ready for streaming export, skipping test aggregation notification", rootInvID.Name())
		return nil
	}

	// 3. Check for existing checkpoint.
	checkpointKey, exists, err := p.checkCheckpoint(ctx, rootInvID, rootInv.Realm)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}

	// 4. Collect test aggregations.
	collectedAggregations, nextToken, err := p.collectTestAggregations(ctx, rootInvID)
	if err != nil {
		return errors.Fmt("collect test aggregations: %w", err)
	}

	// 5. Construct the message attributes.
	attrs := generateAttributes(rootInv)

	// 6. Publish notifications.
	// We do this before the transaction to ensure that if the transaction fails
	// and retries, we might send redundant notifications (handled by
	// deduplication), but we won't lose them if the transaction commits but
	// publishing fails (which is unlikely but possible if we did it after).
	// Since NotifyTestAggregations is non-transactional, it must be called
	// outside a transaction.
	if err := p.publishNotifications(ctx, collectedAggregations, attrs); err != nil {
		return errors.Fmt("publish test aggregation notifications: %w", err)
	}

	// 7. Schedule continuation in a single transaction.
	return p.commitCheckpointAndContinuation(ctx, rootInvID, checkpointKey, nextToken)
}

// collectTestAggregations gathers test aggregations across levels and pages up
// to a total size limit.
func (p *testAggregationsPublisher) collectTestAggregations(ctx context.Context, rootInvID rootinvocations.ID) (collectedAggregations []*pb.TestAggregationsNotification, nextToken *AggregationPageToken, err error) {
	ctx, s := tracing.Start(ctx, "go.chromium.org/luci/resultdb/internal/services/pubsub.collectTestAggregations")
	defer func() { tracing.End(s, err) }()

	task := p.task
	startLevelIndex := int(task.CurrentAggregationLevelIndex)
	startPageToken := task.PageToken

	currentSize := 0
	currentCount := 0
	for i := startLevelIndex; i < len(aggregationLevels); i++ {
		level := aggregationLevels[i]
		q := &testaggregations.SingleLevelQuery{
			RootInvocationID: rootInvID,
			Level:            level,
			Access:           permissions.RootInvocationAccess{Level: permissions.FullAccess},
			PageSize:         testAggregationsPageSize,
		}

		currentPageToken := startPageToken
		// Reset startPageToken for subsequent levels.
		startPageToken = ""

		for {
			batch, nextPageToken, err := q.Fetch(span.Single(ctx), currentPageToken)
			if err != nil {
				return nil, nil, errors.Fmt("fetch aggregations level %q: %w", level, err)
			}

			batchSize := 0
			for _, agg := range batch {
				batchSize += proto.Size(agg)
			}

			// Check if adding this batch exceeds the limits.
			if currentCount+len(batch) > maxTestAggregationsCount || currentSize+batchSize > p.maxPubSubMessageSize {
				// Scenario 1:
				// A single page of aggregations is too large.
				// We fail the task as this scenario is expected to be impossible with current constraints.
				if batchSize > p.maxPubSubMessageSize {
					return nil, nil, errors.Fmt("a single page of aggregations (size %d) exceeds the Pub/Sub message limit (%d)", batchSize, p.maxPubSubMessageSize)
				}

				// Scenario 2:
				// We cannot fit this entire batch.
				// Since we want to align with page boundaries to avoid duplication and
				// complexity with page tokens, we stop here and return what we have.
				// The next token will be the start of the current page.
				return collectedAggregations, &AggregationPageToken{levelIndex: int32(i), pageToken: currentPageToken}, nil
			}

			// Add the batch to the collection.
			if len(batch) > 0 {
				if len(collectedAggregations) == 0 || collectedAggregations[len(collectedAggregations)-1].AggregationLevel != level {
					collectedAggregations = append(collectedAggregations, &pb.TestAggregationsNotification{
						RootInvocation:   rootInvID.Name(),
						ResultdbHost:     p.resultDBHostname,
						AggregationLevel: level,
					})
				}

				lastBlock := collectedAggregations[len(collectedAggregations)-1]
				lastBlock.TestAggregations = append(lastBlock.TestAggregations, batch...)
				currentSize += batchSize
				currentCount += len(batch)
			}

			if nextPageToken == "" {
				break
			}
			currentPageToken = nextPageToken
		}
	}

	return collectedAggregations, nil, nil
}


// publishNotifications publishes the notifications to Pub/Sub.
func (p *testAggregationsPublisher) publishNotifications(ctx context.Context, notifications []*pb.TestAggregationsNotification, attrs map[string]string) error {
	for _, notification := range notifications {
		// Create a copy of attributes for this notification.
		nAttrs := make(map[string]string, len(attrs)+1)
		for k, v := range attrs {
			nAttrs[k] = v
		}
		nAttrs[aggregationLevelFilter] = notification.AggregationLevel.String()

		tasks.NotifyTestAggregations(ctx, notification, nAttrs)
	}
	return nil
}

// enqueueContinuationTask enqueues a new task to continue processing.
// It must be called within a Spanner transaction.
func (p *testAggregationsPublisher) enqueueContinuationTask(ctx context.Context, rootInvID rootinvocations.ID, nextToken *AggregationPageToken) error {
	payload := &taskspb.PublishTestAggregationsTask{
		RootInvocationId:             p.task.RootInvocationId,
		CurrentAggregationLevelIndex: nextToken.levelIndex,
		PageToken:                    nextToken.pageToken,
	}
	// Use level index and page token in task title for uniqueness and debugging.
	if err := tq.AddTask(ctx, &tq.Task{
		Title:   fmt.Sprintf("pubsub-ta-rootInvocations/%s/level/%d/pageTokens/%s", rootInvID, nextToken.levelIndex, nextToken.pageToken),
		Payload: payload,
	}); err != nil {
		return errors.Fmt("schedule continuation task for page: %w", err)
	}
	logging.Infof(ctx, "Scheduled continuation for level index %d, next page token %q", nextToken.levelIndex, nextToken.pageToken)
	return nil
}

// checkCheckpoint checks if a checkpoint already exists for the current task
// state.
func (p *testAggregationsPublisher) checkCheckpoint(ctx context.Context, rootInvID rootinvocations.ID, realm string) (checkpoints.Key, bool, error) {
	project, _ := realms.Split(realm)
	uniquifier := fmt.Sprintf("level/%d/pageTokens/%s", p.task.CurrentAggregationLevelIndex, p.task.PageToken)
	checkpointKey := checkpoints.Key{
		Project:    project,
		ResourceID: string(rootInvID),
		ProcessID:  CheckpointProcessID,
		Uniquifier: uniquifier,
	}
	exists, err := checkpoints.Exists(span.Single(ctx), checkpointKey)
	if err != nil {
		return checkpoints.Key{}, false, errors.Fmt("check checkpoint existence %q: %w", checkpointKey, err)
	}
	if exists {
		logging.Infof(ctx, "Checkpoint already exists for resource ID %q and uniquifier %q, skipping", rootInvID.Name(), uniquifier)
		return checkpointKey, true, nil
	}
	return checkpointKey, false, nil
}

// commitCheckpointAndContinuation commits the checkpoint and enqueues the
// continuation task in a single transaction.
func (p *testAggregationsPublisher) commitCheckpointAndContinuation(ctx context.Context, rootInvID rootinvocations.ID, checkpointKey checkpoints.Key, nextPageToken *AggregationPageToken) error {
	_, err := span.ReadWriteTransaction(ctx, func(ctx context.Context) error {
		// Re-checks checkpoint within transaction.
		exists, err := checkpoints.Exists(ctx, checkpointKey)
		if err != nil {
			return errors.Fmt("check checkpoint existence in transaction: %w", err)
		}
		if exists {
			return nil
		}

		// Inserts checkpoint.
		span.BufferWrite(ctx, checkpoints.Insert(ctx, checkpointKey, CheckpointTTL))

		// Schedules continuation task if necessary.
		if nextPageToken != nil {
			if err := p.enqueueContinuationTask(ctx, rootInvID, nextPageToken); err != nil {
				return errors.Fmt("enqueue continuation task: %w", err)
			}
		}
		return nil
	})

	if err != nil {
		return errors.Fmt("Continuation transaction failed for root invocation %q and next page token %+v: %w", rootInvID.Name(), nextPageToken, err)
	}
	return nil
}
