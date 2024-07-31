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

// Package server contains the common entrypoint for MILO GAE services.
package server

import (
	"context"
	"net/http"
	"time"

	"github.com/golang/protobuf/proto"

	buildbucketpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/api/gitiles"
	gitilespb "go.chromium.org/luci/common/proto/gitiles"
	"go.chromium.org/luci/config/server/cfgmodule"
	"go.chromium.org/luci/grpc/appstatus"
	"go.chromium.org/luci/grpc/prpc"
	resultpb "go.chromium.org/luci/resultdb/proto/v1"
	"go.chromium.org/luci/server"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/cron"
	"go.chromium.org/luci/server/encryptedcookies"
	"go.chromium.org/luci/server/gaeemulation"
	"go.chromium.org/luci/server/gtm"
	"go.chromium.org/luci/server/loginsessions"
	"go.chromium.org/luci/server/module"
	"go.chromium.org/luci/server/redisconn"
	"go.chromium.org/luci/server/secrets"

	"go.chromium.org/luci/milo/frontend/handlers"
	"go.chromium.org/luci/milo/httpservice"
	"go.chromium.org/luci/milo/internal/buildsource/buildbucket"
	"go.chromium.org/luci/milo/internal/config"
	"go.chromium.org/luci/milo/internal/hosts"
	"go.chromium.org/luci/milo/internal/projectconfig"
	configpb "go.chromium.org/luci/milo/proto/config"
	milopb "go.chromium.org/luci/milo/proto/v1"
	"go.chromium.org/luci/milo/rpc"

	// Register store impl for encryptedcookies module.
	_ "go.chromium.org/luci/server/encryptedcookies/session/datastore"
)

// Main implements the common entrypoint for all MILO GAE services.
//
// Note, if changing responsibiltiy between services, please be aware
// that dispatch.yaml changes are not deployed atomically with service
// changes.
func Main(init func(srv *server.Server) error) {
	modules := []module.Module{
		cfgmodule.NewModuleFromFlags(),
		cron.NewModuleFromFlags(),
		secrets.NewModuleFromFlags(),
		encryptedcookies.NewModuleFromFlags(),
		gaeemulation.NewModuleFromFlags(),
		hosts.NewModuleFromFlags(),
		redisconn.NewModuleFromFlags(),
		gtm.NewModuleFromFlags(),
		loginsessions.NewModuleFromFlags(),
	}
	server.Main(nil, modules, init)
}

// CreateInternalService initialises a new MILO Internal service.
func CreateInternalService() *rpc.MiloInternalService {
	return &rpc.MiloInternalService{
		GetSettings: func(c context.Context) (*configpb.Settings, error) {
			settings := config.GetSettings(c)
			return settings, nil
		},
		GetGitilesClient: func(c context.Context, host string, as auth.RPCAuthorityKind) (gitilespb.GitilesClient, error) {
			t, err := auth.GetRPCTransport(c, as)
			if err != nil {
				return nil, err
			}
			client, err := gitiles.NewRESTClient(&http.Client{Transport: t}, host, false)
			if err != nil {
				return nil, err
			}

			return client, nil
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
}

func RegisterPRPCHandlers(srv *server.Server, service *rpc.MiloInternalService) {
	srv.ConfigurePRPC(func(s *prpc.Server) {
		s.AccessControl = prpc.AllowOriginAll
	})
	milopb.RegisterMiloInternalServer(srv, &milopb.DecoratedMiloInternal{
		Service: service,
		Postlude: func(ctx context.Context, methodName string, rsp proto.Message, err error) error {
			return appstatus.GRPCifyAndLog(ctx, err)
		},
	})
}

func RegisterCrons(srv *server.Server, service *rpc.MiloInternalService) {
	cron.RegisterHandler("update-project-configs", projectconfig.UpdateProjectConfigsHandler)
	cron.RegisterHandler("update-config", config.UpdateConfigHandler)
	cron.RegisterHandler("update-builders", service.UpdateBuilderCache)
	cron.RegisterHandler("delete-builds", buildbucket.DeleteOldBuilds)
	cron.RegisterHandler("sync-builds", buildbucket.SyncBuilds)
}

func RegisterFrontend(srv *server.Server) {
	handlers.Run(srv, "../frontend/templates")

	httpService := httpservice.HTTPService{
		Server: srv,
		GetSettings: func(c context.Context) (*configpb.Settings, error) {
			settings := config.GetSettings(c)
			return settings, nil
		},
		GetResultDBClient: func(c context.Context, host string, as auth.RPCAuthorityKind) (resultpb.ResultDBClient, error) {
			t, err := auth.GetRPCTransport(c, as)
			if err != nil {
				return nil, err
			}

			rpcOpts := prpc.DefaultOptions()
			rpcOpts.PerRPCTimeout = time.Minute - time.Second
			return resultpb.NewResultDBClient(&prpc.Client{
				C:       &http.Client{Transport: t},
				Host:    host,
				Options: rpcOpts,
			}), nil
		},
	}
	httpService.RegisterRoutes()
}
