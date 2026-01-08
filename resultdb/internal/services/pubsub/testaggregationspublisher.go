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

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/server/span"
	"google.golang.org/protobuf/proto"

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

// testAggregationsPublisher is a helper struct for publishing test aggregations.
type testAggregationsPublisher struct {
	// task is the task payload.
	task *taskspb.PublishTestAggregationsTask

	// resultDBHostname is the hostname of the ResultDB service.
	resultDBHostname string
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

	// 1. Reads Root Invocation to check its state.
	rootInv, err := rootinvocations.Read(span.Single(ctx), rootInvID)
	if err != nil {
		return errors.Fmt("read root invocation %q: %w", rootInvID.Name(), err)
	}

	// 2. Checks StreamingExportState: Only publish if metadata is final.
	if rootInv.StreamingExportState != pb.RootInvocation_METADATA_FINAL {
		logging.Infof(ctx, "Root invocation %q is not ready for streaming export, skipping test aggregation notification", rootInvID.Name())
		return nil
	}

	// 3. Collects test aggregations.
	collectedAggregations, _, err := p.collectTestAggregations(ctx, rootInvID)
	if err != nil {
		return errors.Fmt("collect test aggregations: %w", err)
	}

	// 4. Constructs the message attributes.
	attrs := generateAttributes(rootInv)

	// 5. Publishes notifications.
	if err := p.publishNotifications(ctx, collectedAggregations, attrs); err != nil {
		return errors.Fmt("publish notifications: %w", err)
	}

	// TODO: b/454120520 - Implement checkpoints and continuous tasks.
	return nil
}

// collectTestAggregations gathers test aggregations across levels and pages up
// to a total size limit.
func (p *testAggregationsPublisher) collectTestAggregations(ctx context.Context, rootInvID rootinvocations.ID) (collectedAggregations []*pb.TestAggregationsNotification, nextToken *AggregationPageToken, err error) {
	ctx, s := tracing.Start(ctx, "go.chromium.org/luci/resultdb/internal/services/pubsub.collectTestAggregations")
	defer func() { tracing.End(s, err) }()

	levels := []pb.AggregationLevel{
		pb.AggregationLevel_INVOCATION,
		pb.AggregationLevel_MODULE,
		pb.AggregationLevel_COARSE,
		pb.AggregationLevel_FINE,
	}

	// TODO(zhihuixie): Support continuation token.
	startLevelIndex := 0
	startPageToken := ""

	currentSize := 0
	currentCount := 0
	for i := startLevelIndex; i < len(levels); i++ {
		level := levels[i]
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
			if currentCount+len(batch) > maxTestAggregationsCount || currentSize+batchSize > maxPubSubMessageSize {
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
