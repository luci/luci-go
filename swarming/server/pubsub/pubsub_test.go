// Copyright 2023 The LUCI Authors.
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
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
	"go.chromium.org/luci/server/router"
)

func TestPubSubHandler(t *testing.T) {
	t.Parallel()

	ftt.Run("With mocks", t, func(t *ftt.Test) {
		const expectedPusherID = "user:push@example.com"
		var (
			testTime = time.Unix(1689000000, 0)
			testMsg  = &timestamppb.Timestamp{Seconds: 123456}
		)

		var body pushRequestBody
		body.Subscription = "sub"
		body.Message.Attributes = map[string]string{"a1": "v1", "a2": "v2"}
		body.Message.MessageID = "msg"
		body.Message.PublishTime = testTime
		body.Message.Data, _ = proto.Marshal(testMsg)

		goodBlob, _ := json.Marshal(&body)

		ctx := auth.WithState(context.Background(),
			&authtest.FakeState{
				Identity: expectedPusherID,
			},
		)

		call := func(
			ctx context.Context,
			path string,
			body []byte,
			cb func(ctx context.Context, msg *timestamppb.Timestamp, md *Metadata) error,
		) (statusCode int) {
			rr := httptest.NewRecorder()
			rc := &router.Context{
				Request: httptest.NewRequest("POST", path, bytes.NewReader(body)).WithContext(ctx),
				Writer:  rr,
			}
			handler(rc, expectedPusherID, cb)
			return rr.Code
		}

		t.Run("Success", func(t *ftt.Test) {
			var gotMsg *timestamppb.Timestamp
			var gotMD *Metadata
			resp := call(ctx, "/p?a=1&b=2", goodBlob, func(ctx context.Context, msg *timestamppb.Timestamp, md *Metadata) error {
				gotMsg = msg
				gotMD = md
				return nil
			})

			assert.Loosely(t, resp, should.Equal(http.StatusOK))
			assert.Loosely(t, gotMsg, should.Resemble(testMsg))
			assert.Loosely(t, gotMD.Subscription, should.Equal("sub"))
			assert.Loosely(t, gotMD.MessageID, should.Equal("msg"))
			assert.Loosely(t, gotMD.PublishTime, should.Equal(testTime))
			assert.Loosely(t, gotMD.Attributes, should.Resemble(body.Message.Attributes))
			assert.Loosely(t, gotMD.Query, should.Resemble(url.Values{"a": {"1"}, "b": {"2"}}))
		})

		t.Run("Transient error", func(t *ftt.Test) {
			resp := call(ctx, "/p", goodBlob, func(ctx context.Context, msg *timestamppb.Timestamp, md *Metadata) error {
				return errors.New("boo", transient.Tag)
			})
			assert.Loosely(t, resp, should.Equal(http.StatusInternalServerError))
		})

		t.Run("Fatal error", func(t *ftt.Test) {
			resp := call(ctx, "/p", goodBlob, func(ctx context.Context, msg *timestamppb.Timestamp, md *Metadata) error {
				return errors.New("boo")
			})
			assert.Loosely(t, resp, should.Equal(http.StatusAccepted))
		})

		t.Run("Wrong caller ID", func(t *ftt.Test) {
			ctx := auth.WithState(context.Background(),
				&authtest.FakeState{
					Identity: "user:wrong@example.com",
				},
			)
			resp := call(ctx, "/p", goodBlob, func(ctx context.Context, msg *timestamppb.Timestamp, md *Metadata) error {
				return nil
			})
			assert.Loosely(t, resp, should.Equal(http.StatusForbidden))
		})

		t.Run("Bad wrapper", func(t *ftt.Test) {
			resp := call(ctx, "/p", []byte("not json"), func(ctx context.Context, msg *timestamppb.Timestamp, md *Metadata) error {
				return nil
			})
			assert.Loosely(t, resp, should.Equal(http.StatusBadRequest))
		})

		t.Run("Bad payload", func(t *ftt.Test) {
			var body pushRequestBody
			body.Message.Data = []byte("bad proto")
			blob, _ := json.Marshal(&body)
			resp := call(ctx, "/p", blob, func(ctx context.Context, msg *timestamppb.Timestamp, md *Metadata) error {
				return nil
			})
			assert.Loosely(t, resp, should.Equal(http.StatusBadRequest))
		})
	})
}
