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
	"go.chromium.org/luci/server"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/openid"
	"go.chromium.org/luci/server/router"

	configpb "go.chromium.org/luci/config_service/proto"
	"go.chromium.org/luci/config_service/rpc"
)

func main() {
	server.Main(nil, nil, func(srv *server.Server) error {
		mw := router.MiddlewareChain{
			auth.Authenticate(&openid.GoogleIDTokenAuthMethod{
				AudienceCheck: openid.AudienceMatchesHost,
			}),
		}
		// TODO(crbug.com/1444599): for debugging purpose.
		srv.Routes.GET("/", mw, func(c *router.Context) {
			c.Writer.Write([]byte("Hello world!"))
		})
		configpb.RegisterConfigsServer(srv, rpc.NewConfigs())
		return nil
	})
}
