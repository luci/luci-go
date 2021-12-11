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

package service

import (
	"context"
	"flag"

	// This import changes the behavior of datastore within this application
	// to make datastore.Get always zero-out all struct fields prior to
	// populating them from datastore.
	_ "go.chromium.org/luci/gae/service/datastore/crbug1242998safeget"

	"go.chromium.org/luci/server"
	"go.chromium.org/luci/server/gaeemulation"
	"go.chromium.org/luci/server/module"

	logdog "go.chromium.org/luci/logdog/api/endpoints/coordinator/services/v1"
	"go.chromium.org/luci/logdog/common/storage"
	"go.chromium.org/luci/logdog/common/storage/bigtable"
	"go.chromium.org/luci/logdog/server/config"
)

// Implementations contains some preconfigured Logdog subsystem clients.
type Implementations struct {
	Storage     storage.Storage       // the intermediate storage
	Coordinator logdog.ServicesClient // the bundling coordinator client
}

// MainCfg are global configuration options for `Main`.
//
// The settings here are tied to the code-to-be-executed, rather than the
// the execution environment for the code.
type MainCfg struct {
	// The AppProfile for the global BigTable client for this service.
	//
	// If empty, the default application profile will be used.
	BigTableAppProfile string
}

// Main initializes and runs a logdog microservice process.
func Main(cfg MainCfg, init func(srv *server.Server, impl *Implementations) error) {
	modules := []module.Module{
		gaeemulation.NewModuleFromFlags(), // for fetching LUCI project configs
	}

	coordFlags := coordinatorFlags{}
	coordFlags.register(flag.CommandLine)

	storageFlags := bigtable.Flags{
		AppProfile: cfg.BigTableAppProfile,
	}
	storageFlags.Register(flag.CommandLine)

	server.Main(nil, modules, func(srv *server.Server) error {
		if err := coordFlags.validate(); err != nil {
			return err
		}
		if err := storageFlags.Validate(); err != nil {
			return err
		}

		// Add an in-memory config caching to avoid hitting datastore all the time.
		srv.Context = config.WithStore(srv.Context, &config.Store{})

		// Initialize our Storage.
		st, err := bigtable.StorageFromFlags(srv.Context, &storageFlags)
		if err != nil {
			return err
		}
		srv.RegisterCleanup(func(context.Context) { st.Close() })

		// Initialize a Coordinator client that bundles requests together.
		coordClient, err := coordinator(srv.Context, &coordFlags)
		if err != nil {
			return err
		}
		srv.RegisterCleanup(func(context.Context) { coordClient.Flush() })

		// Let the particular microservice initialize itself.
		return init(srv, &Implementations{
			Storage:     st,
			Coordinator: coordClient,
		})
	})
}
