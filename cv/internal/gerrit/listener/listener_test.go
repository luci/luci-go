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
	"crypto/sha256"
	"errors"
	"fmt"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/config"
	"go.chromium.org/luci/cv/internal/configs/srvcfg"
	"go.chromium.org/luci/cv/internal/cvtesting"
	listenerpb "go.chromium.org/luci/cv/settings/listener"

	. "github.com/smartystreets/goconvey/convey"
)

func TestListener(t *testing.T) {
	t.Parallel()

	Convey("Listener", t, func() {
		ct := cvtesting.Test{}
		ctx := ct.SetUp(t)
		client, closeFn := mockPubSub(ctx)
		defer closeFn()
		_ = mockTopicSub(ctx, client, "a.example.org", "a.example.org")
		_ = mockTopicSub(ctx, client, "b.example.org", "b.example.org")
		sch := &testScheduler{}
		l := NewListener(client, sch)

		updateCfg := func(s *listenerpb.Settings) error {
			b, err := proto.MarshalOptions{Deterministic: true}.Marshal(s)
			if err != nil {
				panic(err)
			}
			sha := sha256.New()
			sha.Write(b)
			m := &config.Meta{ContentHash: fmt.Sprintf("%x", sha.Sum(nil))}
			return srvcfg.SetTestListenerConfig(ctx, s, m)
		}

		Convey("Run stops if context cancelled", func() {
			ctx, cancel := context.WithCancel(ctx)
			defer cancel()
			lCfg := &listenerpb.Settings{
				GerritSubscriptions: []*listenerpb.Settings_GerritSubscription{
					{Host: "a.example.org"},
				},
			}
			So(updateCfg(lCfg), ShouldBeNil)

			// launch and wait until the subscriber is up.
			endC := make(chan struct{})
			go func() {
				l.Run(ctx)
				close(endC)
			}()
			var sber *subscriber
			for i := 0; i < 100; i++ {
				sber = l.getSubscriber("a.example.org")
				if sber != nil && !sber.isStopped() {
					break
				}
				time.Sleep(100 * time.Millisecond)
			}
			So(sber, ShouldNotBeNil)
			So(sber.isStopped(), ShouldBeFalse)

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
			subCfgs := []*listenerpb.Settings_GerritSubscription{
				{Host: "a.example.org"},
				{Host: "b.example.org"},
			}
			So(l.sbers, ShouldHaveLength, 0)
			So(l.reload(ctx, &listenerpb.Settings{GerritSubscriptions: subCfgs}), ShouldBeNil)
			aSub, bSub := l.getSubscriber("a.example.org"), l.getSubscriber("b.example.org")
			So(aSub, ShouldNotBeNil)
			So(aSub.sub.ID(), ShouldEqual, "a.example.org")
			So(bSub, ShouldNotBeNil)
			So(bSub.sub.ID(), ShouldEqual, "b.example.org")

			Convey("adds subscribers for new subscriptions", func() {
				mockTopicSub(ctx, client, "c.example.org", "c.example.org")
				subCfgs = append(subCfgs, &listenerpb.Settings_GerritSubscription{
					Host: "c.example.org",
				})
				So(l.reload(ctx, &listenerpb.Settings{GerritSubscriptions: subCfgs}), ShouldBeNil)
				So(l.sbers, ShouldHaveLength, 3)
				So(l.getSubscriber("c.example.org").sub.ID(), ShouldEqual, "c.example.org")
			})

			Convey("removes subscribers for removed subscriptions", func() {
				subCfgs = subCfgs[0 : len(subCfgs)-1]
				So(l.reload(ctx, &listenerpb.Settings{GerritSubscriptions: subCfgs}), ShouldBeNil)
				So(l.sbers, ShouldHaveLength, 1)
				So(l.getSubscriber("b.example.org"), ShouldBeNil)
			})

			Convey("reload subscribers with new settings", func() {
				want := defaultNumGoroutines + 1
				subCfgs[1].ReceiveSettings = &listenerpb.Settings_ReceiveSettings{
					NumGoroutines: uint64(want),
				}
				So(l.reload(ctx, &listenerpb.Settings{GerritSubscriptions: subCfgs}), ShouldBeNil)
				bSub := l.getSubscriber("b.example.org")
				So(bSub, ShouldNotBeNil)
				So(bSub.sub.ReceiveSettings.NumGoroutines, ShouldEqual, want)
			})
		})

	})
}
