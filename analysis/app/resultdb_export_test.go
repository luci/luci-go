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
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/common/tsmon"
	resultpb "go.chromium.org/luci/resultdb/proto/v1"
	"go.chromium.org/luci/server/pubsub"
	"go.chromium.org/luci/server/router"
	"go.chromium.org/luci/server/tq"

	"go.chromium.org/luci/analysis/internal/tasks/taskspb"

	_ "go.chromium.org/luci/analysis/internal/services/resultingester" // Needed to ensure task class is registered.

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
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

func TestInvocationReadyForExportHandlerLegacy(t *testing.T) {
	Convey(`Test InvocationReadyForExportHandler`, t, func() {
		ctx, _ := tsmon.WithDummyInMemory(context.Background())
		ctx, taskScheduler := tq.TestingContext(ctx, nil)

		h := &InvocationReadyForExportHandler{}
		rsp := httptest.NewRecorder()
		rctx := &router.Context{
			Writer: rsp,
		}

		Convey(`Valid message`, func() {
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
			rctx.Request = (&http.Request{Body: makeInvocationReadyForExportReqLegacy(notification)}).WithContext(ctx)

			h.HandleLegacy(rctx)
			So(rsp.Code, ShouldEqual, http.StatusOK)
			So(invocationsReadyForExportCounter.Get(ctx, "testproject", "success"), ShouldEqual, 1)
			So(taskScheduler.Tasks().Payloads(), ShouldResembleProto, []proto.Message{
				&taskspb.IngestTestResults{
					Notification: notification,
					TaskIndex:    1,
				},
			})
		})
		Convey(`Invalid message`, func() {
			rctx.Request = (&http.Request{Body: makeReq([]byte("Hello"), nil)}).WithContext(ctx)

			h.HandleLegacy(rctx)
			So(rsp.Code, ShouldEqual, http.StatusAccepted)
			So(invocationsReadyForExportCounter.Get(ctx, "unknown", "permanent-failure"), ShouldEqual, 1)
		})
	})
}

func makeInvocationReadyForExportReqLegacy(notification *resultpb.InvocationReadyForExportNotification) io.ReadCloser {
	blob, _ := protojson.Marshal(notification)
	return makeReq(blob, nil)
}
