// Copyright 2018 The LUCI Authors.
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

// Package main is the main entry point for the app.
package main

import (
	"context"
	"errors"
	"net/http"

	"google.golang.org/appengine"

	gaeserver "go.chromium.org/luci/appengine/gaeauth/server"
	"go.chromium.org/luci/appengine/gaemiddleware/standard"
	cfgcommonpb "go.chromium.org/luci/common/proto/config"
	"go.chromium.org/luci/config/appengine/gaeconfig"
	"go.chromium.org/luci/config/server/cfgmodule"
	"go.chromium.org/luci/config/validation"
	"go.chromium.org/luci/grpc/discovery"
	"go.chromium.org/luci/grpc/grpcmon"
	"go.chromium.org/luci/grpc/grpcutil"
	"go.chromium.org/luci/grpc/prpc"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/signing"
	"go.chromium.org/luci/server/router"
	"go.chromium.org/luci/web/rpcexplorer"

	server "go.chromium.org/luci/gce/api/config/v1"
	"go.chromium.org/luci/gce/api/instances/v1"
	"go.chromium.org/luci/gce/api/projects/v1"
	"go.chromium.org/luci/gce/appengine/backend"
	"go.chromium.org/luci/gce/appengine/config"
	"go.chromium.org/luci/gce/appengine/rpc"
	"go.chromium.org/luci/gce/vmtoken"
)

func main() {
	api := prpc.Server{
		UnaryServerInterceptor: grpcutil.ChainUnaryServerInterceptors(
			grpcmon.UnaryServerInterceptor,
			auth.AuthenticatingInterceptor([]auth.Method{
				&gaeserver.OAuth2Method{Scopes: []string{gaeserver.EmailScope}},
			}).Unary(),
		),
		// TODO(crbug/1082369): Remove this workaround once non-standard field masks
		// are no longer used in the API.
		EnableNonStandardFieldMasks: true,
	}
	server.RegisterConfigurationServer(&api, rpc.NewConfigurationServer())
	instances.RegisterInstancesServer(&api, rpc.NewInstancesServer())
	projects.RegisterProjectsServer(&api, rpc.NewProjectsServer())
	cfgcommonpb.RegisterConsumerServer(&api, &cfgmodule.ConsumerServer{
		Rules: &validation.Rules,
		GetConfigServiceAccountFn: func(ctx context.Context) (string, error) {
			settings, err := gaeconfig.FetchCachedSettings(ctx)
			switch {
			case err != nil:
				return "", err
			case settings.ConfigServiceHost == "":
				return "", errors.New("can not find config service host from settings")
			}
			info, err := signing.FetchServiceInfoFromLUCIService(ctx, "https://"+settings.ConfigServiceHost)
			if err != nil {
				return "", err
			}
			return info.ServiceAccountName, nil
		},
	})
	discovery.Enable(&api)

	r := router.New()

	standard.InstallHandlers(r)
	rpcexplorer.Install(r, nil)

	mw := standard.Base()
	api.InstallHandlers(r, mw.Extend(vmtoken.Middleware))
	backend.InstallHandlers(r, mw)
	config.InstallHandlers(r, mw)

	http.DefaultServeMux.Handle("/", r)
	appengine.Main()
}
