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

package app

import (
	"context"

	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/tsmon/field"
	"go.chromium.org/luci/common/tsmon/metric"
	rdbpb "go.chromium.org/luci/resultdb/proto/v1"
	"go.chromium.org/luci/server/pubsub"
)

var (
	testResultsNotificationCounter = metric.NewCounter(
		"analysis/ingestion/pubsub/test_results",
		"The number of test results notifications received by LUCI Analysis from PubSub.",
		nil,
		// "success", "transient-failure", "permanent-failure" or "ignored".
		field.String("status"))
)

// TestResultsPubSubHandler accepts and processes ResultDB Test Results PubSub
// messages.
type TestResultsPubSubHandler struct {
}

// NewTestResultsPubSubHandler initialises a new TestResultsPubSubHandler.
func NewTestResultsPubSubHandler() *TestResultsPubSubHandler {
	return &TestResultsPubSubHandler{}
}

// Handle processes the test results pubsub message.
func (h *TestResultsPubSubHandler) Handle(ctx context.Context, message pubsub.Message, notification *rdbpb.TestResultsNotification) error {
	status := "unknown"
	defer func() {
		// Closure for late binding.
		testResultsNotificationCounter.Add(ctx, 1, status)
	}()

	// For now, we just log and acknowledge the message.
	// This is to verify the pubsub flow.
	status = "success"
	workUnitCounts := make(map[string]int, len(notification.GetTestResultsByWorkUnit()))
	for _, group := range notification.GetTestResultsByWorkUnit() {
		workUnitCounts[group.GetWorkUnitName()] = len(group.GetTestResults())
	}

	// TODO(b/454120970): Remove the logging once we verify the pubsub flow.
	logging.Infof(ctx, "Successfully received test results notification with results grouped by work unit: %v", workUnitCounts)
	return nil
}
