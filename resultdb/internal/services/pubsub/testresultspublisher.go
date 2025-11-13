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

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/resultdb/internal/invocations"
	"go.chromium.org/luci/resultdb/internal/rootinvocations"
	"go.chromium.org/luci/resultdb/internal/tasks"
	"go.chromium.org/luci/resultdb/internal/tasks/taskspb"
	"go.chromium.org/luci/resultdb/internal/testresults"
	"go.chromium.org/luci/resultdb/internal/workunits"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

const (
	// maxPubSubMessageSize is the maximum size of a Pub/Sub message. We use 9MB
	// as a safe limit to leave room for overhead, as the official limit is
	// 10MB (https://docs.cloud.google.com/pubsub/quotas#resource_limits).
	maxPubSubMessageSize = 9 * 1024 * 1024
)

// handlePublishTestResultsTask handles the task to publish test results for a

func handlePublishTestResultsTask(ctx context.Context, msg proto.Message, resultDBHostname string) error {
	if !strings.HasPrefix(resultDBHostname, "staging.") {
		return nil
	}

	task := msg.(*taskspb.PublishTestResultsTask)
	rootInvID := rootinvocations.ID(task.RootInvocationId)

	// 1. Read Root Invocation to check its state and get sources.
	// We read outside a transaction first to quickly check the state.
	rootInv, err := rootinvocations.Read(span.Single(ctx), rootInvID)
	if err != nil {
		return errors.Fmt("failed to read root invocation %s: %w", rootInvID.Name(), err)
	}

	// 2. Check StreamingExportState: Only publish if metadata is final.
	if rootInv.StreamingExportState != pb.RootInvocation_METADATA_FINAL {
		logging.Infof(ctx, "Root invocation %s is not ready for streaming export, skipping test result notification", rootInvID.Name())
		return nil
	}

	// 3. Construct the message attributes.
	attrs := make(map[string]string)
	if rootInv.Properties != nil {
		if primaryBuild, ok := rootInv.Properties.Fields["primary_build"]; ok {
			if buildStruct := primaryBuild.GetStructValue(); buildStruct != nil {
				if branchVal, ok := buildStruct.Fields["branch"]; ok {
					attrs["branch"] = branchVal.GetStringValue()
				}
			}
		}
	}

	// 4. Collect test results for all work units.
	testResultsByWorkUnit, err := collectTestResultsForWorkUnits(ctx, rootInvID, task.WorkUnitIds)
	if err != nil {
		return err
	}
	testResultsNotification := testResultsNotification(testResultsByWorkUnit)

	// 5. Enqueue batched notifications.
	err = enqueueBatchedNotifications(ctx, testResultsNotification, resultDBHostname, rootInv.Sources, attrs, maxPubSubMessageSize)
	if err != nil {
		logging.Errorf(ctx, "Failed to enqueue batched notifications: %s", err)
		return err
	}
	return nil
}

// enqueueBatchedNotifications takes the test results grouped by work unit and
// enqueues them into TQ, respecting Pub/Sub message size limits.
func enqueueBatchedNotifications(ctx context.Context, testResultsNotification []*pb.TestResultsNotification_TestResultsByWorkUnit, resultDBHostname string, sources *pb.Sources, attrs map[string]string, maxSize int) error {
	var currentBatch []*pb.TestResultsNotification_TestResultsByWorkUnit
	currentSize := 0

	sendBatch := func() {
		if len(currentBatch) == 0 {
			return
		}

		// Create a stable representation of the work units in this batch.
		notification := &pb.TestResultsNotification{
			TestResultsByWorkUnit: currentBatch,
			ResultdbHost:          resultDBHostname,
			Sources:               sources,
			DeduplicationKey:      generateDeduplicationKey(currentBatch),
		}
		tasks.NotifyTestResults(ctx, notification, attrs)
		currentBatch = nil
		currentSize = 0
	}

	for _, wuResult := range testResultsNotification {
		if len(wuResult.TestResults) == 0 {
			continue
		}
		wuSize := proto.Size(wuResult)

		if wuSize <= maxSize {
			// Work unit fits, check if it fits in the current batch.
			if len(currentBatch) > 0 && currentSize+wuSize > maxSize {
				sendBatch()
			}
			currentBatch = append(currentBatch, wuResult)
			currentSize += wuSize
		} else {
			// This single work unit is too large, send existing batch first.
			sendBatch()

			// Split this large work unit into multiple batches.
			err := splitAndEnqueueWorkUnitResults(ctx, wuResult, resultDBHostname, sources, attrs, maxSize, sendBatch)
			if err != nil {
				return err
			}
		}
	}
	// Send any remaining batch.
	sendBatch()
	return nil
}

