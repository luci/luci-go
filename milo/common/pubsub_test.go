// Copyright 2017 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package common

import (
	"errors"
	"fmt"
	"testing"

	"cloud.google.com/go/pubsub"
	"golang.org/x/net/context"

	"github.com/luci/gae/impl/memory"
	"github.com/luci/luci-go/common/logging/gologger"

	. "github.com/smartystreets/goconvey/convey"
)

type testPubSubClient struct {
	topics         map[string]error
	subscriptions  map[string]error
	createdSubsErr map[string]error
	createdSubs    map[string]pubsub.SubscriptionConfig
}

// Topic returns an empty pubsub topic reference.
func (client *testPubSubClient) getTopic(id string) (*pubsub.Topic, error) {
	if err, ok := client.topics[id]; ok {
		return &pubsub.Topic{}, err
	}
	panic(fmt.Errorf("test error: unknown topic %s", id))
}

// Subscription returns an empty subscription reference.
func (client *testPubSubClient) getSubscription(id string) (
	*pubsub.Subscription, error) {
	if err, ok := client.subscriptions[id]; ok {
		return &pubsub.Subscription{}, err
	}
	panic(fmt.Errorf("test error: unknown sub %s", id))
}

// CreateSubscription records that an attempt to create a subscription with
// an id, then returns an empty subscription.
func (client *testPubSubClient) createSubscription(
	id string, cfg pubsub.SubscriptionConfig) (
	*pubsub.Subscription, error) {

	if err, ok := client.createdSubsErr[id]; ok {
		client.createdSubs[id] = cfg
		return &pubsub.Subscription{}, err
	}
	panic(fmt.Errorf("test error: unknown created sub %s", id))
}

func TestPubSub(t *testing.T) {
	t.Parallel()

	Convey("Test Environment", t, func() {
		c := memory.UseWithAppID(context.Background(), "dev~luci-milo")
		c = gologger.StdConfig.Use(c)
		client := &testPubSubClient{
			topics:         map[string]error{},
			subscriptions:  map[string]error{},
			createdSubsErr: map[string]error{},
			createdSubs:    map[string]pubsub.SubscriptionConfig{}}
		c = context.WithValue(c, &pubSubClientKey, client)

		Convey("Buildbucket PubSub subscriber", func() {
			proj := "foo"
			Convey("Non-existant topic", func() {
				client.topics["builds"] = errNotExist
				err := ensureBuildbucketSubscribed(c, proj)
				So(err.Error(), ShouldEndWith, "does not exist")
			})
			Convey("Permission denied", func() {
				pErr := errors.New(
					"something PermissionDenied something")
				client.topics["builds"] = pErr
				err := ensureBuildbucketSubscribed(c, proj)
				So(err, ShouldEqual, pErr)
			})
			Convey("Normal error", func() {
				pErr := errors.New("foobar")
				client.topics["builds"] = pErr
				err := ensureBuildbucketSubscribed(c, proj)
				So(err, ShouldEqual, pErr)
			})
			client.topics["builds"] = nil
			Convey("Subscription exists", func() {
				client.subscriptions["luci-milo"] = nil
				err := ensureBuildbucketSubscribed(c, proj)
				So(err, ShouldBeNil)
				So(len(client.createdSubs), ShouldEqual, 0)
			})
			client.subscriptions["luci-milo"] = errNotExist
			Convey("Not registered", func() {
				errNotReg := errors.New("The supplied HTTP URL is not registered")
				client.createdSubsErr["luci-milo"] = errNotReg
				err := ensureBuildbucketSubscribed(c, proj)
				So(err, ShouldEqual, errNotReg)
			})
			Convey("Create subscription", func() {
				client.createdSubsErr["luci-milo"] = nil
				err := ensureBuildbucketSubscribed(c, proj)
				So(err, ShouldBeNil)
				So(len(client.createdSubs), ShouldEqual, 1)
				_, ok := client.createdSubs["luci-milo"]
				So(ok, ShouldEqual, true)
			})
		})
	})

}
