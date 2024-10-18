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
	"cloud.google.com/go/pubsub/pstest"
	"google.golang.org/api/option"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// SetupTestPubsub creates a new fake Pub/Sub server and the client connection
// to the server. It also adds the client into the context.
func SetupTestPubsub(ctx context.Context, cloudProject string) (context.Context, *pstest.Server, *pubsub.Client, error) {
	srv := pstest.NewServer()
	conn, err := grpc.NewClient(srv.Addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return ctx, nil, nil, err
	}

	client, err := pubsub.NewClient(ctx, cloudProject, option.WithGRPCConn(conn))
	if err != nil {
		return ctx, nil, nil, err
	}

	mockClients, ok := ctx.Value(&mockPubsubClientKey).(map[string]*pubsub.Client)
	if !ok {
		mockClients = make(map[string]*pubsub.Client)
	}
	mockClients[cloudProject] = client
	ctx = context.WithValue(ctx, &mockPubsubClientKey, mockClients)
	return ctx, srv, client, nil
}
