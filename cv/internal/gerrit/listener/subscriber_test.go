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
	"cloud.google.com/go/pubsub/pstest"
	"google.golang.org/api/option"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	listenerpb "go.chromium.org/luci/cv/settings/listener"
)

type testProcessor struct {
	handler func(context.Context, *pubsub.Message) error
}

func (p *testProcessor) process(ctx context.Context, m *pubsub.Message) error {
	return p.handler(ctx, m)
}

func mockPubSub(t testing.TB, ctx context.Context) (*pubsub.Client, func()) {
	t.Helper()

	srv := pstest.NewServer()
	client, err := pubsub.NewClient(ctx, "luci-change-verifier",
		option.WithEndpoint(srv.Addr),
		option.WithoutAuthentication(),
		option.WithGRPCDialOption(grpc.WithTransportCredentials(insecure.NewCredentials())),
	)
	assert.Loosely(t, err, should.BeNil, truth.LineContext())
	return client, func() {
		_ = client.Close()
		_ = srv.Close()
	}
}

func mockTopicSub(t testing.TB, ctx context.Context, client *pubsub.Client, topicID, subID string) *pubsub.Topic {
	t.Helper()

	topic, err := client.CreateTopic(ctx, topicID)
	assert.Loosely(t, err, should.BeNil, truth.LineContext())
	_, err = client.CreateSubscription(ctx, subID, pubsub.SubscriptionConfig{Topic: topic})
	assert.Loosely(t, err, should.BeNil, truth.LineContext())
	return topic
}

func TestSubscriber(t *testing.T) {
	t.Parallel()

	ftt.Run("Subscriber", t, func(t *ftt.Test) {
		ctx := context.Background()
		client, closeFn := mockPubSub(t, ctx)
		defer closeFn()
		topic := mockTopicSub(t, ctx, client, "topic_1", "sub_id_1")
		ch := make(chan struct{})
		sber := &subscriber{
			sub: client.Subscription("sub_id_1"),
			proc: &testProcessor{handler: func(_ context.Context, m *pubsub.Message) error {
				close(ch)
				return nil
			}},
		}

		t.Run("starts", func(t *ftt.Test) {
			assert.Loosely(t, sber.isStopped(), should.BeTrue)
			assert.Loosely(t, sber.start(ctx), should.BeNil)
			assert.Loosely(t, sber.isStopped(), should.BeFalse)

			// publish a sample message and wait until it gets processed.
			_, err := topic.Publish(ctx, &pubsub.Message{}).Get(ctx)
			assert.Loosely(t, err, should.BeNil)
			select {
			case <-ch:
			case <-time.After(10 * time.Second):
				panic(errors.New("subscriber started but didn't process the message in 10s"))
			}
		})

		t.Run("fails", func(t *ftt.Test) {
			t.Run("if subscription doesn't exist", func(t *ftt.Test) {
				sber.sub = client.Subscription("sub_does_not_exist")
				assert.Loosely(t, sber.start(ctx), should.ErrLike("doesn't exist"))
				assert.Loosely(t, sber.isStopped(), should.BeTrue)
			})

			t.Run("if called multiple times simultaneously", func(t *ftt.Test) {
				assert.Loosely(t, sber.start(ctx), should.BeNil)
				assert.Loosely(t, sber.start(ctx), should.ErrLike(
					"cannot start again, while the subscriber is running"))
			})
		})

		t.Run("stops", func(t *ftt.Test) {
			assert.Loosely(t, sber.isStopped(), should.BeTrue)
			sber.stop(ctx)
			assert.Loosely(t, sber.start(ctx), should.BeNil)
			assert.Loosely(t, sber.isStopped(), should.BeFalse)
			sber.stop(ctx)
			assert.Loosely(t, sber.isStopped(), should.BeTrue)
		})
	})
}

func TestSameReceiveSettings(t *testing.T) {
	t.Parallel()

	ftt.Run("sameReceiveSettings", t, func(t *ftt.Test) {
		ctx := context.Background()
		client, closeFn := mockPubSub(t, ctx)
		defer closeFn()
		cfgs := &listenerpb.Settings_ReceiveSettings{}
		sber := &subscriber{sub: client.Subscription("sub_id_1")}
		sber.sub.ReceiveSettings.NumGoroutines = defaultNumGoroutines
		sber.sub.ReceiveSettings.MaxOutstandingMessages = defaultMaxOutstandingMessages

		t.Run("NumGoroutines", func(t *ftt.Test) {
			t.Run("if 0, should be the default", func(t *ftt.Test) {
				cfgs.NumGoroutines = 0
				assert.Loosely(t, sber.sameReceiveSettings(ctx, cfgs), should.BeTrue)

			})
			t.Run("if != 0, should be different", func(t *ftt.Test) {
				cfgs.NumGoroutines = 1
				assert.Loosely(t, sber.sameReceiveSettings(ctx, cfgs), should.BeFalse)
			})
		})
		t.Run("MaxOutstandingMessages", func(t *ftt.Test) {
			t.Run("if 0, should be the default", func(t *ftt.Test) {
				cfgs.MaxOutstandingMessages = 0
				assert.Loosely(t, sber.sameReceiveSettings(ctx, cfgs), should.BeTrue)

			})
			t.Run("if != 0, should be different", func(t *ftt.Test) {
				cfgs.MaxOutstandingMessages = 1
				assert.Loosely(t, sber.sameReceiveSettings(ctx, cfgs), should.BeFalse)
			})
		})
	})

}
