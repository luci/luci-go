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

	listenerpb "go.chromium.org/luci/cv/settings/listener"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

type testProcessor struct {
	handler func(context.Context, *pubsub.Message) error
}

func (p *testProcessor) process(ctx context.Context, m *pubsub.Message) error {
	return p.handler(ctx, m)
}

func mockPubSub(ctx context.Context) (*pubsub.Client, func()) {
	srv := pstest.NewServer()
	client, err := pubsub.NewClient(ctx, "luci-change-verifier",
		option.WithEndpoint(srv.Addr),
		option.WithoutAuthentication(),
		option.WithGRPCDialOption(grpc.WithTransportCredentials(insecure.NewCredentials())),
	)
	So(err, ShouldBeNil)
	return client, func() {
		_ = client.Close()
		_ = srv.Close()
	}
}

func mockTopicSub(ctx context.Context, client *pubsub.Client, topicID, subID string) *pubsub.Topic {
	topic, err := client.CreateTopic(ctx, topicID)
	So(err, ShouldBeNil)
	_, err = client.CreateSubscription(ctx, subID, pubsub.SubscriptionConfig{Topic: topic})
	So(err, ShouldBeNil)
	return topic
}

func TestSubscriber(t *testing.T) {
	t.Parallel()

	Convey("Subscriber", t, func() {
		ctx := context.Background()
		client, closeFn := mockPubSub(ctx)
		defer closeFn()
		topic := mockTopicSub(ctx, client, "topic_1", "sub_id_1")
		ch := make(chan struct{})
		sber := &subscriber{
			sub: client.Subscription("sub_id_1"),
			proc: &testProcessor{handler: func(_ context.Context, m *pubsub.Message) error {
				close(ch)
				return nil
			}},
		}

		Convey("starts", func() {
			So(sber.isStopped(), ShouldBeTrue)
			So(sber.start(ctx), ShouldBeNil)
			So(sber.isStopped(), ShouldBeFalse)

			// publish a sample message and wait until it gets processed.
			_, err := topic.Publish(ctx, &pubsub.Message{}).Get(ctx)
			So(err, ShouldBeNil)
			select {
			case <-ch:
			case <-time.After(10 * time.Second):
				panic(errors.New("subscriber started but didn't process the message in 10s"))
			}
		})

		Convey("fails", func() {
			Convey("if subscription doesn't exist", func() {
				sber.sub = client.Subscription("sub_does_not_exist")
				So(sber.start(ctx), ShouldErrLike, "doesn't exist")
				So(sber.isStopped(), ShouldBeTrue)
			})

			Convey("if called multiple times simultaneously", func() {
				So(sber.start(ctx), ShouldBeNil)
				So(sber.start(ctx), ShouldErrLike,
					"cannot start again, while the subscriber is running")
			})
		})

		Convey("stops", func() {
			So(sber.isStopped(), ShouldBeTrue)
			sber.stop(ctx)
			So(sber.start(ctx), ShouldBeNil)
			So(sber.isStopped(), ShouldBeFalse)
			sber.stop(ctx)
			So(sber.isStopped(), ShouldBeTrue)
		})
	})
}

func TestSameReceiveSettings(t *testing.T) {
	t.Parallel()

	Convey("sameReceiveSettings", t, func() {
		ctx := context.Background()
		client, closeFn := mockPubSub(ctx)
		defer closeFn()
		cfgs := &listenerpb.Settings_ReceiveSettings{}
		sber := &subscriber{sub: client.Subscription("sub_id_1")}
		sber.sub.ReceiveSettings.NumGoroutines = defaultNumGoroutines
		sber.sub.ReceiveSettings.MaxOutstandingMessages = defaultMaxOutstandingMessages

		Convey("NumGoroutines", func() {
			Convey("if 0, should be the default", func() {
				cfgs.NumGoroutines = 0
				So(sber.sameReceiveSettings(ctx, cfgs), ShouldBeTrue)

			})
			Convey("if != 0, should be different", func() {
				cfgs.NumGoroutines = 1
				So(sber.sameReceiveSettings(ctx, cfgs), ShouldBeFalse)
			})
		})
		Convey("MaxOutstandingMessages", func() {
			Convey("if 0, should be the default", func() {
				cfgs.MaxOutstandingMessages = 0
				So(sber.sameReceiveSettings(ctx, cfgs), ShouldBeTrue)

			})
			Convey("if != 0, should be different", func() {
				cfgs.MaxOutstandingMessages = 1
				So(sber.sameReceiveSettings(ctx, cfgs), ShouldBeFalse)
			})
		})
	})

}
