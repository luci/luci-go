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

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/config"

	"go.chromium.org/luci/cv/internal/configs/srvcfg"
	"go.chromium.org/luci/cv/internal/cvtesting"
	listenerpb "go.chromium.org/luci/cv/settings/listener"
)

func TestListener(t *testing.T) {
	t.Parallel()

	ftt.Run("Listener", t, func(t *ftt.Test) {
		ct := cvtesting.Test{}
		ctx := ct.SetUp(t)
		client, closeFn := mockPubSub(t, ctx)
		defer closeFn()
		_ = mockTopicSub(t, ctx, client, "a.example.org", "a.example.org")
		_ = mockTopicSub(t, ctx, client, "b.example.org", "b.example.org")
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

		t.Run("Run stops if context cancelled", func(t *ftt.Test) {
			ctx, cancel := context.WithCancel(ctx)
			defer cancel()
			lCfg := &listenerpb.Settings{
				GerritSubscriptions: []*listenerpb.Settings_GerritSubscription{
					{Host: "a.example.org"},
				},
			}
			assert.NoErr(t, updateCfg(lCfg))

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
			assert.Loosely(t, sber, should.NotBeNil)
			assert.Loosely(t, sber.isStopped(), should.BeFalse)

			// cancel the context and wait until it terminates.
			cancel()
			select {
			case <-endC:
			case <-time.After(10 * time.Second):
				panic(errors.New("listener didnt't end in 10s"))
			}
			// It's all good.
		})

		t.Run("reload", func(t *ftt.Test) {
			subCfgs := []*listenerpb.Settings_GerritSubscription{
				{Host: "a.example.org"},
				{Host: "b.example.org"},
			}
			assert.Loosely(t, l.sbers, should.HaveLength(0))
			assert.NoErr(t, l.reload(ctx, &listenerpb.Settings{GerritSubscriptions: subCfgs}))
			aSub, bSub := l.getSubscriber("a.example.org"), l.getSubscriber("b.example.org")
			assert.Loosely(t, aSub, should.NotBeNil)
			assert.Loosely(t, aSub.sub.ID(), should.Equal("a.example.org"))
			assert.Loosely(t, bSub, should.NotBeNil)
			assert.Loosely(t, bSub.sub.ID(), should.Equal("b.example.org"))

			t.Run("adds subscribers for new subscriptions", func(t *ftt.Test) {
				mockTopicSub(t, ctx, client, "c.example.org", "c.example.org")
				subCfgs = append(subCfgs, &listenerpb.Settings_GerritSubscription{
					Host: "c.example.org",
				})
				assert.NoErr(t, l.reload(ctx, &listenerpb.Settings{GerritSubscriptions: subCfgs}))
				assert.Loosely(t, l.sbers, should.HaveLength(3))
				assert.Loosely(t, l.getSubscriber("c.example.org").sub.ID(), should.Equal("c.example.org"))
			})

			t.Run("removes subscribers for removed subscriptions", func(t *ftt.Test) {
				subCfgs = subCfgs[0 : len(subCfgs)-1]
				assert.NoErr(t, l.reload(ctx, &listenerpb.Settings{GerritSubscriptions: subCfgs}))
				assert.Loosely(t, l.sbers, should.HaveLength(1))
				assert.Loosely(t, l.getSubscriber("b.example.org"), should.BeNil)
			})

			t.Run("reload subscribers with new settings", func(t *ftt.Test) {
				want := defaultNumGoroutines + 1
				subCfgs[1].ReceiveSettings = &listenerpb.Settings_ReceiveSettings{
					NumGoroutines: uint64(want),
				}
				assert.NoErr(t, l.reload(ctx, &listenerpb.Settings{GerritSubscriptions: subCfgs}))
				bSub := l.getSubscriber("b.example.org")
				assert.Loosely(t, bSub, should.NotBeNil)
				assert.Loosely(t, bSub.sub.ReceiveSettings.NumGoroutines, should.Equal(want))
			})
		})

	})
}
