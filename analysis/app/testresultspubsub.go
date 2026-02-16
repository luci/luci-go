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
	"go.chromium.org/luci/server/auth/realms"
	"go.chromium.org/luci/server/pubsub"

	"go.chromium.org/luci/analysis/internal/services/resultingester"
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
	orchestrator *resultingester.Orchestrator
}

// NewTestResultsPubSubHandler initialises a new TestResultsPubSubHandler.
func NewTestResultsPubSubHandler(orchestrator *resultingester.Orchestrator) *TestResultsPubSubHandler {
	return &TestResultsPubSubHandler{
		orchestrator: orchestrator,
	}
}

// Handle processes the test results pubsub message.
func (h *TestResultsPubSubHandler) Handle(ctx context.Context, message pubsub.Message, notification *rdbpb.TestResultsNotification) error {
	status := "unknown"
	defer func() {
		// Closure for late binding.
		testResultsNotificationCounter.Add(ctx, 1, status)
	}()

	// For reading the root invocation, use a ResultDB client acting as
	// the project of the root invocation.
	project, _ := realms.Split(notification.RootInvocationMetadata.Realm)
	fields := logging.Fields{
		"Project":          project,
		"RootInvocation":   notification.RootInvocationMetadata.RootInvocationId,
		"DeduplicationKey": notification.DeduplicationKey,
	}
	ctx = logging.SetFields(ctx, fields)

	if err := h.orchestrator.HandlePubSub(ctx, notification); err != nil {
		status = errStatus(err)
		return err
	}

	status = "success"
	return nil
}
