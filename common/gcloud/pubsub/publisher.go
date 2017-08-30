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
	"cloud.google.com/go/pubsub"
	vkit "cloud.google.com/go/pubsub/apiv1"
	gax "github.com/googleapis/gax-go"
	pb "google.golang.org/genproto/googleapis/pubsub/v1"

	"golang.org/x/net/context"
)

// Publisher is a generic interface to something that can publish Pub/Sub
// messages.
//
// A Publisher should be Closed when finished with it.
type Publisher interface {
	Publish(c context.Context, msgs ...*pubsub.Message) ([]string, error)
	Close() error
}

// UnbufferedPublisher directly instantiates a Pub/Sub client and publishes a
// message to it.
//
// The standard Pub/Sub library has several issues, especially when used from
// AppEngine:
//	- It uses an empty Context, discarding AppEngine context.
//	- It uses a buffer, which expects a lifecycle beyond that of a simple
//	  AppEngine Request.
type UnbufferedPublisher struct {
	// Topic is the name of the Topic to publish to.
	Topic Topic

	// Client is the Pub/Sub publisher client to use. Client will be closed when
	// this UnbufferedPublisher is closed.
	Client *vkit.PublisherClient

	// CallOpts are arbitrary call options that will be passed to the Publish
	// call.
	CallOpts []gax.CallOption
}

var _ Publisher = (*UnbufferedPublisher)(nil)

// Publish publishes a message immediately, blocking until it completes.
//
// "c" must be an AppEngine context.
func (up *UnbufferedPublisher) Publish(c context.Context, msgs ...*pubsub.Message) ([]string, error) {
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

	resp, err := up.Client.Publish(c, &pb.PublishRequest{
		Topic:    string(up.Topic),
		Messages: messages,
	}, up.CallOpts...)
	if err != nil {
		return nil, err
	}
	return resp.MessageIds, nil
}

// Close closes the UnbufferedPublisher, notably its Client.
func (up *UnbufferedPublisher) Close() error { return up.Client.Close() }
