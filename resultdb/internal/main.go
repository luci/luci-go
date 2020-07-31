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
	"flag"

	"go.chromium.org/luci/server"
	"go.chromium.org/luci/server/limiter"
	"go.chromium.org/luci/server/module"
	"go.chromium.org/luci/server/redisconn"
	"go.chromium.org/luci/server/secrets"
	"go.chromium.org/luci/server/span"
	"go.chromium.org/luci/server/tq"
)

// Main registers all dependencies and runs a service.
func Main(init func(srv *server.Server) error) {
	modules := []module.Module{
		limiter.NewModuleFromFlags(),
		secrets.NewModuleFromFlags(),
		redisconn.NewModuleFromFlags(),
		span.NewModuleFromFlags(),
		tq.NewModuleFromFlags(),
	}

	// TODO(vadimsh): Should this be removed?
	_ = flag.Bool("resultdb-prod", false, "Run ResultDB in production mode")

	server.Main(nil, modules, init)
}
