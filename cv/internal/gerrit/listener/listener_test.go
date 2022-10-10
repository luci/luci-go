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

package listener

import (
	"context"
	"errors"
	"testing"
	"time"

	"cloud.google.com/go/pubsub"
	"go.chromium.org/luci/cv/internal/cvtesting"
	listenerpb "go.chromium.org/luci/cv/settings/listener"

	. "github.com/smartystreets/goconvey/convey"
)

func TestGerritListener(t *testing.T) {
	t.Parallel()

	Convey("Listener", t, func() {
		ct := cvtesting.Test{}
		ctx, cancel := ct.SetUp()
		defer cancel()
		client, closeFn := mockPubSub(ctx)
		defer closeFn()
		topic := mockTopicSub(ctx, client, "a.example.org", "a.example.org")
		_ = mockTopicSub(ctx, client, "b.example.org", "b.example.org")
		sch := &testScheduler{}
		l := NewListener(client, sch)

		Convey("Run stops if context cancelled", func() {
			ctx, cancel = context.WithCancel(ctx)
			defer cancel()
			gSettings := []*listenerpb.Settings_GerritSubscription{{Host: "a.example.org"}}
			So(l.reload(ctx, gSettings), ShouldBeNil)

			// override the proc with a custom handler to ensure that
			// the listener is fully loaded and functioning.
			startC := make(chan struct{})
			l.sbers["a.example.org"].proc = &testProcessor{
				handler: func(_ context.Context, m *pubsub.Message) {
					close(startC)
					m.Ack()
				},
			}
			// let it run and wait until it serves the first pubsub msg.
			endC := make(chan struct{})
			go func() {
				l.Run(ctx)
				close(endC)
			}()
			_, err := topic.Publish(ctx, &pubsub.Message{}).Get(ctx)
			So(err, ShouldBeNil)
			select {
			case <-startC:
			case <-time.After(10 * time.Second):
				panic(errors.New("listener didn't start in 10s"))
			}
			// cancel the context and wait until it terminates.
			cancel()
			select {
			case <-endC:
			case <-time.After(10 * time.Second):
				panic(errors.New("listener didnt't end in 10s"))
			}
			// It's all good.
		})

		Convey("reload", func() {
			gSettings := []*listenerpb.Settings_GerritSubscription{
				{Host: "a.example.org"},
				{Host: "b.example.org"},
			}
			So(l.sbers, ShouldHaveLength, 0)
			So(l.reload(ctx, gSettings), ShouldBeNil)
			So(l.sbers, ShouldHaveLength, 2)

			So(l.sbers["a.example.org"].sub.ID(), ShouldEqual, "a.example.org")
			So(l.sbers["b.example.org"].sub.ID(), ShouldEqual, "b.example.org")

			Convey("adds subscribers for new subscriptions", func() {
				mockTopicSub(ctx, client, "c.example.org", "c.example.org")
				gSettings = append(gSettings, &listenerpb.Settings_GerritSubscription{
					Host: "c.example.org",
				})
				So(l.reload(ctx, gSettings), ShouldBeNil)
				So(l.sbers, ShouldHaveLength, 3)
				So(l.sbers["c.example.org"].sub.ID(), ShouldEqual, "c.example.org")
			})

			Convey("removes subscribers for removed subscriptions", func() {
				gSettings = gSettings[0 : len(gSettings)-1]
				So(l.reload(ctx, gSettings), ShouldBeNil)
				So(l.sbers, ShouldHaveLength, 1)
				So(l.sbers, ShouldNotContainKey, "b.example.org")
			})

			Convey("reload subscribers with new settings", func() {
				want := defaultNumGoroutines + 1
				gSettings[1].ReceiveSettings = &listenerpb.Settings_ReceiveSettings{
					NumGoroutines: uint64(want),
				}
				So(l.reload(ctx, gSettings), ShouldBeNil)
				So(l.sbers["b.example.org"].sub.ReceiveSettings.NumGoroutines, ShouldEqual, want)
			})
		})

	})
}
