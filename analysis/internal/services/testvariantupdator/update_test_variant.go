// Copyright 2022 The LUCI Authors.
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

package testvariantupdator

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"time"

	"cloud.google.com/go/spanner"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/durationpb"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/resultdb/pbutil"
	"go.chromium.org/luci/server/span"
	"go.chromium.org/luci/server/tq"
	_ "go.chromium.org/luci/server/tq/txn/spanner"

	"go.chromium.org/luci/analysis/internal/analyzedtestvariants"
	"go.chromium.org/luci/analysis/internal/config"
	spanutil "go.chromium.org/luci/analysis/internal/span"
	"go.chromium.org/luci/analysis/internal/tasks/taskspb"
	"go.chromium.org/luci/analysis/internal/verdicts"
	atvpb "go.chromium.org/luci/analysis/proto/analyzedtestvariant"
	configpb "go.chromium.org/luci/analysis/proto/config"
)

const (
	taskClass = "update-test-variant"
	queue     = "update-test-variant"
)

// errDoesNotExist returned if the AnalyzedTestVariant spanner row
// does not exist or does not have a timestamp.
// This could be because it was deleted due to a TTL being applied.
var errDoesNotExist = fmt.Errorf("test variant analysis row does not exist")

// errUnknownTask is returned if the task has a mismatched timestamp.
var errUnknownTask = fmt.Errorf("the task is unknown")

// RegisterTaskClass registers the task class for tq dispatcher.
func RegisterTaskClass() {
	tq.RegisterTaskClass(tq.TaskClass{
		ID:        taskClass,
		Prototype: &taskspb.UpdateTestVariant{},
		Queue:     queue,
		Kind:      tq.Transactional,
		Handler: func(ctx context.Context, payload proto.Message) error {
			task := payload.(*taskspb.UpdateTestVariant)
			tvKey := task.TestVariantKey
			_, err := checkTask(span.Single(ctx), task)
			switch {
			case err == errDoesNotExist:
				// Ignore the task.
				logging.Infof(ctx, "test variant %s/%s/%s deleted", tvKey.Realm, tvKey.TestId, tvKey.VariantHash)
				return nil
			case err == errUnknownTask:
				// Ignore the task.
				logging.Errorf(ctx, "unknown task found for test variant %s/%s/%s", tvKey.Realm, tvKey.TestId, tvKey.VariantHash)
				return nil
			case err != nil:
				return err
			}

			err = updateTestVariant(ctx, task)
			if err != nil {
				// If the error is transient, return err to retry.
				if transient.Tag.In(err) {
					return err
				}
				// If the error is not transient, just return nil. The task
				// will not be retried.
				logging.Errorf(ctx, "updateTestVariant error: %v", err.Error())
			}
			return nil
		},
	})
}

// Schedule enqueues a task to update an AnalyzedTestVariant row.
func Schedule(ctx context.Context, realm, testID, variantHash string, delay *durationpb.Duration, enqTime time.Time) {
	tq.MustAddTask(ctx, &tq.Task{
		Title: fmt.Sprintf("%s-%s-%s", realm, url.PathEscape(testID), variantHash),
		Payload: &taskspb.UpdateTestVariant{
			TestVariantKey: &taskspb.TestVariantKey{
				Realm:       realm,
				TestId:      testID,
				VariantHash: variantHash,
			},
			EnqueueTime: pbutil.MustTimestampProto(enqTime),
		},
		Delay: delay.AsDuration(),
	})
}

func configs(ctx context.Context, realm string) (*configpb.UpdateTestVariantTask, error) {
	rc, err := config.Realm(ctx, realm)
	switch {
	case err != nil:
		return nil, err
	case rc.GetTestVariantAnalysis().GetUpdateTestVariantTask() == nil:
		return nil, fmt.Errorf("no UpdateTestVariantTask config found for realm %s", realm)
	case rc.TestVariantAnalysis.UpdateTestVariantTask.GetUpdateTestVariantTaskInterval() == nil:
		return nil, fmt.Errorf("no GetUpdateTestVariantTaskInterval config found for realm %s", realm)
	case rc.TestVariantAnalysis.UpdateTestVariantTask.GetTestVariantStatusUpdateDuration() == nil:
		return nil, fmt.Errorf("no GetTestVariantStatusUpdateDuration config found for realm %s", realm)
	default:
		return rc.TestVariantAnalysis.UpdateTestVariantTask, nil
	}
}

