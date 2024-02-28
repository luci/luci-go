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
}

type prodClient struct {
	baseClient *pubsub.Client
}

// newProdClient creates a new production Pubsub client (not a mock).
func newProdClient(ctx context.Context) (*prodClient, error) {
	project := GetProject(ctx)
	client, err := pubsub.NewClient(ctx, project)
	if err != nil {
		return nil, errors.Annotate(err, "failed to create PubSub client for project %s", project).Err()
	}

	return &prodClient{
		baseClient: client,
	}, nil
}

func (c *prodClient) Close() error {
	if c.baseClient != nil {
		if err := c.baseClient.Close(); err != nil {
			return errors.Annotate(err, "error closing PubSub client").Err()
		}
		c.baseClient = nil
	}
	return nil
}

func (c *prodClient) GetIAMPolicy(ctx context.Context) (*iam.Policy, error) {
	if c.baseClient == nil {
		return nil, status.Error(codes.Internal, "aborting - no PubSub client")
	}

	p, err := c.baseClient.Topic(AuthDBChangeTopicName).IAM().Policy(ctx)
	if err != nil {
		return nil, err
	}

	return p, nil
}

func (c *prodClient) SetIAMPolicy(ctx context.Context, policy *iam.Policy) error {
	if c.baseClient == nil {
		return status.Error(codes.Internal, "aborting - no PubSub client")
	}

	err := c.baseClient.Topic(AuthDBChangeTopicName).IAM().SetPolicy(ctx, policy)
	if err != nil {
		return err
	}

	return nil
}
