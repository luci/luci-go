// Copyright 2017 The LUCI Authors.
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

	"cloud.google.com/go/pubsub"
	vkit "cloud.google.com/go/pubsub/apiv1"
	pb "cloud.google.com/go/pubsub/apiv1/pubsubpb"
	gax "github.com/googleapis/gax-go/v2"

	"go.chromium.org/luci/common/logging"
)

// Publisher is a generic interface to something that can publish Pub/Sub
// messages.
//
// A Publisher should be Closed when finished with it.
type Publisher interface {
	Publish(ctx context.Context, msgs ...*pubsub.Message) ([]string, error)
	Close() error
}

// ClientFactory is passed into an UnbufferedPublisher to create or reset a client.
type ClientFactory interface {
	// Client returns the Pub/Sub publisher client to use.
	// Client will be closed when this UnbufferedPublisher is closed.
	Client(context.Context) (*vkit.PublisherClient, error)

	// RecreateClient is called if any publish calls fail.
	// This is used to tell the underlying service to maybe generate a new client.
	RecreateClient()
}

// UnbufferedPublisher directly instantiates a Pub/Sub client and publishes a
// message to it.
//
// The standard Pub/Sub library has several issues, especially when used from
// AppEngine:
//   - It uses an empty Context, discarding AppEngine context.
//   - It uses a buffer, which expects a lifecycle beyond that of a simple
//     AppEngine Request.
type UnbufferedPublisher struct {
	// AECtx is the AppEngine context used to create a pubsub client.
	AECtx context.Context

	// Topic is the name of the Topic to publish to.
	Topic Topic

	// ClientFactory produces a client for the publisher.  This is called on each
	// and every publish request.  If a publish request fails, then RecreateClient is called.
	ClientFactory ClientFactory

	// CallOpts are arbitrary call options that will be passed to the Publish
	// call.
	CallOpts []gax.CallOption
}

var _ Publisher = (*UnbufferedPublisher)(nil)

// Publish publishes a message immediately, blocking until it completes.
//
// "c" must be an AppEngine context.
func (up *UnbufferedPublisher) Publish(ctx context.Context, msgs ...*pubsub.Message) ([]string, error) {
	if len(msgs) == 0 {
		return nil, nil
	}

	messages := make([]*pb.PubsubMessage, len(msgs))
	for i, msg := range msgs {
		messages[i] = &pb.PubsubMessage{
			Data:       msg.Data,
			Attributes: msg.Attributes,
		}
	}

	client, err := up.ClientFactory.Client(up.AECtx)
	if err != nil {
		return nil, err
	}

	resp, err := client.Publish(ctx, &pb.PublishRequest{
		Topic:    string(up.Topic),
		Messages: messages,
	}, up.CallOpts...)
	if err != nil {
		// Optimistically recreate the client.
		up.ClientFactory.RecreateClient()
		logging.Debugf(ctx, "Recreating a new PubSub client due to error")
		return nil, err
	}
	return resp.MessageIds, nil
}

// Close closes the UnbufferedPublisher, notably its Client.
func (up *UnbufferedPublisher) Close() error {
	client, err := up.ClientFactory.Client(up.AECtx)
	if err != nil {
		return err
	}
	return client.Close()
}
