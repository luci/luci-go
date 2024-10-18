// Copyright 2021 The LUCI Authors.
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
	"fmt"
	"time"

	"cloud.google.com/go/pubsub"
	"cloud.google.com/go/pubsub/pstest"
	"google.golang.org/api/option"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
)

// TestPSServer embeds a pubsub test server for testing PullingBatchProcessor.
//
// It also exposes a client that can be used to talk to the test server and the
// fields needed to retrieve messages from a subscription.
type TestPSServer struct {
	*pstest.Server

	Client *pubsub.Client
	ProjID string
	SubID  string

	topicID, subName, topicName string
	closers                     []func() error
}

// NewTestPSServer starts a test pubsub server, binds a client to it,
// creates a default topic and subscription.
func NewTestPSServer(ctx context.Context) (*TestPSServer, error) {
	ret := &TestPSServer{}
	var err error
	defer func() {
		if err != nil {
			if closeErr := ret.Close(); closeErr != nil {
				logging.Errorf(ctx, "Error closing mock pubsub server: %s", closeErr)
			}
		}
	}()

	ret.topicID, ret.ProjID, ret.SubID = "all-builds", "some-project", "some-sub-id"
	ret.subName = fmt.Sprintf("projects/%s/subscriptions/%s", ret.ProjID, ret.SubID)
	ret.topicName = fmt.Sprintf("projects/%s/topics/%s", ret.ProjID, ret.topicID)
	ret.Server = pstest.NewServer()
	conn, err := grpc.NewClient(ret.Server.Addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, err
	}
	ret.closers = append(ret.closers, conn.Close)
	ret.Client, err = pubsub.NewClient(ctx, ret.ProjID, option.WithGRPCConn(conn))
	if err != nil {
		return nil, err
	}
	ret.closers = append(ret.closers, ret.Client.Close)
	topic, err := ret.Client.CreateTopic(ctx, ret.topicID)
	if err != nil {
		return nil, err
	}
	_, err = ret.Client.CreateSubscription(ctx, ret.SubID, pubsub.SubscriptionConfig{
		Topic:       topic,
		AckDeadline: 600 * time.Second,
	})
	if err != nil {
		return nil, err
	}
	return ret, nil
}

// Close closes the embedded server and also the pubsub client and underlying
// grpc connection.
func (mps *TestPSServer) Close() error {
	errs := &errors.MultiError{}
	for i := len(mps.closers) - 1; i >= 0; i-- {
		errs.MaybeAdd(mps.closers[i]())
	}
	errs.MaybeAdd(mps.Server.Close())
	return errs.AsError()
}

// PublishTestMessages puts `n` distinct messages into the test servers default
// topic.
func (mps *TestPSServer) PublishTestMessages(n int) stringset.Set {
	out := stringset.New(n)
	for i := 0; i < n; i++ {
		msgStr := fmt.Sprintf("msg%03d", i)
		out.Add(msgStr)
		// The call below returns the ID of the mock message, ignore it.
		_ = mps.Publish(mps.topicName, []byte(msgStr), nil)
	}
	return out
}
