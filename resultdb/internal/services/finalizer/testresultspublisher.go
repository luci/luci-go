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

package finalizer

import (
	"context"
	"crypto/sha256"
	"fmt"
	"strings"
	"time"

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

// handlePublishTestResultsTask handles the task to publish test results for a
// set of work units.
func handlePublishTestResultsTask(ctx context.Context, msg proto.Message, resultDBHostname string) error {
	// TODO(b/454120970): Remove the hostname check to enable by default
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

	// TODO(b/372537579): Implement proper batching based on message size to
	// stay within Pub/Sub limits. For now, one notification for all work units.
	dedupKey := fmt.Sprintf("%s-%s", task.WorkUnitIds, time.Now().UTC().Format(time.RFC3339))
	notification := &pb.TestResultsNotification{
		TestResultsByWorkUnit: testResultsNotification,
		ResultdbHost:          resultDBHostname,
		Sources:               rootInv.Sources,
		DeduplicationKey:      fmt.Sprintf("%x", sha256.Sum256([]byte(dedupKey))),
	}

	// Enqueue the task to publish the notification.
	tasks.NotifyTestResults(ctx, notification, attrs)
	return nil
}

// collectTestResultsForWorkUnits queries and collects all test results for the
// given work units. It returns a map of work unit names to their corresponding
// test results.
func collectTestResultsForWorkUnits(ctx context.Context, rootInvID rootinvocations.ID, workUnitIDs []string) (map[string][]*pb.TestResult, error) {
	testResultsByWorkUnit := make(map[string][]*pb.TestResult)
	legacyInvIDs := invocations.NewIDSet()
	workUnitIDMap := make(map[invocations.ID]string) // Map legacy inv ID to work unit ID

	for _, wuID := range workUnitIDs {
		legacyInvID := workunits.ID{RootInvocationID: rootInvID, WorkUnitID: wuID}.LegacyInvocationID()
		legacyInvIDs.Add(legacyInvID)
		workUnitIDMap[legacyInvID] = wuID
		testResultsByWorkUnit[pbutil.WorkUnitName(string(rootInvID), wuID)] = []*pb.TestResult{}
	}

	q := &testresults.Query{
		InvocationIDs: legacyInvIDs,
		Predicate:     &pb.TestResultPredicate{},
		Mask:          testresults.AllFields,
	}

	err := q.Run(span.Single(ctx), func(tr *pb.TestResult) error {
		// TODO(b/454120970): Update testresults.Query to return the correct
		// test results name format
		// The test result name from the query will be in the legacy format.
		legacyInvID, testID, resultID := testresults.MustParseLegacyName(tr.Name)
		wuID := workUnitIDMap[legacyInvID]

		// Reconstruct the name in the new format.
		tr.Name = pbutil.TestResultName(string(rootInvID), wuID, testID, resultID)

		wuName := pbutil.WorkUnitName(string(rootInvID), wuID)
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