// checkTask checks if the task has the same timestamp as the one saved in the
// row.
// Task has a mismatched timestamp will be ignored.
func checkTask(ctx context.Context, task *taskspb.UpdateTestVariant) (*analyzedtestvariants.StatusHistory, error) {
	statusHistory, enqTime, err := analyzedtestvariants.ReadStatusHistory(ctx, toSpannerKey(task.TestVariantKey))
	switch {
	case err != nil:
		return &analyzedtestvariants.StatusHistory{}, err
	case enqTime.IsNull():
		return statusHistory, errDoesNotExist
	case enqTime.Time != pbutil.MustTimestamp(task.EnqueueTime):
		return statusHistory, errUnknownTask
	default:
		return statusHistory, nil
	}
}

func updateTestVariant(ctx context.Context, task *taskspb.UpdateTestVariant) error {
	rc, err := configs(ctx, task.TestVariantKey.Realm)

	// We don't want to retry RealmNotExistsErr.
	if errors.Is(err, config.RealmNotExistsErr) {
		return err
	}
	if err != nil {
		return transient.Tag.Apply(err)
	}
	status, err := verdicts.ComputeTestVariantStatusFromVerdicts(span.Single(ctx), task.TestVariantKey, rc.TestVariantStatusUpdateDuration)
	if err != nil {
		return transient.Tag.Apply(err)
	}
	err = updateTestVariantStatus(ctx, task, status)
	if err != nil {
		return transient.Tag.Apply(err)
	}
	return nil
}

// updateTestVariantStatus updates the Status and StatusUpdateTime of the
// AnalyzedTestVariants row if the provided status is different from the one
// in the row.
func updateTestVariantStatus(ctx context.Context, task *taskspb.UpdateTestVariant, newStatus atvpb.Status) error {
	tvKey := task.TestVariantKey
	_, err := span.ReadWriteTransaction(ctx, func(ctx context.Context) error {
		// Get the old status, and check the token once again.
		statusHistory, err := checkTask(ctx, task)
		if err != nil {
			return err
		}

		// Update the Spanner row.
		vals := map[string]any{
			"Realm":       tvKey.Realm,
			"TestId":      tvKey.TestId,
			"VariantHash": tvKey.VariantHash,
		}
		now := clock.Now(ctx)

		oldStatus := statusHistory.Status
		if oldStatus == newStatus {
			if newStatus == atvpb.Status_CONSISTENTLY_EXPECTED || newStatus == atvpb.Status_NO_NEW_RESULTS {
				// This should never happen. But it doesn't have a huge negative impact,
				// so just log an error and return immediately.
				logging.Errorf(ctx, "UpdateTestVariant task runs for a test variant without any new unexpected failures: %s/%s/%s", tvKey.Realm, tvKey.TestId, tvKey.VariantHash)
				return nil
			}
			vals["NextUpdateTaskEnqueueTime"] = now
		} else {
			vals["Status"] = newStatus

			if statusHistory.PreviousStatuses == nil {
				vals["PreviousStatuses"] = []atvpb.Status{oldStatus}
				vals["PreviousStatusUpdateTimes"] = []time.Time{statusHistory.StatusUpdateTime}
			} else {
				// "Prepend" the old status and update time so the slices are ordered
				// by status update time in descending order.
				// Currently all of the status update records are kept, because we don't
				// expect to update each test variant's status frequently.
				// In the future we could consider to remove the old records.
				vals["PreviousStatuses"] = append([]atvpb.Status{oldStatus}, statusHistory.PreviousStatuses...)
				vals["PreviousStatusUpdateTimes"] = append([]time.Time{statusHistory.StatusUpdateTime}, statusHistory.PreviousStatusUpdateTimes...)
			}

			vals["StatusUpdateTime"] = spanner.CommitTimestamp
			if newStatus != atvpb.Status_CONSISTENTLY_EXPECTED && newStatus != atvpb.Status_NO_NEW_RESULTS {
				// Only schedule the next UpdateTestVariant task if the test variant
				// still has unexpected failures.
				vals["NextUpdateTaskEnqueueTime"] = now
			}
		}
		span.BufferWrite(ctx, spanutil.UpdateMap("AnalyzedTestVariants", vals))

		// Enqueue the next task.
		if _, ok := vals["NextUpdateTaskEnqueueTime"]; ok {
			rc, err := configs(ctx, tvKey.Realm)
			switch {
			case err != nil:
				return err
			default:
				Schedule(ctx, tvKey.Realm, tvKey.TestId, tvKey.VariantHash, rc.UpdateTestVariantTaskInterval, now)
			}
		}
		return nil
	})
	return err
}

func toSpannerKey(tvKey *taskspb.TestVariantKey) spanner.Key {
	return spanner.Key{tvKey.Realm, tvKey.TestId, tvKey.VariantHash}
}
