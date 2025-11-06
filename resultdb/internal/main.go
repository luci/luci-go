// Copyright 2019 The LUCI Authors.
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

package internal

import (
	"go.chromium.org/luci/auth/scopes"
	"go.chromium.org/luci/common/logging"
	grpcLogging "go.chromium.org/luci/grpc/logging"
	"go.chromium.org/luci/server"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/gaeemulation"
	"go.chromium.org/luci/server/gerritauth"
	"go.chromium.org/luci/server/limiter"
	"go.chromium.org/luci/server/module"
	"go.chromium.org/luci/server/redisconn"
	"go.chromium.org/luci/server/secrets"
	"go.chromium.org/luci/server/span"
	"go.chromium.org/luci/server/tq"
)

// Main registers all dependencies and runs a service.
func Main(init func(srv *server.Server) error) {
	MainWithModules([]module.Module{}, init)
}

// MainWithModules is like Main, but accepts additional modules to register.
func MainWithModules(modules []module.Module, init func(srv *server.Server) error) {
	defaultModules := []module.Module{
		gaeemulation.NewModuleFromFlags(),
		gerritauth.NewModuleFromFlags(),
		limiter.NewModuleFromFlags(),
		redisconn.NewModuleFromFlags(),
		secrets.NewModuleFromFlags(),
		span.NewModuleFromFlags(nil),
		tq.NewModuleFromFlags(),
	}
	modules = append(modules, defaultModules...)
	server.Main(nil, modules, func(srv *server.Server) error {
		srv.SetRPCAuthMethods([]auth.Method{
			// The default method used by majority of clients.
			&auth.GoogleOAuth2Method{
				Scopes: []string{scopes.Email},
			},
			// For authenticating calls from Gerrit plugins.
			&gerritauth.Method,
		})

		// Route gRPC logs to LUCI logs. Only keep warnings and above in severity to
		// avoid log spam.
		logger := logging.Get(srv.Context)
		grpcLogging.Install(logger, logging.Debug)

		return init(srv)
	})
}
