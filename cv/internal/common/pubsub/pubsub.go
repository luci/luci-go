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

// Package pubsub handles publishing messages for CV events.
package pubsub

import (
	"context"

	"cloud.google.com/go/pubsub"
	"google.golang.org/api/option"
	"google.golang.org/grpc"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/grpc/grpcmon"
	"go.chromium.org/luci/server/auth"
)

// Client defines a set of the Google pubsub.Client functions that are
// used in CV.
//
// This interface is to mock the Google pubsub.Client for testing purposes.
type Client interface {
}

// NewClient constructs a PubSub client.
func NewClient(ctx context.Context, cloudProject string) (Client, error) {
	creds, err := auth.GetPerRPCCredentials(ctx, auth.AsSelf, auth.WithScopes(auth.CloudOAuthScopes...))
	if err != nil {
		return nil, errors.Annotate(err, "failed to initialize credentials").Err()
	}
	client, err := pubsub.NewClient(ctx, cloudProject,
		option.WithGRPCDialOption(grpcmon.WithClientRPCStatsMonitor()),
		option.WithGRPCDialOption(grpc.WithPerRPCCredentials(creds)),
	)
	if err != nil {
		return nil, errors.Annotate(err, "failed to create PubSub client").Err()
	}
	return &psClient{client}, nil
}

type psClient struct {
	psc *pubsub.Client
}
