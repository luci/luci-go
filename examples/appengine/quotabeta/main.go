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

// Package main contains a GAE app demonstrating how to use the server/quotabeta
// module to implement rate limiting for requests. Navigate to /rpcexplorer
// to try out quota operations.
//
// Not intended to be run locally. A local demo can be found under
// server/quotabeta/example.
package main

import (
	"go.chromium.org/luci/config/server/cfgmodule"
	pb "go.chromium.org/luci/examples/appengine/quotabeta/proto"
	"go.chromium.org/luci/examples/appengine/quotabeta/rpc"
	"go.chromium.org/luci/server"
	"go.chromium.org/luci/server/module"
	quota "go.chromium.org/luci/server/quotabeta"
	quotapb "go.chromium.org/luci/server/quotabeta/proto"
	"go.chromium.org/luci/server/quotabeta/quotaconfig"
	"go.chromium.org/luci/server/redisconn"
)

func main() {
	modules := []module.Module{
		cfgmodule.NewModuleFromFlags(),
		quota.NewModuleFromFlags(),
		redisconn.NewModuleFromFlags(),
	}

	server.Main(nil, modules, func(srv *server.Server) error {
		// Initialize a static, in-memory implementation of quotaconfig.Interface.
		// See the quota/rpc package for how these quotas are used.
		// TODO(crbug/1280055): Fetch from the config service.
		m, err := quotaconfig.NewMemory(srv.Context, []*quotapb.Policy{
			{
				Name:          "global-rate-limit",
				Resources:     60,
				Replenishment: 1,
			},
			{
				Name:          "per-user-rate-limit/${user}",
				Resources:     60,
				Replenishment: 1,
			},
		})
		if err != nil {
			panic(err)
		}

		// Register the quotaconfig.Interface.
		srv.Context = quota.Use(srv.Context, m)

		// Register the demo RPC service.
		pb.RegisterDemoServer(srv, rpc.New())
		return nil
	})
}
