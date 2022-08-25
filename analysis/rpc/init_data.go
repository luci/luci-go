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

package rpc

import (
	"context"

	"go.chromium.org/luci/server/auth"

	"go.chromium.org/luci/analysis/internal/config"
	pb "go.chromium.org/luci/analysis/proto/v1"
)

// A server that provides the data to initialize the client.
type initDataGeneratorServer struct{}

// Creates a new initialization data server.
func NewInitDataGeneratorServer() *pb.DecoratedInitDataGenerator {
	return &pb.DecoratedInitDataGenerator{
		Prelude:  checkAllowedPrelude,
		Service:  &initDataGeneratorServer{},
		Postlude: gRPCifyAndLogPostlude,
	}
}

// Gets the initialization data.
func (*initDataGeneratorServer) GenerateInitData(ctx context.Context, request *pb.GenerateInitDataRequest) (*pb.GenerateInitDataResponse, error) {
	logoutURL, err := auth.LogoutURL(ctx, request.ReferrerUrl)
	if err != nil {
		return nil, err
	}

	config, err := config.Get(ctx)
	if err != nil {
		return nil, err
	}

	return &pb.GenerateInitDataResponse{
		InitData: &pb.InitData{
			Hostnames: &pb.Hostnames{
				MonorailHostname: config.MonorailHostname,
			},
			User: &pb.User{
				Email: auth.CurrentUser(ctx).Email,
			},
			AuthUrls: &pb.AuthUrls{
				LogoutUrl: logoutURL,
			},
		},
	}, nil
}
