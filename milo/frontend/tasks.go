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

package frontend

import (
	"context"
	"net/http"
	"time"

	buildbucketpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/grpc/prpc"
	"go.chromium.org/luci/milo/common"
	configpb "go.chromium.org/luci/milo/proto/config"
	"go.chromium.org/luci/milo/rpc"
	"go.chromium.org/luci/server/auth"
)

// UpdateBuilders updates the builders cache if the cache TTL falls below
// cacheRefreshThreshold.
func UpdateBuilders(c context.Context) error {
	service := &rpc.MiloInternalService{
		GetSettings: func(c context.Context) (*configpb.Settings, error) {
			settings := common.GetSettings(c)
			return settings, nil
		},
		GetBuildersClient: func(c context.Context, host string, as auth.RPCAuthorityKind) (buildbucketpb.BuildersClient, error) {
			t, err := auth.GetRPCTransport(c, as)
			if err != nil {
				return nil, err
			}

			rpcOpts := prpc.DefaultOptions()
			rpcOpts.PerRPCTimeout = time.Minute - time.Second
			return buildbucketpb.NewBuildersClient(&prpc.Client{
				C:       &http.Client{Transport: t},
				Host:    host,
				Options: rpcOpts,
			}), nil
		},
	}

	return service.UpdateBuilderCache(c)
}
