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

package app

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"google.golang.org/protobuf/encoding/protojson"

	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/common/tsmon"
	resultpb "go.chromium.org/luci/resultdb/proto/v1"
	"go.chromium.org/luci/server/pubsub"
	"go.chromium.org/luci/server/router"

	_ "go.chromium.org/luci/analysis/internal/services/verdictingester" // Needed to ensure task class is registered.

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestInvocationFinalizedHandler(t *testing.T) {
	ftt.Run(`Test InvocationFinalizedHandler`, t, func(t *ftt.Test) {
		ctx, _ := tsmon.WithDummyInMemory(context.Background())

		h := &InvocationFinalizedHandler{}

		t.Run(`Valid message`, func(t *ftt.Test) {
			called := false
			var processed bool
			h.handleInvocation = func(ctx context.Context, notification *resultpb.InvocationFinalizedNotification) (bool, error) {
				assert.Loosely(t, called, should.BeFalse)
				assert.Loosely(t, notification, should.Match(&resultpb.InvocationFinalizedNotification{
					Invocation: "invocations/build-6363636363",
					Realm:      "invproject:realm",
				}))
				called = true
				return processed, nil
			}
			// Process invocation finalization.
			request := makeInvocationFinalizedReq(6363636363, "invproject:realm")

			t.Run(`Processed`, func(t *ftt.Test) {
				processed = true

				err := h.Handle(ctx, request)
				assert.That(t, err, should.ErrLike(nil))
				assert.Loosely(t, invocationsFinalizedCounter.Get(ctx, "invproject", "success"), should.Equal(1))
				assert.Loosely(t, called, should.BeTrue)
			})
			t.Run(`Not processed`, func(t *ftt.Test) {
				processed = false

				err := h.Handle(ctx, request)
				assert.That(t, err, should.ErrLike("ignoring invocation finalized notification"))
				assert.Loosely(t, invocationsFinalizedCounter.Get(ctx, "invproject", "ignored"), should.Equal(1))
				assert.Loosely(t, called, should.BeTrue)
			})
		})
		t.Run(`Invalid message`, func(t *ftt.Test) {
			h.handleInvocation = func(ctx context.Context, notification *resultpb.InvocationFinalizedNotification) (bool, error) {
				panic("Should not be reached.")
			}

			message := pubsub.Message{Data: []byte("Hello")}
			err := h.Handle(ctx, message)
			assert.That(t, err, should.ErrLike("parsing pubsub message data"))
			assert.That(t, transient.Tag.In(err), should.BeFalse)
		})
	})
}

func TestInvocationFinalizedHandlerLegacy(t *testing.T) {
	Convey(`Test InvocationFinalizedHandler`, t, func() {
		ctx, _ := tsmon.WithDummyInMemory(context.Background())

		h := &InvocationFinalizedHandler{}
		rsp := httptest.NewRecorder()
		rctx := &router.Context{
			Writer: rsp,
		}

		Convey(`Valid message`, func() {
			called := false
			var processed bool
			h.handleInvocation = func(ctx context.Context, notification *resultpb.InvocationFinalizedNotification) (bool, error) {
				So(called, ShouldBeFalse)
				So(notification, ShouldResembleProto, &resultpb.InvocationFinalizedNotification{
					Invocation: "invocations/build-6363636363",
					Realm:      "invproject:realm",
				})
				called = true
				return processed, nil
			}
			// Process invocation finalization.
			rctx.Request = (&http.Request{Body: makeInvocationFinalizedReqLegacy(6363636363, "invproject:realm")}).WithContext(ctx)

			Convey(`Processed`, func() {
				processed = true

				h.HandleLegacy(rctx)
				So(rsp.Code, ShouldEqual, http.StatusOK)
				So(invocationsFinalizedCounter.Get(ctx, "invproject", "success"), ShouldEqual, 1)
				So(called, ShouldBeTrue)
			})
			Convey(`Not processed`, func() {
				processed = false

				h.HandleLegacy(rctx)
				So(rsp.Code, ShouldEqual, http.StatusNoContent)
				So(invocationsFinalizedCounter.Get(ctx, "invproject", "ignored"), ShouldEqual, 1)
				So(called, ShouldBeTrue)
			})
		})
		Convey(`Invalid message`, func() {
			h.handleInvocation = func(ctx context.Context, notification *resultpb.InvocationFinalizedNotification) (bool, error) {
				panic("Should not be reached.")
			}

			rctx.Request = (&http.Request{Body: makeReq([]byte("Hello"), nil)}).WithContext(ctx)

			h.HandleLegacy(rctx)
			So(rsp.Code, ShouldEqual, http.StatusAccepted)
			So(invocationsFinalizedCounter.Get(ctx, "unknown", "permanent-failure"), ShouldEqual, 1)
		})
	})
}

func makeInvocationFinalizedReq(buildID int64, realm string) pubsub.Message {
	blob, _ := protojson.Marshal(&resultpb.InvocationFinalizedNotification{
		Invocation: fmt.Sprintf("invocations/build-%v", buildID),
		Realm:      realm,
	})
	return pubsub.Message{Data: blob}
}

func makeInvocationFinalizedReqLegacy(buildID int64, realm string) io.ReadCloser {
	blob, _ := protojson.Marshal(&resultpb.InvocationFinalizedNotification{
		Invocation: fmt.Sprintf("invocations/build-%v", buildID),
		Realm:      realm,
	})
	return makeReq(blob, nil)
}
