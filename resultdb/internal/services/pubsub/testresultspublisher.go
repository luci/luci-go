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
	"time"

	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/server/auth/realms"
	"go.chromium.org/luci/server/span"
	"go.chromium.org/luci/server/tq"

	"go.chromium.org/luci/resultdb/internal/checkpoints"
	"go.chromium.org/luci/resultdb/internal/invocations"
	"go.chromium.org/luci/resultdb/internal/rootinvocations"
	"go.chromium.org/luci/resultdb/internal/tasks"
	"go.chromium.org/luci/resultdb/internal/tasks/taskspb"
	"go.chromium.org/luci/resultdb/internal/testresults"
	"go.chromium.org/luci/resultdb/internal/tracing"
	"go.chromium.org/luci/resultdb/internal/workunits"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

const (
	// Pubsub message attributes
	androidBranchFilter  = "primary_build_android_branch"
	androidTargetFilter  = "primary_build_android_target"
	luciProjectFilter    = "luci_project"
	definitionNameFilter = "definition_name"

	// maxPubSubMessageSize is the maximum size of a Pub/Sub message. We use 9MB
	// as a safe limit to leave room for overhead, as the official limit is
	// 10MB (https://docs.cloud.google.com/pubsub/quotas#resource_limits).
	maxPubSubMessageSize = 9 * 1024 * 1024

	// defaultPageSize is the default number of test results to fetch per page
	// from Spanner.
	defaultPageSize = 5000

	// CheckpointProcessID is the process ID for test results publisher
	// checkpoints.
	CheckpointProcessID = "test-results-publisher"

	// CheckpointTTL specifies the TTL for checkpoints in Spanner.
	CheckpointTTL = 7 * 24 * time.Hour
)

// testResultsPublisher is a helper struct for publishing test results.
type testResultsPublisher struct {
	// task is the task payload.
	task *taskspb.PublishTestResultsTask

	// resultDBHostname is the hostname of the ResultDB service.
	resultDBHostname string

	// pageSize is the number of test results to query per page.
	pageSize int
}

// WorkUnitPageToken represents the state of pagination across work units.
type WorkUnitPageToken struct {
	// workUnitIndex is the index of the current work unit.
	workUnitIndex int32

	// pageToken is the page token for test results in the current work unit.
	pageToken string
}

// handleTestResultsPublisher handles the test results publisher task.
func (p *testResultsPublisher) handleTestResultsPublisher(ctx context.Context) (err error) {
	ctx, s := tracing.Start(ctx, "go.chromium.org/luci/resultdb/internal/services/pubsub.handleTestResultsPublisher")
	defer func() { tracing.End(s, err) }()

	if !strings.HasPrefix(p.resultDBHostname, "staging.") {
		return nil
	}

	task := p.task
	if task.CurrentWorkUnitIndex < 0 || int(task.CurrentWorkUnitIndex) >= len(task.WorkUnitIds) {
		return errors.Fmt("CurrentWorkUnitIndex %d is out of bounds for WorkUnitIds list of size %d", task.CurrentWorkUnitIndex, len(task.WorkUnitIds))
	}

	// 1. Reads Root Invocation to check its state and get sources.
	rootInvID := rootinvocations.ID(task.RootInvocationId)
	rootInv, err := rootinvocations.Read(span.Single(ctx), rootInvID)
	if err != nil {
		return errors.Fmt("read root invocation %q: %w", rootInvID.Name(), err)
	}

	// 2. Checks StreamingExportState: Only publish if metadata is final.
	if rootInv.StreamingExportState != pb.RootInvocation_METADATA_FINAL {
		logging.Infof(ctx, "Root invocation %q is not ready for streaming export, skipping test result notification", rootInvID.Name())
		return nil
	}

	// 3. Checks for existing checkpoint.
	checkpointKey, exists, err := p.checkCheckpoint(ctx, rootInvID, rootInv.Realm)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}

	// 4. Collects test results.
	collectedResults, nextToken, err := p.collectTestResults(ctx, rootInvID)
	if err != nil {
		return errors.Fmt("collect test results: %w", err)
	}

	// 5. Partitions test results into notifications.
	notifications, err := p.partitionTestResults(ctx, collectedResults, rootInv.Sources)
	if err != nil {
		return errors.Fmt("partition test results: %w", err)
	}

	// 6. Constructs the message attributes.
	attrs := generateAttributes(rootInv)

	// 7. Publishes notifications.
	// We do this before the transaction to ensure that if the transaction fails
	// and retries, we might send redundant notifications (handled by
	// deduplication), but we won't lose them if the transaction commits but
	// publishing fails (which is unlikely but possible if we did it after).
	// Since NotifyTestResults is non-transactional, it must be called outside a
	// transaction.
	if err := p.publishNotifications(ctx, notifications, attrs); err != nil {
		return errors.Fmt("publish notifications: %w", err)
	}

	// 8. Schedules continuation in a single transaction.
	return p.commitCheckpointAndContinuation(ctx, rootInvID, checkpointKey, nextToken)
}

