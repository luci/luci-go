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

// +build go1.13

package impl

import (
	"context"
	"flag"

	"go.chromium.org/luci/config/server/cfgmodule"
	"go.chromium.org/luci/server"
	"go.chromium.org/luci/server/dsmapper"
	"go.chromium.org/luci/server/gaeemulation"
	"go.chromium.org/luci/server/module"
	"go.chromium.org/luci/server/redisconn"
	"go.chromium.org/luci/server/secrets"
	"go.chromium.org/luci/server/tq"

	"go.chromium.org/luci/cipd/appengine/impl/admin"
	"go.chromium.org/luci/cipd/appengine/impl/cas"
	"go.chromium.org/luci/cipd/appengine/impl/model"
	"go.chromium.org/luci/cipd/appengine/impl/repo"
	"go.chromium.org/luci/cipd/appengine/impl/settings"
)

// Main initializes the core server modules and launches the callback.
func Main(extra []module.Module, cb func(srv *server.Server) error) {
	modules := append([]module.Module{
		cfgmodule.NewModuleFromFlags(),
		dsmapper.NewModuleFromFlags(),
		gaeemulation.NewModuleFromFlags(),
		redisconn.NewModuleFromFlags(),
		secrets.NewModuleFromFlags(),
		tq.NewModuleFromFlags(),
	}, extra...)

	s := &settings.Settings{}
	s.Register(flag.CommandLine)

	server.Main(nil, modules, func(srv *server.Server) error {
		if err := s.Validate(); err != nil {
			return err
		}
		ev, err := model.NewBigQueryEventLogger(srv.Context, srv.Options.CloudProject)
		if err != nil {
			return err
		}

		EventLogger = ev
		model.RegisterTasks(&tq.Default)
		model.EnqueueEventsImpl = model.NewEnqueueEventsCallback(&tq.Default, srv.Options.Prod)

		InternalCAS = cas.Internal(&tq.Default, func(context.Context) (*settings.Settings, error) { return s, nil })
		PublicCAS = cas.Public(InternalCAS)
		PublicRepo = repo.Public(InternalCAS, &tq.Default)
		AdminAPI = admin.AdminAPI(&dsmapper.Default)

		return cb(srv)
	})
}
