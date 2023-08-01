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

package remote

import (
	"context"

	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"

	"go.chromium.org/luci/common/errors"
	pb "go.chromium.org/luci/config_service/proto"
	"go.chromium.org/luci/grpc/grpcmon"
	"go.chromium.org/luci/server/auth"

	"go.chromium.org/luci/config"
)

// NewV2 returns an implementation of the config Interface which talks to the
// real Luci-config service v2.
func NewV2(ctx context.Context, host string) (config.Interface, error) {
	creds, err := auth.GetPerRPCCredentials(ctx,
		auth.AsSelf,
		auth.WithIDTokenAudience("https://"+host),
	)
	if err != nil {
		return nil, errors.Annotate(err, "failed to get credentials to access %s", host).Err()
	}
	conn, err := grpc.DialContext(ctx, host+":443",
		grpc.WithTransportCredentials(credentials.NewTLS(nil)),
		grpc.WithPerRPCCredentials(creds),
		grpc.WithStatsHandler(&grpcmon.ClientRPCStatsMonitor{}),
		grpc.WithUnaryInterceptor(otelgrpc.UnaryClientInterceptor()),
		grpc.WithStreamInterceptor(otelgrpc.StreamClientInterceptor()),
	)
	if err != nil {
		return nil, errors.Annotate(err, "cannot dial to %s", host).Err()
	}
	return &remoteV2Impl{
		conn:   conn,
		client: pb.NewConfigsClient(conn),
	}, nil
}

var _ config.Interface = &remoteV2Impl{}

// remoteV2Impl implements config.Interface and will make gRPC calls to Config
// Service V2.
type remoteV2Impl struct {
	conn   *grpc.ClientConn
	client pb.ConfigsClient
}

func (r *remoteV2Impl) GetConfig(ctx context.Context, configSet config.Set, path string, metaOnly bool) (*config.Config, error) {
	return nil, errors.New("Hasn't implemented yet. Please don't point to Luci-Config v2 service")
}

func (r *remoteV2Impl) GetProjectConfigs(ctx context.Context, path string, metaOnly bool) ([]config.Config, error) {
	return nil, errors.New("Hasn't implemented yet. Please don't point to Luci-Config v2 service")
}

func (r *remoteV2Impl) GetProjects(ctx context.Context) ([]config.Project, error) {
	return nil, errors.New("Hasn't implemented yet. Please don't point to Luci-Config v2 service")
}

func (r *remoteV2Impl) ListFiles(ctx context.Context, configSet config.Set) ([]string, error) {
	return nil, errors.New("Hasn't implemented yet. Please don't point to Luci-Config v2 service")
}

func (r *remoteV2Impl) Close() error {
	if r == nil || r.conn == nil {
		return nil
	}
	return r.conn.Close()
}