// collectTestResults gathers test results across work units and pages up to a
// total size limit.
func (p *testResultsPublisher) collectTestResults(ctx context.Context, rootInvID rootinvocations.ID) (collectedResults []*pb.TestResultsNotification_TestResultsByWorkUnit, nextToken *WorkUnitPageToken, err error) {
	ctx, s := tracing.Start(ctx, "go.chromium.org/luci/resultdb/internal/services/pubsub.collectTestResults")
	defer func() { tracing.End(s, err) }()

	currentSize := 0
	task := p.task
	currentPageToken := task.PageToken
	for i := int(task.CurrentWorkUnitIndex); i < len(task.WorkUnitIds); i++ {
		wuID := task.WorkUnitIds[i]
		legacyInvID := workunits.ID{RootInvocationID: rootInvID, WorkUnitID: wuID}.LegacyInvocationID()

		// Queries test results up to the page size for the current work unit.
		for {
			q := &testresults.Query{
				InvocationIDs: invocations.NewIDSet(legacyInvID),
				Predicate:     &pb.TestResultPredicate{},
				Mask:          testresults.AllFields,
				PageSize:      p.pageSize,
				PageToken:     currentPageToken,
			}

			pageTRs, nextPageToken, err := q.Fetch(span.Single(ctx))
			if err != nil {
				return nil, nil, errors.Fmt("query test results for work unit ID %q: %w", wuID, err)
			}

			if len(pageTRs) > 0 {
				wuName := pbutil.WorkUnitName(string(rootInvID), wuID)
				newWUBlock := &pb.TestResultsNotification_TestResultsByWorkUnit{
					WorkUnitName: wuName,
					TestResults:  pageTRs,
				}
				newBlockSize := proto.Size(newWUBlock)

				// Scenario 1:
				// This should be rare, but a single page of results for a work
				// unit is too large. We'll handle this by returning just this
				// block and the next token.
				if newBlockSize > maxPubSubMessageSize {
					// If we have collected results, return them first.
					if len(collectedResults) > 0 {
						return collectedResults, &WorkUnitPageToken{workUnitIndex: int32(i), pageToken: currentPageToken}, nil
					}

					var token *WorkUnitPageToken
					if nextPageToken != "" {
						token = &WorkUnitPageToken{
							workUnitIndex: int32(i),
							pageToken:     nextPageToken,
						}
					} else if i < len(task.WorkUnitIds)-1 {
						token = &WorkUnitPageToken{
							workUnitIndex: int32(i) + 1,
							pageToken:     "",
						}
					}
					return []*pb.TestResultsNotification_TestResultsByWorkUnit{newWUBlock}, token, nil
				}

				// Scenario 2:
				// Keeps appending the current work unit block to the test
				// result collection until the current block exceeds the limit.
				if currentSize+newBlockSize > maxPubSubMessageSize {
					return collectedResults, &WorkUnitPageToken{workUnitIndex: int32(i), pageToken: currentPageToken}, nil
				}

				lastBlockIndex := len(collectedResults) - 1
				if lastBlockIndex >= 0 && collectedResults[lastBlockIndex].WorkUnitName == wuName {
					// Merges the current block into the last block if they are from the same work unit.
					lastBlock := collectedResults[lastBlockIndex]
					currentSize -= proto.Size(lastBlock)
					lastBlock.TestResults = append(lastBlock.TestResults, pageTRs...)
					currentSize += proto.Size(lastBlock)
				} else {
					// Appends the current block to the collection if they are from different work units.
					collectedResults = append(collectedResults, newWUBlock)
					currentSize += newBlockSize
				}
			}

			// Exits the iteration of the current work unit and resets the page
			// token if no more pages for this work unit.
			if nextPageToken == "" {
				currentPageToken = ""
				break
			}
			currentPageToken = nextPageToken
		}
	}
	// All work units processed.
	return collectedResults, nil, nil
}

