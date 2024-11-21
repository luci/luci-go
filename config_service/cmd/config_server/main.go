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
	"fmt"

	"go.chromium.org/luci/common/errors"
	cfgValidation "go.chromium.org/luci/config/validation"
	"go.chromium.org/luci/gae/service/info"
	"go.chromium.org/luci/grpc/prpc"
	"go.chromium.org/luci/server"
	"go.chromium.org/luci/server/cron"
	"go.chromium.org/luci/server/encryptedcookies"
	"go.chromium.org/luci/server/gaeemulation"
	"go.chromium.org/luci/server/module"
	"go.chromium.org/luci/server/redisconn"
	"go.chromium.org/luci/server/secrets"
	"go.chromium.org/luci/server/tq"

	"go.chromium.org/luci/config_service/internal/clients"
	"go.chromium.org/luci/config_service/internal/importer"
	"go.chromium.org/luci/config_service/internal/retention"
	"go.chromium.org/luci/config_service/internal/schema"
	"go.chromium.org/luci/config_service/internal/service"
	"go.chromium.org/luci/config_service/internal/settings"
	"go.chromium.org/luci/config_service/internal/ui"
	"go.chromium.org/luci/config_service/internal/validation"
	configpb "go.chromium.org/luci/config_service/proto"
	"go.chromium.org/luci/config_service/rpc"

	// Register validation rules for LUCI Config itself.
	_ "go.chromium.org/luci/config_service/internal/rules"
	// Store auth sessions in the datastore.
	_ "go.chromium.org/luci/server/encryptedcookies/session/datastore"
	// Enable gRPC server side gzip compression.
	_ "google.golang.org/grpc/encoding/gzip"
)

func main() {
	mods := []module.Module{
		cron.NewModuleFromFlags(),
		encryptedcookies.NewModuleFromFlags(),
		gaeemulation.NewModuleFromFlags(),
		redisconn.NewModuleFromFlags(),
		secrets.NewModuleFromFlags(),
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
		gsBucket := fmt.Sprintf("storage-%s", info.AppID(srv.Context))

		serviceFinder, err := service.NewFinder(srv.Context)
		if err != nil {
			return errors.Annotate(err, "failed to create service finder").Err()
		}
		srv.RunInBackground("refresh-service-finder", serviceFinder.RefreshPeriodically)
		validator := &validation.Validator{
			GsClient:    gsClient,
			Finder:      serviceFinder,
			SelfRuleSet: &cfgValidation.Rules,
		}
		importer := importer.Importer{
			Validator: validator,
			GSBucket:  gsBucket,
		}

		configsServer := &rpc.Configs{
			Validator:          validator,
			GSValidationBucket: gsBucket,
		}
		configpb.RegisterConfigsServer(srv, configsServer)
		cron.RegisterHandler("update-services", service.Update)
		cron.RegisterHandler("delete-configs", retention.DeleteStaleConfigs)
		importer.RegisterImportConfigsCron(&tq.Default)
		ui.InstallHandlers(srv, configsServer, importer)
		schema.InstallHandler(srv.Routes)

		// Enable protobuf v2 for JSONPB field masks support. This will be default
		// at some point.
		srv.ConfigurePRPC(func(srv *prpc.Server) {
			srv.UseProtobufV2 = true
		})

		return nil
	})
}
