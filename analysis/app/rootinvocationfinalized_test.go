// Copyright 2025 The LUCI Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package app

import (
	"context"
	"fmt"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/common/tsmon"
	resultpb "go.chromium.org/luci/resultdb/proto/v1"
	"go.chromium.org/luci/server/pubsub"
	"go.chromium.org/luci/server/tq"

	"go.chromium.org/luci/analysis/internal/tasks/taskspb"
)

const (
	testRootInvocationID = "build-6363636363"
	testProject          = "rootinvproject"
	testRealm            = testProject + ":realm"
	testRDBHost          = "rdb-host"
)

func TestRootInvocationFinalizedHandler(t *testing.T) {
	ftt.Run(`Test RootInvocationFinalizedHandler`, t, func(t *ftt.Test) {
		ctx := context.Background()
		ctx, _ = tsmon.WithDummyInMemory(ctx)
		ctx, _ = testclock.UseTime(ctx, time.Date(2025, time.January, 1, 12, 0, 0, 0, time.UTC))
		ctx, taskScheduler := tq.TestingContext(ctx, nil)

		h := &RootInvocationFinalizedHandler{}

		t.Run(`Valid message`, func(t *ftt.Test) {
			called := false
			var processed bool

			createTime := timestamppb.New(time.Date(2025, time.January, 1, 12, 0, 0, 0, time.UTC))
			notification := &resultpb.RootInvocationFinalizedNotification{
				RootInvocation: &resultpb.RootInvocation{
					Name:             fmt.Sprintf("rootInvocations/%s", testRootInvocationID),
					Realm:            testRealm,
					RootInvocationId: testRootInvocationID,
					CreateTime:       createTime,
				},
				ResultdbHost: testRDBHost,
			}
			h.joinRootInvocation = func(ctx context.Context, notification *resultpb.RootInvocationFinalizedNotification) (bool, error) {
				assert.Loosely(t, called, should.BeFalse)
				assert.Loosely(t, notification, should.Match(&resultpb.RootInvocationFinalizedNotification{
					RootInvocation: notification.RootInvocation,
					ResultdbHost:   testRDBHost,
				}))
				called = true
				return processed, nil
			}

			// Implementation does not check properties of this parameter,
			// so we can leave it unset.
			message := pubsub.Message{}

			t.Run(`Processed`, func(t *ftt.Test) {
				processed = true

				err := h.Handle(ctx, message, notification)
				assert.NoErr(t, err)
				assert.Loosely(t, rootInvocationsFinalizedCounter.Get(ctx, testProject, "success"), should.Equal(1))
				assert.Loosely(t, called, should.BeTrue)
				// No task scheduled.
				assert.That(t, taskScheduler.Tasks().Payloads(), should.Match([]proto.Message{}))
			})
			t.Run(`Not processed`, func(t *ftt.Test) {
				processed = false

				err := h.Handle(ctx, message, notification)
				assert.That(t, err, should.ErrLike("ignoring root invocation finalized notification"))
				assert.Loosely(t, rootInvocationsFinalizedCounter.Get(ctx, testProject, "ignored"), should.Equal(1))
				assert.Loosely(t, called, should.BeTrue)
				// No task scheduled.
				assert.That(t, taskScheduler.Tasks().Payloads(), should.Match([]proto.Message{}))
			})
			t.Run(`android root invocation`, func(t *ftt.Test) {
				processed = true
				notification.RootInvocation.Realm = "android:test"

				err := h.Handle(ctx, message, notification)
				assert.NoErr(t, err)
				// Task scheduled.
				assert.Loosely(t, taskScheduler.Tasks().Payloads(), should.HaveLength(2))
				expectedWorkUnitIngestion := &taskspb.IngestWorkUnits{
					RootInvocation: fmt.Sprintf("rootInvocations/%s", testRootInvocationID),
					Realm:          "android:test",
					ResultdbHost:   testRDBHost,
					TaskIndex:      1,
				}
				expectedArtifactIngestion := &taskspb.IngestArtifacts{
					RootInvocationNotification: notification,
					TaskIndex:                  1,
				}
				assert.Loosely(t, taskScheduler.Tasks().Payloads(), should.Match([]proto.Message{
					expectedArtifactIngestion,
					expectedWorkUnitIngestion,
				}))
			})
		})
	})
}