// createNotification creates a new notification.
func (p *testResultsPublisher) createNotification(batch []*pb.TestResultsNotification_TestResultsByWorkUnit, sources *pb.Sources) *pb.TestResultsNotification {
	return &pb.TestResultsNotification{
		TestResultsByWorkUnit: batch,
		ResultdbHost:          p.resultDBHostname,
		Sources:               sources,
		DeduplicationKey:      generateDeduplicationKey(batch),
	}
}

// partitionTestResults splits the collected test results into notification
// messages.
func (p *testResultsPublisher) partitionTestResults(ctx context.Context, collectedResults []*pb.TestResultsNotification_TestResultsByWorkUnit, sources *pb.Sources) (notifications []*pb.TestResultsNotification, err error) {
	ctx, s := tracing.Start(ctx, "go.chromium.org/luci/resultdb/internal/services/pubsub.partitionTestResults")
	defer func() { tracing.End(s, err) }()

	if len(collectedResults) == 0 {
		return notifications, nil
	}

	var currentBatch []*pb.TestResultsNotification_TestResultsByWorkUnit
	currentSize := 0
	for _, wuBlock := range collectedResults {
		blockSize := proto.Size(wuBlock)

		// Handle a single work unit block that's already too large.
		// This requires splitting the TestResults within this block.
		if blockSize > maxPubSubMessageSize {
			wuNotifications, err := p.splitWorkUnitBlock(wuBlock, sources)
			if err != nil {
				return nil, err
			}
			notifications = append(notifications, wuNotifications...)
			continue
		}

		// Current batch is full, create a notification.
		if currentSize > 0 && currentSize+blockSize > maxPubSubMessageSize {
			notifications = append(notifications, p.createNotification(currentBatch, sources))
			currentBatch = nil
			currentSize = 0
		}

		currentBatch = append(currentBatch, wuBlock)
		currentSize += blockSize
	}

	// Add the last batch.
	if len(currentBatch) > 0 {
		notifications = append(notifications, p.createNotification(currentBatch, sources))
	}
	return notifications, nil
}

// splitWorkUnitBlock splits a single large TestResultsByWorkUnit into multiple
// notifications.
func (p *testResultsPublisher) splitWorkUnitBlock(wuBlock *pb.TestResultsNotification_TestResultsByWorkUnit, sources *pb.Sources) ([]*pb.TestResultsNotification, error) {
	var notifications []*pb.TestResultsNotification
	var trBatch []*pb.TestResult
	baseWUSize := proto.Size(&pb.TestResultsNotification_TestResultsByWorkUnit{WorkUnitName: wuBlock.WorkUnitName})
	trCurrentSize := baseWUSize

	for _, tr := range wuBlock.TestResults {
		trSize := proto.Size(tr)
		if trSize+baseWUSize > maxPubSubMessageSize {
			return nil, errors.Fmt("Single test result for %q (TestID: %q, ResultID: %q) exceeds Pub/Sub size limit: %d bytes", wuBlock.WorkUnitName, tr.TestId, tr.ResultId, trSize)
		}

		if trCurrentSize+trSize > maxPubSubMessageSize {
			// Current trBatch is full.
			wuResults := []*pb.TestResultsNotification_TestResultsByWorkUnit{{
				WorkUnitName: wuBlock.WorkUnitName,
				TestResults:  trBatch,
			}}
			notifications = append(notifications, p.createNotification(wuResults, sources))
			trBatch = nil
			trCurrentSize = baseWUSize
		}
		trBatch = append(trBatch, tr)
		trCurrentSize += trSize
	}

	// Add the last trBatch for this work unit.
	if len(trBatch) > 0 {
		wuResults := []*pb.TestResultsNotification_TestResultsByWorkUnit{{
			WorkUnitName: wuBlock.WorkUnitName,
			TestResults:  trBatch,
		}}
		notifications = append(notifications, p.createNotification(wuResults, sources))
	}
	return notifications, nil
}

