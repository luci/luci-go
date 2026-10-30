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
// for validating, transforming, caching, and subscribing to configuration via
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
// If you have the in-memory subscription feature turned on in the module,
// then each process will start a single background task to periodically
// refresh from the data in the Store, calling your ToMemory function to
// transform the stored value to a useful in-memory representation.
//
// Each pipeline Register method will return a handle to get the output of that
// pipeline's ToMemory function. This will return the freshest data, but will
// being returning errors if the most recent refresh is too stale.
//
// [global config]: https://config.luci.app/config_set/services/luci-config?path=services.cfg
package cfgcache
