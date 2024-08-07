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

// Package server contains shared server initialisation logic for
// LUCI Source Index services.
package server

import (
	"go.chromium.org/luci/config/server/cfgmodule"
	"go.chromium.org/luci/grpc/prpc"
	luciserver "go.chromium.org/luci/server"
	"go.chromium.org/luci/server/cron"
	"go.chromium.org/luci/server/gaeemulation"
	"go.chromium.org/luci/server/module"
	spanmodule "go.chromium.org/luci/server/span"
	"go.chromium.org/luci/server/tq"

	"go.chromium.org/luci/source_index/internal/config"
	sourceindexpb "go.chromium.org/luci/source_index/proto/v1"
	"go.chromium.org/luci/source_index/rpc"
)

// Main implements the common entrypoint for all LUCI Source Index GAE services.
//
// Note, if changing responsibiltiy between services, please be aware
// that dispatch.yaml changes are not deployed atomically with service
// changes.
func Main(init func(srv *luciserver.Server) error) {
	// Use the same modules for all LUCI Source Index services.
	modules := []module.Module{
		cfgmodule.NewModuleFromFlags(),
		cron.NewModuleFromFlags(),
		gaeemulation.NewModuleFromFlags(), // Needed by cfgmodule.
		spanmodule.NewModuleFromFlags(nil),
		tq.NewModuleFromFlags(),
	}
	luciserver.Main(nil, modules, init)
}

// RegisterPRPCHandlers registers pPRC handlers.
func RegisterPRPCHandlers(srv *luciserver.Server) error {
	srv.ConfigurePRPC(func(s *prpc.Server) {
		s.AccessControl = prpc.AllowOriginAll
		// TODO(crbug/1082369): Remove this workaround once field masks can be decoded.
		s.HackFixFieldMasksForJSON = true
	})

	sourceindexpb.RegisterSourceIndexServer(srv, rpc.NewSourceIndexServer())
	return nil
}

// RegisterCrons registers cron handlers.
func RegisterCrons(srv *luciserver.Server) error {
	cron.RegisterHandler("update-config", config.Update)
	return nil
}

// RegisterPubSubHandlers registers pub/sub handlers.
func RegisterPubSubHandlers(srv *luciserver.Server) error {
	return nil
}

// RegisterTaskQueueHandlers registers task queue handlers.
func RegisterTaskQueueHandlers(srv *luciserver.Server) error {
	return nil
}