// publishNotifications enqueues the given notifications to the task queue.
func (p *testResultsPublisher) publishNotifications(ctx context.Context, notifications []*pb.TestResultsNotification, attrs map[string]string) (err error) {
	ctx, s := tracing.Start(ctx, "go.chromium.org/luci/resultdb/internal/services/pubsub.publishNotifications")
	defer func() { tracing.End(s, err) }()

	for _, notification := range notifications {
		tasks.NotifyTestResults(ctx, notification, attrs)
	}
	return nil
}

// enqueueContinuationTask enqueues a new task to continue processing.
// It must be called within a Spanner transaction.
func (p *testResultsPublisher) enqueueContinuationTask(ctx context.Context, rootInvID rootinvocations.ID, nextToken *WorkUnitPageToken) error {
	payload := &taskspb.PublishTestResultsTask{
		RootInvocationId:     p.task.RootInvocationId,
		WorkUnitIds:          p.task.WorkUnitIds,
		CurrentWorkUnitIndex: nextToken.workUnitIndex,
		PageToken:            nextToken.pageToken,
	}
	wuID := p.task.WorkUnitIds[nextToken.workUnitIndex]
	if err := tq.AddTask(ctx, &tq.Task{
		Title:   fmt.Sprintf("pubsub-tr-rootInvocations/%s/workUnits/%s/pageTokens/%s", rootInvID, wuID, nextToken.pageToken),
		Payload: payload,
	}); err != nil {
		return errors.Fmt("schedule continuation task for page: %w", err)
	}
	logging.Infof(ctx, "Scheduled continuation for work unit %q, index %d, next page token %q", wuID, nextToken.workUnitIndex, nextToken.pageToken)
	return nil
}

// checkCheckpoint checks if a checkpoint already exists for the current task state.
func (p *testResultsPublisher) checkCheckpoint(ctx context.Context, rootInvID rootinvocations.ID, realm string) (checkpoints.Key, bool, error) {
	project, _ := realms.Split(realm)
	uniquifier := fmt.Sprintf("workUnits/%s/pageTokens/%s", p.task.WorkUnitIds[p.task.CurrentWorkUnitIndex], p.task.PageToken)
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
func (p *testResultsPublisher) commitCheckpointAndContinuation(ctx context.Context, rootInvID rootinvocations.ID, checkpointKey checkpoints.Key, nextPageToken *WorkUnitPageToken) error {
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
		return errors.Fmt("Continuation transaction failed for root invocation %q and next page token %q: %w", rootInvID.Name(), nextPageToken, err)
	}
	return nil
}

// generateAttributes generates the pubsub message attributes.
func generateAttributes(rootInv *rootinvocations.RootInvocationRow) map[string]string {
	attrs := make(map[string]string)
	project, _ := realms.Split(rootInv.Realm)
	attrs[luciProjectFilter] = project

	if rootInv.PrimaryBuild != nil {
		if abd := rootInv.PrimaryBuild.GetAndroidBuild(); abd != nil {
			attrs[androidBranchFilter] = abd.Branch
			attrs[androidTargetFilter] = abd.BuildTarget
		}
	}
	if rootInv.Definition != nil {
		attrs[definitionNameFilter] = rootInv.Definition.Name
	}
	return attrs
}

// generateDeduplicationKey creates a unique key for task deduplication.
// The key is based on the sorted work unit names and the first test result
// of the first work unit in the batch.
func generateDeduplicationKey(wuResults []*pb.TestResultsNotification_TestResultsByWorkUnit) string {
	if len(wuResults) == 0 {
		return ""
	}

	// Use the first test result of the first work unit to make the key unique
	// to this specific batch of results.
	firstWU := wuResults[0]
	var firstTRKey string
	if len(firstWU.TestResults) > 0 {
		// Test results are already sorted by TestId and ResultId.
		firstTR := firstWU.TestResults[0]
		firstTRKey = fmt.Sprintf("%s-%s", firstTR.TestId, firstTR.ResultId)
	} else {
		firstTRKey = "no_test_results"
	}

	keyContent := fmt.Sprintf("%s-%s", firstWU.WorkUnitName, firstTRKey)
	return fmt.Sprintf("%x", sha256.Sum256([]byte(keyContent)))
}
