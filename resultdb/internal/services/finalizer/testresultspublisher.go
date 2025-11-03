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
	"time"

	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/resultdb/internal/rootinvocations"
	"go.chromium.org/luci/resultdb/internal/tasks"
	"go.chromium.org/luci/resultdb/internal/tasks/taskspb"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

// handlePublishTestResultsTask handles the task to publish test results for a
// set of work units.
func handlePublishTestResultsTask(ctx context.Context, msg proto.Message, resultDBHostname string) error {
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

	// 4. Iterate through each finalized work unit and enqueue notification
	// tasks.
	testResultsByWorkUnit := make([]*pb.TestResultsNotification_TestResultsByWorkUnit, 0, len(task.WorkUnitIds))
	for _, wuID := range task.WorkUnitIds {
		// TODO(b/372537579): Implement test result querying for the work unit.
		// This will likely involve pagination to handle large numbers of test results.
		testResults := []*pb.TestResult{} // Placeholder

		testResultsByWorkUnit = append(testResultsByWorkUnit, &pb.TestResultsNotification_TestResultsByWorkUnit{
			WorkUnitName: pbutil.WorkUnitName(task.RootInvocationId, wuID),
			TestResults:  testResults,
		})
	}

	// TODO(b/372537579): Implement proper batching based on message size to
	// stay within Pub/Sub limits. For now, one notification for all work units.
	dedupKey := fmt.Sprintf("%s-%s", task.WorkUnitIds, time.Now().UTC().Format(time.RFC3339))
	notification := &pb.TestResultsNotification{
		TestResultsByWorkUnit: testResultsByWorkUnit,
		ResultdbHost:          resultDBHostname,
		Sources:               rootInv.Sources,
		DeduplicationKey:      fmt.Sprintf("%x", sha256.Sum256([]byte(dedupKey))),
	}

	// Enqueue the task to publish the notification.
	tasks.NotifyTestResults(ctx, notification, attrs)
	return nil
}
