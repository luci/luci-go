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

// Package impl instantiates the full implementation of the CIPD backend
// services.
//
// It is imported by GAE's frontend and backend modules that expose appropriate
// bits and pieces over pRPC and HTTP.
package impl

import (
	"flag"

	"go.chromium.org/luci/config/server/cfgmodule"
	"go.chromium.org/luci/server"
	"go.chromium.org/luci/server/bqlog"
	"go.chromium.org/luci/server/cron"
	"go.chromium.org/luci/server/dsmapper"
	"go.chromium.org/luci/server/gaeemulation"
	"go.chromium.org/luci/server/module"
	"go.chromium.org/luci/server/redisconn"
	"go.chromium.org/luci/server/secrets"
	"go.chromium.org/luci/server/tq"

	adminapi "go.chromium.org/luci/cipd/api/admin/v1"
	cipdapi "go.chromium.org/luci/cipd/api/cipd/v1"
	"go.chromium.org/luci/cipd/appengine/impl/admin"
	"go.chromium.org/luci/cipd/appengine/impl/cas"
	"go.chromium.org/luci/cipd/appengine/impl/model"
	"go.chromium.org/luci/cipd/appengine/impl/repo"
	"go.chromium.org/luci/cipd/appengine/impl/settings"
	"go.chromium.org/luci/cipd/appengine/impl/vsa"
)

// Services is a collection of initialized CIPD backend service subsystems.
type Services struct {
	// InternalCAS is non-ACLed implementation of cas.StorageService to be used
	// only from within the backend code itself.
	InternalCAS cas.StorageServer

	// PublicCAS is ACL-protected implementation of cas.StorageServer that can be
	// exposed as a public API.
	PublicCAS cipdapi.StorageServer

	// PublicRepo is ACL-protected implementation of cipd.RepositoryServer that
	// can be exposed as a public API.
	PublicRepo repo.Server

	// AdminAPI is ACL-protected implementation of cipd.AdminServer that can be
	// exposed as an external API to be used by administrators.
	AdminAPI adminapi.AdminServer

	// EventLogger can flush events to BigQuery.
	EventLogger *model.BigQueryEventLogger
}

// Main initializes the core server modules and launches the callback.
func Main(extra []module.Module, cb func(srv *server.Server, svc *Services) error) {
	modules := append([]module.Module{
		bqlog.NewModuleFromFlags(),
		cfgmodule.NewModuleFromFlags(),
		cron.NewModuleFromFlags(),
		dsmapper.NewModuleFromFlags(),
		gaeemulation.NewModuleFromFlags(),
		redisconn.NewModuleFromFlags(),
		secrets.NewModuleFromFlags(),
		tq.NewModuleFromFlags(),
	}, extra...)

	s := &settings.Settings{}
	s.Register(flag.CommandLine)

	b := vsa.NewClient()
	b.Register(flag.CommandLine)

	server.Main(nil, modules, func(srv *server.Server) error {
		if err := s.Validate(); err != nil {
			return err
		}
		if err := b.Init(srv.Context); err != nil {
			return err
		}

		ev, err := model.NewBigQueryEventLogger(srv.Context, srv.Options.CloudProject)
		if err != nil {
			return err
		}
		srv.Context = ev.RegisterSink(srv.Context, &tq.Default, srv.Options.Prod)

		internalCAS := cas.Internal(&tq.Default, &bqlog.Default, s, &srv.Options)
		return cb(srv, &Services{
			InternalCAS: internalCAS,
			PublicCAS:   cas.Public(internalCAS),
			PublicRepo:  repo.Public(internalCAS, &tq.Default, b),
			AdminAPI:    admin.AdminAPI(&dsmapper.Default),
			EventLogger: ev,
		})
	})
}
