// Copyright 2017 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package common

import (
	"fmt"
	"testing"

	"cloud.google.com/go/pubsub"
	"golang.org/x/net/context"

	"github.com/luci/gae/impl/memory"
	"github.com/luci/luci-go/common/errors"
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
func (client *testPubSubClient) getTopic(c context.Context, id string) (*pubsub.Topic, error) {
	if err, ok := client.topics[id]; ok {
		return &pubsub.Topic{}, err
	}
	panic(fmt.Errorf("test error: unknown topic %s", id))
}

// Subscription returns an empty subscription reference.
func (client *testPubSubClient) getSubscription(c context.Context, id string) (
	*pubsub.Subscription, error) {
	if err, ok := client.subscriptions[id]; ok {
		return &pubsub.Subscription{}, err
	}
	panic(fmt.Errorf("test error: unknown sub %s", id))
}

// CreateSubscription records that an attempt to create a subscription with
// an id, then returns an empty subscription.
func (client *testPubSubClient) createSubscription(
	c context.Context, id string, cfg pubsub.SubscriptionConfig) (
	*pubsub.Subscription, error) {

	if err, ok := client.createdSubsErr[id]; ok {
		client.createdSubs[id] = cfg
		return &pubsub.Subscription{}, err
	}
	panic(fmt.Errorf("test error: unknown created sub %s", id))
}

type testFactory struct {
	clients map[string]pubsubClient
}

// makeTestClientFactory returns a closed pubsubClientFactory.
// Golang Protip: A bound method will not match the function type signature
// of an unbound function, but a closed function will.
func (fac *testFactory) makeTestClientFactory() pubsubClientFactory {
	return func(c context.Context, projectID string) (pubsubClient, error) {
		if cli, ok := fac.clients[projectID]; ok {
			return cli, nil
		}
		return nil, fmt.Errorf("client for project %s does not exist", projectID)
	}
}

func TestPubSub(t *testing.T) {
	t.Parallel()

	Convey("Test Environment", t, func() {
		c := memory.UseWithAppID(context.Background(), "dev~luci-milo")
		c = gologger.StdConfig.Use(c)
		miloClient := &testPubSubClient{
			topics:         map[string]error{},
			subscriptions:  map[string]error{},
			createdSubsErr: map[string]error{},
			createdSubs:    map[string]pubsub.SubscriptionConfig{}}
		bbClient := &testPubSubClient{
			topics:         map[string]error{},
			subscriptions:  map[string]error{},
			createdSubsErr: map[string]error{},
			createdSubs:    map[string]pubsub.SubscriptionConfig{}}
		fac := testFactory{
			clients: map[string]pubsubClient{
				"luci-milo":   miloClient,
				"buildbucket": bbClient,
			},
		}
		c = context.WithValue(c, &pubsubClientFactoryKey, fac.makeTestClientFactory())

		Convey("Buildbucket PubSub subscriber", func() {
			proj := "buildbucket"
			Convey("Non-existant topic", func() {
				bbClient.topics["builds"] = errNotExist
				err := ensureBuildbucketSubscribed(c, proj)
				So(err.Error(), ShouldEndWith, "does not exist")
			})
			Convey("Permission denied", func() {
				pErr := errors.New(
					"something PermissionDenied something")
				bbClient.topics["builds"] = pErr
				err := ensureBuildbucketSubscribed(c, proj)
				So(err, ShouldEqual, pErr)
			})
			Convey("Normal error", func() {
				pErr := errors.New("foobar")
				bbClient.topics["builds"] = pErr
				err := ensureBuildbucketSubscribed(c, proj)
				So(err, ShouldEqual, pErr)
			})
			bbClient.topics["builds"] = nil
			Convey("Subscription exists", func() {
				miloClient.subscriptions["buildbucket"] = nil
				err := ensureBuildbucketSubscribed(c, proj)
				So(err, ShouldBeNil)
				So(len(miloClient.createdSubs), ShouldEqual, 0)
				So(len(bbClient.createdSubs), ShouldEqual, 0)
			})
			miloClient.subscriptions["buildbucket"] = errNotExist
			Convey("Not registered", func() {
				errNotReg := errors.New("The supplied HTTP URL is not registered")
				miloClient.createdSubsErr["buildbucket"] = errNotReg
				err := ensureBuildbucketSubscribed(c, proj)
				So((err.(errors.Wrapped)).InnerError(), ShouldEqual, errNotReg)
			})
			Convey("Create subscription", func() {
				miloClient.createdSubsErr["buildbucket"] = nil
				err := ensureBuildbucketSubscribed(c, proj)
				So(err, ShouldBeNil)
				So(len(miloClient.createdSubs), ShouldEqual, 1)
				_, ok := miloClient.createdSubs["buildbucket"]
				So(ok, ShouldEqual, true)
			})
		})
	})

}
