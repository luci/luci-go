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

package pubsub

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/logging/gologger"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/common/tsmon"
	"go.chromium.org/luci/common/tsmon/distribution"
	"go.chromium.org/luci/common/tsmon/store"
	"go.chromium.org/luci/common/tsmon/target"
	"go.chromium.org/luci/common/tsmon/types"

	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
	"go.chromium.org/luci/server/router"
)

// Examples taken from https://cloud.google.com/pubsub/docs/push.
const (
	validBody = `{
		"deliveryAttempt": 5,
		"message": {
				"attributes": {
						"key": "value"
				},
				"data": "SGVsbG8gQ2xvdWQgUHViL1N1YiEgSGVyZSBpcyBteSBtZXNzYWdlIQ==",
				"messageId": "2070443601311540",
				"message_id": "2070443601311540",
				"orderingKey": "key",
				"publishTime": "2021-02-26T19:13:55.749Z",
				"publish_time": "2021-02-26T19:13:55.749Z"
		},
		"subscription": "projects/myproject/subscriptions/mysubscription"
	}`
)

func validBodyMinimal(payload []byte) string {
	return fmt.Sprintf(`{
		"message": {
				"data": "%s",
				"messageId": "2070443601311540",
				"message_id": "2070443601311540",
				"publishTime": "2021-02-26T19:13:55.749Z",
				"publish_time": "2021-02-26T19:13:55.749Z"
		},
		"subscription": "projects/myproject/subscriptions/mysubscription"
	}`, base64.StdEncoding.EncodeToString(payload))
}

