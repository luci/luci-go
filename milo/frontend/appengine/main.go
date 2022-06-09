// Copyright 2017 The LUCI Authors.
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

package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	buildbucketpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/config/server/cfgmodule"
	"go.chromium.org/luci/grpc/prpc"
	milopb "go.chromium.org/luci/milo/api/service/v1"
	"go.chromium.org/luci/milo/backend"
	"go.chromium.org/luci/milo/buildsource/buildbucket"
	"go.chromium.org/luci/milo/common"
	"go.chromium.org/luci/milo/frontend"
	"go.chromium.org/luci/milo/git"
	"go.chromium.org/luci/milo/git/gitacls"
	"go.chromium.org/luci/server"
	"go.chromium.org/luci/server/analytics"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/caching"
	"go.chromium.org/luci/server/cron"
	"go.chromium.org/luci/server/encryptedcookies"
	"go.chromium.org/luci/server/gaeemulation"
	"go.chromium.org/luci/server/module"
	"go.chromium.org/luci/server/redisconn"
	"go.chromium.org/luci/server/secrets"

	// Register store impl for encryptedcookies module.
	_ "go.chromium.org/luci/server/encryptedcookies/session/datastore"
)

func main() {
	modules := []module.Module{
		cfgmodule.NewModuleFromFlags(),
		cron.NewModuleFromFlags(),
		secrets.NewModuleFromFlags(),
		encryptedcookies.NewModuleFromFlags(),
		gaeemulation.NewModuleFromFlags(),
		redisconn.NewModuleFromFlags(),
		analytics.NewModuleFromFlags(),
	}
	server.Main(nil, modules, func(srv *server.Server) error {
		frontend.Run(srv, "templates")
		cron.RegisterHandler("update-config", frontend.UpdateConfigHandler)
		cron.RegisterHandler("update-pools", buildbucket.UpdatePools)
		cron.RegisterHandler("update-builders", buildbucket.UpdateBuilders)
		cron.RegisterHandler("delete-builds", buildbucket.DeleteOldBuilds)
		cron.RegisterHandler("sync-builds", buildbucket.SyncBuilds)
		milopb.RegisterMiloInternalServer(srv.PRPC, &backend.MiloInternalService{
			GetGitClient: func(c context.Context) (git.Client, error) {
				acls, err := gitacls.FromConfig(c, common.GetSettings(c).SourceAcls)
				if err != nil {
					return nil, err
				}
				return git.NewClient(acls), nil
			},
			GetBuildersClient: func(c context.Context, as auth.RPCAuthorityKind) (buildbucketpb.BuildersClient, error) {
				settings := common.GetSettings(c)
				host := settings.GetBuildbucket().GetHost()
				if host == "" {
					return nil, errors.New("missing buildbucket host in settings")
				}

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
			GetCachedAccessClient: func(c context.Context) (*common.CachedAccessClient, error) {
				settings := common.GetSettings(c)
				host := settings.GetBuildbucket().GetHost()
				if host == "" {
					return nil, errors.New("missing buildbucket host in settings")
				}

				cache := caching.GlobalCache(c, fmt.Sprintf("buildbucket-access-%s", settings.Buildbucket.Host))
				if cache == nil {
					return nil, errors.New("no global cache configured")
				}

				return common.NewCachedAccessClient(c, settings.Buildbucket.Host, cache)
			},
		})
		return nil
	})
}
