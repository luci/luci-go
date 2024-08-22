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
	"go.chromium.org/luci/common/tsmon"
	"go.chromium.org/luci/common/tsmon/distribution"
	"go.chromium.org/luci/common/tsmon/store"
	"go.chromium.org/luci/common/tsmon/target"
	"go.chromium.org/luci/common/tsmon/types"

	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
	"go.chromium.org/luci/server/router"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
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

	Convey("With dispatcher", t, func() {
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
			return tsmon.GetState(ctx).Store().Get(ctx, m, time.Time{}, fieldVals)
		}

		metricDist := func(m types.Metric, fieldVals ...any) (count int64) {
			val := metric(m, fieldVals...)
			if val != nil {
				So(val, ShouldHaveSameTypeAs, &distribution.Distribution{})
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

		Convey("Auth required", func() {
			d.InstallPubSubRoutes(srv, "/pubsub")

			called := false
			d.RegisterHandler("somehandler", func(ctx context.Context, message Message) error {
				called = true
				return nil
			})
			So(call("/pubsub/somehandler", validBody), ShouldEqual, 403)
			So(called, ShouldBeFalse)
			So(metric(callsCounter, "somehandler", "auth"), ShouldEqual, 1)
		})
		Convey("With auth", func() {
			d.DisableAuth = true
			d.InstallPubSubRoutes(srv, "/pubsub")

			Convey("Handler IDs", func() {
				d.RegisterHandler("h1", func(ctx context.Context, message Message) error { return nil })
				d.RegisterHandler("h2", func(ctx context.Context, message Message) error { return nil })
				So(d.handlerIDs(), ShouldResemble, []string{"h1", "h2"})
			})

			Convey("Works", func() {
				called := false
				var message Message
				d.RegisterHandler("ok", func(ctx context.Context, m Message) error {
					called = true
					message = m
					return nil
				})
				Convey("With maximal body", func() {
					So(call("/pubsub/ok?param1=1&param2=2", validBody), ShouldEqual, 200)
					So(called, ShouldBeTrue)

					So(message, ShouldResemble, Message{
						Data:         []byte("Hello Cloud Pub/Sub! Here is my message!"),
						Subscription: "projects/myproject/subscriptions/mysubscription",
						MessageID:    "2070443601311540",
						PublishTime:  time.Date(2021, 2, 26, 19, 13, 55, 749000000, time.UTC),
						Attributes:   map[string]string{"key": "value"},
						Query: url.Values{
							"param1": []string{"1"},
							"param2": []string{"2"},
						},
					})
					So(metric(callsCounter, "ok", "OK"), ShouldEqual, 1)
					So(metricDist(callsDurationMS, "ok", "OK"), ShouldEqual, 1)
				})
				Convey("With minimal body", func() {
					payload := []byte("Hello Cloud Pub/Sub! Here is my message!")
					So(call("/pubsub/ok", validBodyMinimal(payload)), ShouldEqual, 200)
					So(called, ShouldBeTrue)
					So(message, ShouldResemble, Message{
						Data:         payload,
						Subscription: "projects/myproject/subscriptions/mysubscription",
						MessageID:    "2070443601311540",
						PublishTime:  time.Date(2021, 2, 26, 19, 13, 55, 749000000, time.UTC),
						Attributes:   map[string]string{},
						Query:        url.Values{},
					})
					So(metric(callsCounter, "ok", "OK"), ShouldEqual, 1)
					So(metricDist(callsDurationMS, "ok", "OK"), ShouldEqual, 1)
				})
				Convey("JSONPB handler", func() {
					original := timestamppb.New(time.Date(2044, time.April, 4, 4, 4, 4, 4, time.UTC))
					payload, err := protojson.Marshal(original)
					So(err, ShouldBeNil)
					var called *timestamppb.Timestamp
					d.RegisterHandler("json", JSONPB(func(ctx context.Context, m Message, pb *timestamppb.Timestamp) error {
						called = pb
						return nil
					}))
					So(call("/pubsub/json", validBodyMinimal(payload)), ShouldEqual, 200)
					So(called, ShouldResembleProto, original)
				})
				Convey("WirePB handler", func() {
					original := timestamppb.New(time.Date(2044, time.April, 4, 4, 4, 4, 4, time.UTC))
					payload, err := proto.Marshal(original)
					So(err, ShouldBeNil)
					var called *timestamppb.Timestamp
					d.RegisterHandler("wire", WirePB(func(ctx context.Context, m Message, pb *timestamppb.Timestamp) error {
						called = pb
						return nil
					}))
					So(call("/pubsub/wire", validBodyMinimal(payload)), ShouldEqual, 200)
					So(called, ShouldResembleProto, original)
				})
			})

			Convey("Use of unwrapped messages", func() {
				d.RegisterHandler("unwrapped", func(ctx context.Context, message Message) error {
					panic("Should never be reached")
				})
				// Unwrapped messages are not supported.
				invalidBody := `{"field":"This is an unwrapped push subscription message."}`
				So(call("/pubsub/unwrapped", invalidBody), ShouldEqual, 202)
				So(metric(callsCounter, "unwrapped", "fatal"), ShouldEqual, 1)
				So(metricDist(callsDurationMS, "unwrapped", "fatal"), ShouldEqual, 1)
			})

			Convey("Invalid message", func() {
				d.RegisterHandler("invalidcontent", func(ctx context.Context, message Message) error {
					panic("Should never be reached")
				})
				// Cloud Pub/Sub sends an invalid message.
				invalidBody := `// Invalid contents`
				So(call("/pubsub/invalidcontent", invalidBody), ShouldEqual, 202)
				So(metric(callsCounter, "invalidcontent", "fatal"), ShouldEqual, 1)
				So(metricDist(callsDurationMS, "invalidcontent", "fatal"), ShouldEqual, 1)
			})

			Convey("Fatal error in handler", func() {
				d.RegisterHandler("boom", func(ctx context.Context, message Message) error {
					return errors.New("boom")
				})
				So(call("/pubsub/boom", validBody), ShouldEqual, 202)
				So(metric(callsCounter, "boom", "fatal"), ShouldEqual, 1)
				So(metricDist(callsDurationMS, "boom", "fatal"), ShouldEqual, 1)
			})

			Convey("Transient error in handler", func() {
				d.RegisterHandler("smaller-boom", func(ctx context.Context, message Message) error {
					return transient.Tag.Apply(errors.New("smaller boom"))
				})
				So(call("/pubsub/smaller-boom", validBody), ShouldEqual, 500)
				So(metric(callsCounter, "smaller-boom", "transient"), ShouldEqual, 1)
				So(metricDist(callsDurationMS, "smaller-boom", "transient"), ShouldEqual, 1)
			})

			Convey("Message ignored in handler", func() {
				d.RegisterHandler("ignore-all", func(ctx context.Context, message Message) error {
					return Ignore.Apply(errors.New("message ignored"))
				})
				So(call("/pubsub/ignore-all", validBody), ShouldEqual, 204)
				So(metric(callsCounter, "ignore-all", "ignore"), ShouldEqual, 1)
				So(metricDist(callsDurationMS, "ignore-all", "ignore"), ShouldEqual, 1)
			})

			Convey("Unknown handler", func() {
				So(call("/pubsub/unknown", validBody), ShouldEqual, 202)
				So(metric(callsCounter, "unknown", "no_handler"), ShouldEqual, 1)
			})

			Convey("Panic", func() {
				d.RegisterHandler("panic", func(ctx context.Context, message Message) error {
					panic("boom")
				})
				So(func() { call("/pubsub/panic", validBody) }, ShouldPanic)
				So(metric(callsCounter, "panic", "panic"), ShouldEqual, 1)
				So(metricDist(callsDurationMS, "panic", "panic"), ShouldEqual, 1)
			})
		})
	})
}
