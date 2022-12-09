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

package main

import (
	"context"
	"flag"

	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/server"
	"go.chromium.org/luci/server/cron"
	"go.chromium.org/luci/server/gaeemulation"
	"go.chromium.org/luci/server/module"
	"go.chromium.org/luci/server/secrets"
	"go.chromium.org/luci/server/tq"

	"go.chromium.org/luci/swarming/server/botsrv"
)

func main() {
	modules := []module.Module{
		gaeemulation.NewModuleFromFlags(),
		cron.NewModuleFromFlags(),
		tq.NewModuleFromFlags(),
		secrets.NewModuleFromFlags(),
	}

	pollTokenSecret := flag.String(
		"poll-token-secret",
		"sm://poll-token-hmac",
		"A name of a secret with poll token HMAC key.",
	)

	server.Main(nil, modules, func(srv *server.Server) error {
		botSrv, err := botsrv.New(srv.Context, srv.Routes, *pollTokenSecret)
		if err != nil {
			return err
		}
		botSrv.InstallHandler("/swarming/api/v1/bot/rbe/ping", pingHandler)
		return nil
	})
}

func pingHandler(ctx context.Context, r *botsrv.Request) (botsrv.Response, error) {
	logging.Infof(ctx, "Dimensions: %v", r.Body.Dimensions)
	logging.Infof(ctx, "PollState: %v", r.PollState)
	return nil, nil
}
