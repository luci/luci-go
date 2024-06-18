// Copyright 2024 The LUCI Authors.
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

// Package main defines the entrypoint to the experiments service.
package main

import (
	"go.chromium.org/luci/config/server/cfgmodule"
	"go.chromium.org/luci/server"
	"go.chromium.org/luci/server/module"
	quota "go.chromium.org/luci/server/quotabeta"

	"go.chromium.org/luci/resultdb/internal"
	"go.chromium.org/luci/resultdb/internal/rpcquota"
	"go.chromium.org/luci/resultdb/internal/services/experiments"
)

func main() {
	modules := []module.Module{
		// rpcquota and cfgmodule must be installed before quota, so
		// they need to be passed here rather than declared as
		// Dependencies of quota.
		rpcquota.NewModuleFromFlags(),
		cfgmodule.NewModuleFromFlags(),
		quota.NewModuleFromFlags(),
	}
	internal.MainWithModules(modules, func(srv *server.Server) error {
		return experiments.InitServer(srv)
	})
}
