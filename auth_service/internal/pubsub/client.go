// Copyright 2024 The LUCI Authors.
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

	"cloud.google.com/go/iam"
	"cloud.google.com/go/pubsub"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
)

// PubsubClient abstracts functionality to connect with Pubsub.
//
// Non-production implementations are used for unit testing.
type PubsubClient interface {
	// Close closes the connection to the Pubsub server.
	Close() error

	// GetIAMPolicy returns the IAM policy for the AuthDBChange topic.
	GetIAMPolicy(ctx context.Context) (*iam.Policy, error)

	// SetIAMPolicy sets the IAM policy for the AuthDBChange topic.
	SetIAMPolicy(ctx context.Context, policy *iam.Policy) error

	// Publish publishes the message to the AuthDBChange topic.
	Publish(ctx context.Context, msg *pubsub.Message) error
}

type prodClient struct {
	baseClient *pubsub.Client
	projectID  string
}

// newProdClient creates a new production Pubsub client (not a mock).
func newProdClient(ctx context.Context) (*prodClient, error) {
	projectID := getProject(ctx)
	client, err := pubsub.NewClient(ctx, projectID)
	if err != nil {
		return nil, errors.Fmt("failed to create PubSub client for project %s: %w", projectID, err)
	}

	return &prodClient{
		baseClient: client,
		projectID:  projectID,
	}, nil
}

func (c *prodClient) Close() error {
	if c.baseClient != nil {
		if err := c.baseClient.Close(); err != nil {
			return errors.Fmt("error closing PubSub client: %w", err)
		}
		c.baseClient = nil
	}
	return nil
}

func (c *prodClient) GetIAMPolicy(ctx context.Context) (*iam.Policy, error) {
	if c.baseClient == nil {
		return nil, status.Error(codes.Internal, "aborting - no PubSub client")
	}

	topic, err := c.ensureTopic(ctx, AuthDBChangeTopicName)
	if err != nil {
		return nil, err
	}

	p, err := topic.IAM().Policy(ctx)
	if err != nil {
		return nil, err
	}

	return p, nil
}

func (c *prodClient) SetIAMPolicy(ctx context.Context, policy *iam.Policy) error {
	if c.baseClient == nil {
		return status.Error(codes.Internal, "aborting - no PubSub client")
	}

	topic, err := c.ensureTopic(ctx, AuthDBChangeTopicName)
	if err != nil {
		return err
	}

	if err := topic.IAM().SetPolicy(ctx, policy); err != nil {
		return err
	}

	return nil
}

func (c *prodClient) Publish(ctx context.Context, msg *pubsub.Message) (retErr error) {
	if c.baseClient == nil {
		return status.Error(codes.Internal, "aborting - no PubSub client")
	}

	topic, err := c.ensureTopic(ctx, AuthDBChangeTopicName)
	if err != nil {
		return err
	}

	// Clean up any goroutines that will be created from calling
	// topic.Publish(...), using topic.Stop().
	defer topic.Stop()
	result := topic.Publish(ctx, msg)
	if _, err := result.Get(ctx); err != nil {
		switch status.Code(err) {
		case codes.PermissionDenied:
			return status.Errorf(codes.PermissionDenied,
				"missing permission to publish PubSub message for project %s on topic %s",
				c.projectID, AuthDBChangeTopicName)
		default:
			return status.Errorf(codes.Internal,
				"error publishing Pubsub message: %s", err)
		}
	}

	return nil
}

// ensureTopic returns the PubSub topic with the given name, attempting
// to create it if necessary.
func (c *prodClient) ensureTopic(ctx context.Context, name string) (*pubsub.Topic, error) {
	if c.baseClient == nil {
		return nil, status.Error(codes.Internal, "aborting - no PubSub client")
	}

	topic := c.baseClient.Topic(name)
	ok, err := topic.Exists(ctx)
	if err != nil {
		return nil, errors.Fmt("error checking topic existence: %w", err)
	}
	if !ok {
		// The topic doesn't exist; it must be created.
		logging.Infof(ctx, "creating topic %s in project %s",
			name, c.projectID)
		topic, err = c.baseClient.CreateTopic(ctx, name)
		if err != nil {
			return nil, errors.Fmt("error creating topic %s in project %s: %w",
				name, c.projectID, err)
		}
	}

	return topic, nil
}
