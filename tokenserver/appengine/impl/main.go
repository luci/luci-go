// Copyright 2021 The LUCI Authors.
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

// Package impl holds code to initialize the server environment.
package impl

import (
	"go.chromium.org/luci/config/server/cfgmodule"
	"go.chromium.org/luci/config/validation"
	"go.chromium.org/luci/server"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/cron"
	"go.chromium.org/luci/server/gaeemulation"
	"go.chromium.org/luci/server/module"
	"go.chromium.org/luci/server/redisconn"
	"go.chromium.org/luci/server/tq"

	"go.chromium.org/luci/tokenserver/api/admin/v1"
	"go.chromium.org/luci/tokenserver/api/minter/v1"
	"go.chromium.org/luci/tokenserver/appengine/impl/services/admin/adminsrv"
	"go.chromium.org/luci/tokenserver/appengine/impl/services/admin/certauthorities"
	"go.chromium.org/luci/tokenserver/appengine/impl/services/minter/tokenminter"
)

// Services carries concrete implementation of all Token Server RPC services.
type Services struct {
	Admin  admin.AdminServer
	Certs  admin.CertificateAuthoritiesServer
	Minter minter.TokenMinterServer
}

// Main initializes and runs the server.
func Main(init func(srv *server.Server, services *Services) error) {
	modules := []module.Module{
		cfgmodule.NewModuleFromFlags(),
		cron.NewModuleFromFlags(),
		gaeemulation.NewModuleFromFlags(),
		redisconn.NewModuleFromFlags(),
		tq.NewModuleFromFlags(),
	}

	server.Main(nil, modules, func(srv *server.Server) error {
		signer := auth.GetSigner(srv.Context)

		adminSrv := adminsrv.NewServer(signer)
		adminSrv.ImportCAConfigsRPC.SetupConfigValidation(&validation.Rules)
		adminSrv.ImportDelegationConfigsRPC.SetupConfigValidation(&validation.Rules)
		adminSrv.ImportProjectIdentityConfigsRPC.SetupConfigValidation(&validation.Rules)
		adminSrv.ImportProjectOwnedAccountsConfigsRPC.SetupConfigValidation(&validation.Rules)

		return init(srv, &Services{
			Admin:  adminSrv,
			Certs:  certauthorities.NewServer(),
			Minter: tokenminter.NewServer(signer, srv.Options.Prod),
		})
	})
}
