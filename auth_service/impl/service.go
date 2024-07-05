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

// Package impl contains code shared by `frontend` and `backend` services.
package impl

import (
	"go.chromium.org/luci/config/server/cfgmodule"
	"go.chromium.org/luci/server"
	"go.chromium.org/luci/server/gaeemulation"
	"go.chromium.org/luci/server/module"
	"go.chromium.org/luci/server/secrets"
	"go.chromium.org/luci/server/tq"

	"go.chromium.org/luci/auth_service/impl/info"
	"go.chromium.org/luci/auth_service/internal/configs/validation"
)

// Main launches a server with some default modules and configuration installed.
func Main(modules []module.Module, cb func(srv *server.Server) error) {
	authDBProvider := &AuthDBProvider{}

	opts := &server.Options{
		// Use the AuthDB built directly from the datastore entities.
		AuthDBProvider: authDBProvider.GetAuthDB,
	}

	modules = append([]module.Module{
		gaeemulation.NewModuleFromFlags(), // for accessing Datastore
		secrets.NewModuleFromFlags(),      // for accessing Cloud Secret Manager
		tq.NewModuleFromFlags(),
		cfgmodule.NewModuleFromFlags(), // for accessing luci-cfg configs.
	}, modules...)

	server.Main(opts, modules, func(srv *server.Server) error {
		// Inject app version into the root context so request handlers can use it.
		srv.Context = info.SetImageVersion(srv.Context, srv.Options.ImageVersion())

		// Add validation for realms[-dev].cfg files.
		validation.RegisterRealmsCfgValidation(srv.Context)

		srv.RunInBackground("authdb", authDBProvider.RefreshPeriodically)
		return cb(srv)
	})
}