func TestDispatcher(t *testing.T) {
	t.Parallel()

	ftt.Run("With dispatcher", t, func(t *ftt.Test) {
		ctx := context.Background()
		ctx = gologger.StdConfig.Use(ctx)
		ctx, _, _ = tsmon.WithFakes(ctx)
		tsmon.GetState(ctx).SetStore(store.NewInMemory(&target.Task{}))

		ctx = authtest.MockAuthConfig(ctx)
		authState := &authtest.FakeState{
			Identity: "anonymous:anonymous",
		}
		ctx = auth.WithState(ctx, authState)

		metric := func(m types.Metric, fieldVals ...any) any {
			return tsmon.GetState(ctx).Store().Get(ctx, m, fieldVals)
		}

		metricDist := func(m types.Metric, fieldVals ...any) (count int64) {
			val := metric(m, fieldVals...)
			if val != nil {
				assert.Loosely(t, val, should.HaveType[*distribution.Distribution])
				count = val.(*distribution.Distribution).Count()
			}
			return
		}

		d := &Dispatcher{}
		srv := router.New()

		call := func(path, body string) int {
			req := httptest.NewRequest("POST", path, strings.NewReader(body)).WithContext(ctx)
			rec := httptest.NewRecorder()
			srv.ServeHTTP(rec, req)
			return rec.Result().StatusCode
		}

		t.Run("Auth required", func(t *ftt.Test) {
			d.InstallPubSubRoutes(srv, "/pubsub")

			called := false
			d.RegisterHandler("somehandler", func(ctx context.Context, message Message) error {
				called = true
				return nil
			})
			assert.Loosely(t, call("/pubsub/somehandler", validBody), should.Equal(403))
			assert.Loosely(t, called, should.BeFalse)
			assert.Loosely(t, metric(callsCounter, "somehandler", "auth"), should.Equal(1))
		})
		t.Run("With auth", func(t *ftt.Test) {
			d.DisableAuth = true
			d.InstallPubSubRoutes(srv, "/pubsub")

			t.Run("Handler IDs", func(t *ftt.Test) {
				d.RegisterHandler("h1", func(ctx context.Context, message Message) error { return nil })
				d.RegisterHandler("h2", func(ctx context.Context, message Message) error { return nil })
				assert.Loosely(t, d.handlerIDs(), should.Match([]string{"h1", "h2"}))
			})

			t.Run("Works", func(t *ftt.Test) {
				called := false
				var message Message
				d.RegisterHandler("ok", func(ctx context.Context, m Message) error {
					called = true
					message = m
					return nil
				})
				t.Run("With maximal body", func(t *ftt.Test) {
					assert.Loosely(t, call("/pubsub/ok?param1=1&param2=2", validBody), should.Equal(200))
					assert.Loosely(t, called, should.BeTrue)

					assert.Loosely(t, message, should.Match(Message{
						Data:         []byte("Hello Cloud Pub/Sub! Here is my message!"),
						Subscription: "projects/myproject/subscriptions/mysubscription",
						MessageID:    "2070443601311540",
						PublishTime:  time.Date(2021, 2, 26, 19, 13, 55, 749000000, time.UTC),
						Attributes:   map[string]string{"key": "value"},
						Query: url.Values{
							"param1": []string{"1"},
							"param2": []string{"2"},
						},
					}))
					assert.Loosely(t, metric(callsCounter, "ok", "OK"), should.Equal(1))
					assert.Loosely(t, metricDist(callsDurationMS, "ok", "OK"), should.Equal(1))
				})
				t.Run("With minimal body", func(t *ftt.Test) {
					payload := []byte("Hello Cloud Pub/Sub! Here is my message!")
					assert.Loosely(t, call("/pubsub/ok", validBodyMinimal(payload)), should.Equal(200))
					assert.Loosely(t, called, should.BeTrue)
					assert.Loosely(t, message, should.Match(Message{
						Data:         payload,
						Subscription: "projects/myproject/subscriptions/mysubscription",
						MessageID:    "2070443601311540",
						PublishTime:  time.Date(2021, 2, 26, 19, 13, 55, 749000000, time.UTC),
						Attributes:   map[string]string{},
						Query:        url.Values{},
					}))
					assert.Loosely(t, metric(callsCounter, "ok", "OK"), should.Equal(1))
					assert.Loosely(t, metricDist(callsDurationMS, "ok", "OK"), should.Equal(1))
				})
				t.Run("JSONPB handler", func(t *ftt.Test) {
					original := timestamppb.New(time.Date(2044, time.April, 4, 4, 4, 4, 4, time.UTC))
					payload, err := protojson.Marshal(original)
					assert.Loosely(t, err, should.BeNil)
					var called *timestamppb.Timestamp
					d.RegisterHandler("json", JSONPB(func(ctx context.Context, m Message, pb *timestamppb.Timestamp) error {
						called = pb
						return nil
					}))
					assert.Loosely(t, call("/pubsub/json", validBodyMinimal(payload)), should.Equal(200))
					assert.Loosely(t, called, should.Match(original))
				})
				t.Run("WirePB handler", func(t *ftt.Test) {
					original := timestamppb.New(time.Date(2044, time.April, 4, 4, 4, 4, 4, time.UTC))
					payload, err := proto.Marshal(original)
					assert.Loosely(t, err, should.BeNil)
					var called *timestamppb.Timestamp
					d.RegisterHandler("wire", WirePB(func(ctx context.Context, m Message, pb *timestamppb.Timestamp) error {
						called = pb
						return nil
					}))
					assert.Loosely(t, call("/pubsub/wire", validBodyMinimal(payload)), should.Equal(200))
					assert.Loosely(t, called, should.Match(original))
				})
			})

			t.Run("Use of unwrapped messages", func(t *ftt.Test) {
				d.RegisterHandler("unwrapped", func(ctx context.Context, message Message) error {
					panic("Should never be reached")
				})
				// Unwrapped messages are not supported.
				invalidBody := `{"field":"This is an unwrapped push subscription message."}`
				assert.Loosely(t, call("/pubsub/unwrapped", invalidBody), should.Equal(202))
				assert.Loosely(t, metric(callsCounter, "unwrapped", "fatal"), should.Equal(1))
				assert.Loosely(t, metricDist(callsDurationMS, "unwrapped", "fatal"), should.Equal(1))
			})

			t.Run("Invalid message", func(t *ftt.Test) {
				d.RegisterHandler("invalidcontent", func(ctx context.Context, message Message) error {
					panic("Should never be reached")
				})
				// Cloud Pub/Sub sends an invalid message.
				invalidBody := `// Invalid contents`
				assert.Loosely(t, call("/pubsub/invalidcontent", invalidBody), should.Equal(202))
				assert.Loosely(t, metric(callsCounter, "invalidcontent", "fatal"), should.Equal(1))
				assert.Loosely(t, metricDist(callsDurationMS, "invalidcontent", "fatal"), should.Equal(1))
			})

			t.Run("Fatal error in handler", func(t *ftt.Test) {
				d.RegisterHandler("boom", func(ctx context.Context, message Message) error {
					return errors.New("boom")
				})
				assert.Loosely(t, call("/pubsub/boom", validBody), should.Equal(202))
				assert.Loosely(t, metric(callsCounter, "boom", "fatal"), should.Equal(1))
				assert.Loosely(t, metricDist(callsDurationMS, "boom", "fatal"), should.Equal(1))
			})

			t.Run("Transient error in handler", func(t *ftt.Test) {
				d.RegisterHandler("smaller-boom", func(ctx context.Context, message Message) error {
					return transient.Tag.Apply(errors.New("smaller boom"))
				})
				assert.Loosely(t, call("/pubsub/smaller-boom", validBody), should.Equal(500))
				assert.Loosely(t, metric(callsCounter, "smaller-boom", "transient"), should.Equal(1))
				assert.Loosely(t, metricDist(callsDurationMS, "smaller-boom", "transient"), should.Equal(1))
			})

			t.Run("Message ignored in handler", func(t *ftt.Test) {
				d.RegisterHandler("ignore-all", func(ctx context.Context, message Message) error {
					return Ignore.Apply(errors.New("message ignored"))
				})
				assert.Loosely(t, call("/pubsub/ignore-all", validBody), should.Equal(204))
				assert.Loosely(t, metric(callsCounter, "ignore-all", "ignore"), should.Equal(1))
				assert.Loosely(t, metricDist(callsDurationMS, "ignore-all", "ignore"), should.Equal(1))
			})

			t.Run("Unknown handler", func(t *ftt.Test) {
				assert.Loosely(t, call("/pubsub/unknown", validBody), should.Equal(202))
				assert.Loosely(t, metric(callsCounter, "unknown", "no_handler"), should.Equal(1))
			})

			t.Run("Panic", func(t *ftt.Test) {
				d.RegisterHandler("panic", func(ctx context.Context, message Message) error {
					panic("boom")
				})
				assert.Loosely(t, func() { call("/pubsub/panic", validBody) }, should.Panic)
				assert.Loosely(t, metric(callsCounter, "panic", "panic"), should.Equal(1))
				assert.Loosely(t, metricDist(callsDurationMS, "panic", "panic"), should.Equal(1))
			})
		})
	})
}
