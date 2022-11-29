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

package clients

import (
	"context"

	"cloud.google.com/go/pubsub"
	"google.golang.org/api/option"
	"google.golang.org/grpc"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/grpc/grpcmon"
	"go.chromium.org/luci/server/auth"
)

var mockPubsubClientKey = "mock pubsub clients key for testing only"

func NewPubsubClient(ctx context.Context, cloudProject, luciProject string) (*pubsub.Client, error) {
	if mockClients, ok := ctx.Value(&mockPubsubClientKey).(map[string]*pubsub.Client); ok {
		if mockClient, exist := mockClients[cloudProject]; exist {
			return mockClient, nil
		}
		return nil, errors.Reason("couldn't find mock pubsub client for %s", cloudProject).Err()
	}

	creds, err := auth.GetPerRPCCredentials(ctx, auth.AsProject, auth.WithProject(luciProject), auth.WithScopes(auth.CloudOAuthScopes...))
	if err != nil {
		return nil, err
	}
	client, err := pubsub.NewClient(
		ctx, cloudProject,
		option.WithGRPCDialOption(grpcmon.WithClientRPCStatsMonitor()),
		option.WithGRPCDialOption(grpc.WithPerRPCCredentials(creds)),
	)
	if err != nil {
		return nil, err
	}
	return client, nil
}
