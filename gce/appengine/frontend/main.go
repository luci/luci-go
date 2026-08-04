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
	"strings"

	"google.golang.org/appengine"

	"go.chromium.org/luci/appengine/gaeauth/client"
	gaeauth "go.chromium.org/luci/appengine/gaeauth/server"
	"go.chromium.org/luci/appengine/gaeauth/server/gaesigner"
	"go.chromium.org/luci/appengine/gaemiddleware"
	gaetsmon "go.chromium.org/luci/appengine/tsmon"
	"go.chromium.org/luci/auth/scopes"
	cfgcommonpb "go.chromium.org/luci/common/proto/config"
	"go.chromium.org/luci/common/tsmon/target"
	"go.chromium.org/luci/config/appengine/gaeconfig"
	"go.chromium.org/luci/config/server/cfgmodule"
	"go.chromium.org/luci/config/validation"
	"go.chromium.org/luci/gae/impl/prod"
	"go.chromium.org/luci/gae/service/info"
	"go.chromium.org/luci/grpc/discovery"
	"go.chromium.org/luci/grpc/grpcmon"
	"go.chromium.org/luci/grpc/grpcutil"
	"go.chromium.org/luci/grpc/prpc"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authdb"
	"go.chromium.org/luci/server/auth/signing"
	"go.chromium.org/luci/server/middleware"
	"go.chromium.org/luci/server/portal"
	"go.chromium.org/luci/server/router"
	"go.chromium.org/luci/server/tsmon"
	"go.chromium.org/luci/web/rpcexplorer"

	server "go.chromium.org/luci/gce/api/config/v1"
	"go.chromium.org/luci/gce/api/instances/v1"
	"go.chromium.org/luci/gce/api/projects/v1"
	"go.chromium.org/luci/gce/appengine/backend"
	"go.chromium.org/luci/gce/appengine/config"
	"go.chromium.org/luci/gce/appengine/rpc"
	"go.chromium.org/luci/gce/vmtoken"
)

type contextAwareURLFetch struct{ ctx context.Context }

func (f *contextAwareURLFetch) RoundTrip(req *http.Request) (*http.Response, error) {
	if ctx := req.Context(); ctx == nil || ctx == context.Background() {
		req = req.WithContext(f.ctx)
	}
	return http.DefaultTransport.RoundTrip(req)
}

var (
	authConfig = auth.Config{
		DBProvider:          authdb.NewDBCache(gaeauth.GetAuthDB),
		Signer:              gaesigner.Signer{},
		AccessTokenProvider: client.GetAccessToken,
		AnonymousTransport: func(ctx context.Context) http.RoundTripper {
			return &contextAwareURLFetch{ctx}
		},
		FrontendClientID: gaeauth.FetchFrontendClientID,
		IsDevMode:        appengine.IsDevAppServer(),
	}

	tsMonState = &tsmon.State{
		Target: func(ctx context.Context) target.Task {
			return target.Task{
				DataCenter:  "appengine",
				ServiceName: info.AppID(ctx),
				JobName:     info.ModuleName(ctx),
				HostName:    strings.SplitN(info.VersionID(ctx), ".", 2)[0],
			}
		},
		InstanceID:        info.InstanceID,
		TaskNumAllocator:  gaetsmon.DatastoreTaskNumAllocator{},
		FlushInMiddleware: true,
	}

	appEnv = gaemiddleware.Environment{
		MemcacheAvailable:  true,
		WithInitialRequest: prod.Use,
		WithConfig:         gaeconfig.Use,
		WithAuth: func(ctx context.Context) context.Context {
			return auth.Initialize(ctx, &authConfig)
		},
		ExtraMiddleware: func() router.MiddlewareChain {
			mw := make([]router.Middleware, 0, 2)
			if !appengine.IsDevAppServer() {
				mw = append(mw, middleware.WithPanicCatcher)
			}
			mw = append(mw, tsMonState.Middleware)
			return router.NewMiddlewareChain(mw...)
		}(),
		ExtraHandlers: func(r *router.Router, base router.MiddlewareChain) {
			gaeauth.InstallHandlers(r, base)
			gaetsmon.InstallHandlers(r, base)
			portal.InstallHandlers(r, base, &gaeauth.UsersAPIAuthMethod{})
		},
	}
)

func main() {
	api := prpc.Server{
		UnaryServerInterceptor: grpcutil.ChainUnaryServerInterceptors(
			grpcmon.UnaryServerInterceptor,
			auth.AuthenticatingInterceptor([]auth.Method{
				&gaeauth.OAuth2Method{Scopes: []string{scopes.Email}},
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

	appEnv.InstallHandlers(r)
	rpcexplorer.Install(r, nil)

	mw := appEnv.Base()
	api.InstallHandlers(r, mw.Extend(vmtoken.Middleware))
	backend.InstallHandlers(r, mw)
	config.InstallHandlers(r, mw)

	http.DefaultServeMux.Handle("/", r)
	appengine.Main()
}
