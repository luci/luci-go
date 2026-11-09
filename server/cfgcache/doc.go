// Copyright 2026 The LUCI Authors.
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

// Package cfgcache provides a LUCI server module and registration mechanism
// for validating, transforming, caching (in durable storage, like your
// database), and asynchronously refreshing (into memory) configuration via
// the LUCI Config service.
//
// Services declare configuration pipelines via [ServicePipeline] (for
// service-wide configs) or [ProjectPipeline] (for project-scoped configs) and
// register them in a [Registry].
//
// # Pipelines
//
// All pipelines work roughly in three parts: validation, cron, and in-memory.
//
// # The Validation part
//
// The LUCI Config service has a [global config] file which lists services
// (like yours!) which implement the `ConsumerServer` endpoints.
//
// When you register a pipeline, this module will install validation rules
// for each config file pattern, and these get registered with the `cfgmodule`
// LUCI Server module, which implements the ConsumerServer endpoint.
//
// When LUCI Server is asked to validate configuration, the TransformFunc
// function will be called with the [Snapshot] of files in the config set, and
// it can record warnings or errors which will be pushed back to the original
// requester.
//
// # The Cron part
//
// Your service will register a cron job entry (in cron.yaml) which routinely
// calls [Registry.IngestToStorage]. This will evaluate the current data in
// LUCI Config against what's stored inside each pipeline's `Store`. If anything
// is different in LUCI Config vs. what's in the pipeline's Store, the handler
// will call the pipeline's TransformFunc on the relevant snapshot of the config,
// and then save whatever the output of the TransformFunc is to the Store.
//
// # The In-memory part
//
// If you have the in-memory refresh feature turned on in the module,
// then each process will start a single background task to periodically
// refresh from the data in the Store, calling your ToMemory function to
// transform the stored value to a useful in-memory representation.
//
// Each pipeline Register method will return a handle to get the output of that
// pipeline's ToMemory function. This will return the freshest data, but will
// begin returning errors if the most recent refresh is too stale.
//
// # Basic Usage
//
// Declare pipeline definitions, and register them with a Registry (`nil` uses
// the [Default] registry). It can be useful for tests to put these definitions
// into a function so that you can directly pass which registry to use, but is
// not required.
//
//	package myconfigpkg
//
//	type InMemoryType struct {
//	}
//
//	var (
//		// A handle to get the unmarshaled MyCfgType proto message from
//		// `global.cfg` from inside the Transform function. This shouldn't be
//		// exported, since the Transform function is entirely contained by
//		// myconfigpkg.
//		globalCfgFile = &cfgcache.SingleFile[*MyCfgType]{Path: "global.cfg"}
//
//		// The GlobalHandle to get the current *InMemoryType.
//		//
//		// ServicePipeline defines the process of transforming some set of configs
//		// from a services/XXX config.Set into *MyStorageType in the store (e.g.
//		// datastore), and then into *InMemoryType when getting datta back via
//		// this handle.
//		GlobalHandle = (&cfgcache.ServicePipeline[*MyStorageType, *InMemoryType]{
//		  // ID is a unique name within the Registry. Used for logging, and also
//		  // to key the stored configs in e.g. datastore.
//			ID:               "my-global-cfg",
//
//			// The file or files you want to pull into the snapshot for Transform.
//			// Transform will be called as long as ONE of these files exists.
//			Files:            []cfgcache.File{globalCfgFile},
//
//			// Where should the transformed data be stored. Required.
//			// `dsstore` stores these to Google Cloud Datastore.
//			Store:            dsstore.Store[*MyStorageType]{},
//
//			// The version of the Transform function below. If the process of
//			// transforming `Files` to `MyStorageType` changes, increment this along
//			// with the updated algorithm.
//			TransformVersion: 1,
//
//			// Your function which validates `snap` (using the handles given to
//			// Files) and transforms it to *MyStorageType.
//			//
//			// This function should NOT have side effects, and will be called with
//			// all proposed config sets as well as actually committed config sets.
//			Transform: func(c *validation.Context, snap *cfgcache.Snapshot) (*MyStorageType, error) {
//				msg := globalCfgFile.Get(snap)
//				// do validation and transformation of `msg` to *MyStorageType
//				// This function runs in the cron handler.
//				return ...
//			},
//
//			// The minimum and maximum versions of Transform that ToMemory supports.
//			// This allows structured rollout of schema changes by incrementing
//			// ToMemoryMaxVersion (and adjusting ToMemory to handle the upgraded
//			// MyStorageType schema) and deploying this first, and then deploying
//			// the change to update TransformVersion+Transform.
//			//
//			// After this rolls out, you can then update ToMemoryMinVersion and
//			// remove the backwards compatibility support from ToMemory.
//			ToMemoryMinVersion: 1,
//			ToMemoryMaxVersion: 1,
//			ToMemory: func(ctx context.Context, cs config.Set, msg *MyStorageType) (*InMemoryType, error) {
//				// Consume the `msg` into *InMemoryType.
//				// This function is called inside all your server processes which
//				// need the config data.
//			},
//		}).Register(nil)
//	)
//
// # Server configuration
//
// Next, you have to wire this up to your LUCI Server. Do so like:
//
//	func Main(...) {
//	  modules := []module.Module{
//	    // If this process implements the cron portion of the pipeline, you
//	    // need the cron module.
//	    cron.NewModuleFromFlags(),
//	    cfgmodule.NewModuleFromFlags(),
//			// If you use `dsstore`, then you need `gaeemulation`.
//	    gaeemulation.NewModuleFromFlags(),
//	    cfgcache.NewModuleFromFlags(),
//	  }
//
//	  server.Main(nil, modules, func(srv *server.Server) error {
//	    coolServer := &CoolServer{
//	      myConfigHandle: myconfigpkg.GlobalHandle,
//	    }
//	    // register routes, etc.
//	    return nil
//	  })
//	}
//
// You must also register the cron job, e.g. on GAE add a cron.yaml entry:
//
//   - description: Imports latest configuration to datastore
//     url: /internal/cron/cfgcache
//     schedule: every 1 minutes
//
// After deploying and the cron successfully refreshes, your service processes
// should get a steady stream of configuration updates propagated to them which
// they can access via GlobalHandle.
//
// [global config]: https://config.luci.app/config_set/services/luci-config?path=services.cfg
package cfgcache
