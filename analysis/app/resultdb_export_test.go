// Copyright 2024 The LUCI Authors.
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
	"testing"

	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/common/tsmon"
	resultpb "go.chromium.org/luci/resultdb/proto/v1"
	"go.chromium.org/luci/server/pubsub"
	"go.chromium.org/luci/server/tq"

	"go.chromium.org/luci/analysis/internal/tasks/taskspb"

	_ "go.chromium.org/luci/analysis/internal/services/resultingester" // Needed to ensure task class is registered.
)

func TestInvocationReadyForExportHandler(t *testing.T) {
	ftt.Run(`Test InvocationReadyForExportHandler`, t, func(t *ftt.Test) {
		ctx, _ := tsmon.WithDummyInMemory(context.Background())
		ctx, taskScheduler := tq.TestingContext(ctx, nil)

		h := &InvocationReadyForExportHandler{}

		t.Run(`Valid message`, func(t *ftt.Test) {
			notification := &resultpb.InvocationReadyForExportNotification{
				RootInvocation:      "root-invocation",
				RootInvocationRealm: "testproject:ci",
				Invocation:          "my-invocation",
				InvocationRealm:     "includedproject:test_runner",
				Sources: &resultpb.Sources{
					GitilesCommit: &resultpb.GitilesCommit{
						Host:       "testproject.googlesource.com",
						Project:    "testproject/src",
						Ref:        "refs/heads/main",
						CommitHash: "1234567890123456789012345678901234567890",
						Position:   123,
					},
				},
			}
			// Process invocation finalization.
			err := h.Handle(ctx, pubsub.Message{}, notification)
			assert.That(t, err, should.ErrLike(nil))
			assert.Loosely(t, invocationsReadyForExportCounter.Get(ctx, "testproject", "success"), should.Equal(1))
			assert.That(t, taskScheduler.Tasks().Payloads(), should.Match([]proto.Message{
				&taskspb.IngestTestResults{
					Notification: notification,
					TaskIndex:    1,
				},
			}))
		})
	})
}
