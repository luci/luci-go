// Copyright 2023 The LUCI Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"flag"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/server"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/openid"
	"go.chromium.org/luci/server/cron"
	"go.chromium.org/luci/server/gaeemulation"
	"go.chromium.org/luci/server/module"
	"go.chromium.org/luci/server/router"
	"go.chromium.org/luci/server/tq"

	// Register validation rules for LUCI Config itself.
	_ "go.chromium.org/luci/config_service/internal/rules"

	"go.chromium.org/luci/config_service/internal/clients"
	"go.chromium.org/luci/config_service/internal/common"
	"go.chromium.org/luci/config_service/internal/service"
	"go.chromium.org/luci/config_service/internal/settings"
	"go.chromium.org/luci/config_service/internal/validation"
	configpb "go.chromium.org/luci/config_service/proto"
	"go.chromium.org/luci/config_service/rpc"
)

func main() {
	mods := []module.Module{
		cron.NewModuleFromFlags(),
		gaeemulation.NewModuleFromFlags(),
		tq.NewModuleFromFlags(),
	}

	loc := settings.GlobalConfigLoc{}
	loc.RegisterFlags(flag.CommandLine)

	server.Main(nil, mods, func(srv *server.Server) error {
		if err := loc.Validate(); err != nil {
			return errors.Annotate(err, "Wrong global config location flag value").Err()
		}
		srv.Context = settings.WithGlobalConfigLoc(srv.Context, loc.GitilesLocation)

		// Install a global Cloud Storage client.
		gsClient, err := clients.NewGsProdClient(srv.Context)
		if err != nil {
			return errors.Annotate(err, "failed to initiate the global GCS client").Err()
		}
		srv.Context = clients.WithGsClient(srv.Context, gsClient)
		gsBucket := common.BucketName(srv.Context)

		serviceFinder, err := service.NewFinder(srv.Context)
		if err != nil {
			return errors.Annotate(err, "failed to create service finder").Err()
		}
		srv.RunInBackground("refresh-service-finder", serviceFinder.RefreshPeriodically)
		validator := &validation.Validator{
			GsClient: gsClient,
			Finder:   serviceFinder,
		}

		mw := router.MiddlewareChain{
			auth.Authenticate(&openid.GoogleIDTokenAuthMethod{
				AudienceCheck: openid.AudienceMatchesHost,
			}),
		}
		// TODO(crbug.com/1444599): for debugging purpose.
		srv.Routes.GET("/", mw, func(c *router.Context) {
			c.Writer.Write([]byte("Hello world!"))
		})
		configpb.RegisterConfigsServer(srv, &rpc.Configs{
			Validator:          validator,
			GSValidationBucket: gsBucket,
		})

		cron.RegisterHandler("update-services", service.Update)
		return nil
	})
}