// splitAndEnqueueWorkUnitResults handles a single work unit whose test results
// exceed the maxSize, splitting them into multiple smaller notification tasks.
func splitAndEnqueueWorkUnitResults(ctx context.Context, wuResult *pb.TestResultsNotification_TestResultsByWorkUnit, resultDBHostname string, sources *pb.Sources, attrs map[string]string, maxSize int, sendBatch func()) error {
	var trBatch []*pb.TestResult
	baseWUSize := proto.Size(&pb.TestResultsNotification_TestResultsByWorkUnit{WorkUnitName: wuResult.WorkUnitName})
	trCurrentSize := baseWUSize

	for _, tr := range wuResult.TestResults {
		trSize := proto.Size(tr)
		if trSize+baseWUSize > maxSize {
			return errors.Fmt("Single test result for %s (TestID: %s, ResultID: %s) exceeds Pub/Sub size limit: %d bytes", wuResult.WorkUnitName, tr.TestId, tr.ResultId, trSize)
		}

		if trCurrentSize+trSize > maxSize {
			// Current trBatch is full, send it.
			if len(trBatch) > 0 {
				// We need to call the outer sendBatch, so we wrap and enqueue.
				wuResults := []*pb.TestResultsNotification_TestResultsByWorkUnit{
					{
						WorkUnitName: wuResult.WorkUnitName,
						TestResults:  trBatch,
					},
				}
				notification := &pb.TestResultsNotification{
					TestResultsByWorkUnit: wuResults,
					ResultdbHost:          resultDBHostname,
					Sources:               sources,
					DeduplicationKey:      generateDeduplicationKey(wuResults),
				}
				tasks.NotifyTestResults(ctx, notification, attrs)
			}
			// Start a new trBatch with the current test result.
			trBatch = []*pb.TestResult{tr}
			trCurrentSize = baseWUSize + trSize
		} else {
			// Add to the current trBatch.
			trBatch = append(trBatch, tr)
			trCurrentSize += trSize
		}
	}
	// Send the last trBatch for this work unit.
	if len(trBatch) > 0 {
		wuResults := []*pb.TestResultsNotification_TestResultsByWorkUnit{
			{
				WorkUnitName: wuResult.WorkUnitName,
				TestResults:  trBatch,
			},
		}
		notification := &pb.TestResultsNotification{
			TestResultsByWorkUnit: wuResults,
			ResultdbHost:          resultDBHostname,
			Sources:               sources,
			DeduplicationKey:      generateDeduplicationKey(wuResults),
		}
		tasks.NotifyTestResults(ctx, notification, attrs)
	}
	return nil
}

// collectTestResultsForWorkUnits queries and collects all test results for the
// given work units. It returns a map of work unit names to their corresponding
// test results, sorted by TestId and ResultId.
func collectTestResultsForWorkUnits(ctx context.Context, rootInvID rootinvocations.ID, workUnitIDs []string) (map[string][]*pb.TestResult, error) {
	testResultsByWorkUnit := make(map[string][]*pb.TestResult)
	legacyInvIDs := invocations.NewIDSet()

	for _, wuID := range workUnitIDs {
		wu := workunits.ID{RootInvocationID: rootInvID, WorkUnitID: wuID}
		legacyInvID := wu.LegacyInvocationID()
		legacyInvIDs.Add(legacyInvID)
		testResultsByWorkUnit[pbutil.WorkUnitName(string(rootInvID), wuID)] = []*pb.TestResult{}
	}

	q := &testresults.Query{
		InvocationIDs: legacyInvIDs,
		Predicate:     &pb.TestResultPredicate{},
		Mask:          testresults.AllFields,
	}

	err := q.Run(span.Single(ctx), func(tr *pb.TestResult) error {
		// Name is now in the new format, extract work unit ID from it.
		parts, err := pbutil.ParseTestResultName(tr.Name)
		if err != nil {
			return errors.Fmt("failed to parse the test result name %q: %w", tr.Name, err)
		}
		wuName := pbutil.WorkUnitName(parts.RootInvocationID, parts.WorkUnitID)
		testResultsByWorkUnit[wuName] = append(testResultsByWorkUnit[wuName], tr)
		return nil
	})

	if err != nil {
		return nil, errors.Fmt("failed to query test results for the root invocation %q and work units: %s: %w", string(rootInvID), workUnitIDs, err)
	}

	return testResultsByWorkUnit, nil
}

// testResultsNotification constructs the test results notification.
func testResultsNotification(testResultsByWorkUnit map[string][]*pb.TestResult) []*pb.TestResultsNotification_TestResultsByWorkUnit {
	testResultsNotification := make([]*pb.TestResultsNotification_TestResultsByWorkUnit, 0, len(testResultsByWorkUnit))
	for wuName, trs := range testResultsByWorkUnit {
		testResultsNotification = append(testResultsNotification, &pb.TestResultsNotification_TestResultsByWorkUnit{
			WorkUnitName: wuName,
			TestResults:  trs,
		})
	}
	return testResultsNotification
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
