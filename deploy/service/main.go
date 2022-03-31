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

// Package main contains the main LUCI Deploy service binary.
package main

import (
	"go.chromium.org/luci/hardcoded/chromeinfra"
	"go.chromium.org/luci/server"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/openid"
	"go.chromium.org/luci/server/auth/rpcacl"
	"go.chromium.org/luci/server/cron"
	"go.chromium.org/luci/server/encryptedcookies"
	"go.chromium.org/luci/server/gaeemulation"
	"go.chromium.org/luci/server/module"
	"go.chromium.org/luci/server/secrets"
	"go.chromium.org/luci/server/tq"

	"go.chromium.org/luci/deploy/api/rpcpb"
	"go.chromium.org/luci/deploy/service/rpcs"
	"go.chromium.org/luci/deploy/service/ui"

	// Using datastore for user sessions.
	_ "go.chromium.org/luci/server/encryptedcookies/session/datastore"
	// Using datastore for transactional tasks.
	_ "go.chromium.org/luci/server/tq/txn/datastore"
	// Shut up error message.
	_ "go.chromium.org/luci/gae/service/datastore/crbug1242998safeget"
)

const (
	// Members are actuation agents running actual deployments.
	actuatorsGroup = "luci-deploy-actuators"
	// Members have read-only access to the UI and API.
	accessGroup = "luci-deploy-access"
)

// RPC-level ACLs.
var rpcACL = rpcacl.Map{
	"/discovery.Discovery/*":       rpcacl.All,
	"/deploy.service.Actuations/*": actuatorsGroup,
	"/deploy.service.Assets/*":     accessGroup,
}

func main() {
	modules := []module.Module{
		cron.NewModuleFromFlags(),
		encryptedcookies.NewModuleFromFlags(),
		gaeemulation.NewModuleFromFlags(),
		secrets.NewModuleFromFlags(),
		tq.NewModuleFromFlags(),
	}

	server.Main(nil, modules, func(srv *server.Server) error {
		actuations := rpcs.Actuations{}
		assets := rpcs.Assets{}

		// All pRPC APIs.
		rpcpb.RegisterActuationsServer(srv.PRPC, &actuations)
		rpcpb.RegisterAssetsServer(srv.PRPC, &assets)

		// Authentication methods for pRPC APIs.
		srv.PRPC.Authenticator = &auth.Authenticator{
			Methods: []auth.Method{
				// The preferred authentication method.
				&openid.GoogleIDTokenAuthMethod{
					Audience: []string{
						// Used by `prpc` binary.
						chromeinfra.DefaultAuthOptions().ClientID,
					},
					AudienceCheck: openid.AudienceMatchesHost,
					SkipNonJWT:    true, // pass OAuth2 access tokens through
				},
				// Backward compatibility for the RPC Explorer and old clients.
				&auth.GoogleOAuth2Method{
					Scopes: []string{"https://www.googleapis.com/auth/userinfo.email"},
				},
			},
		}

		// Per-RPC authorization interceptor.
		srv.PRPC.UnaryServerInterceptor = rpcacl.Interceptor(rpcACL)

		// Web UI routes.
		ui.RegisterRoutes(srv, accessGroup, &assets)

		return nil
	})
}
