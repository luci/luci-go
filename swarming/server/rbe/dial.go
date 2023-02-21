// Copyright 2023 The LUCI Authors.
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

package rbe

import (
	"context"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/grpc/grpcmon"
	"go.chromium.org/luci/server/auth"
)

// Dial dials RBE backend with proper authentication.
func Dial(ctx context.Context) (*grpc.ClientConn, error) {
	creds, err := auth.GetPerRPCCredentials(ctx,
		auth.AsSelf,
		auth.WithScopes(auth.CloudOAuthScopes...),
	)
	if err != nil {
		return nil, errors.Annotate(err, "failed to get credentials").Err()
	}

	logging.Infof(ctx, "Dialing the RBE backend...")
	conn, err := grpc.DialContext(ctx, "remotebuildexecution.googleapis.com:443",
		grpc.WithTransportCredentials(credentials.NewTLS(nil)),
		grpc.WithPerRPCCredentials(creds),
		grpc.WithBlock(),
		grpcmon.WithClientRPCStatsMonitor(),
	)
	if err != nil {
		return nil, errors.Annotate(err, "failed to dial RBE backend").Err()
	}
	return conn, nil
}
