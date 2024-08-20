// Copyright 2023 The LUCI Authors.
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

// Package cfg implements Swarming server configuration logic.
//
// It consists of 3 components:
//
//  1. A cron job that periodically fetches all server configs at the most
//     recent revision, preprocesses them and stores the result in the local
//     datastore in a compact form.
//  2. A background goroutine running in every server process that periodically
//     polls configs stored in the datastore, unpacks and transforms them into
//     a form optimized for querying from RPC handlers. Its job is to maintain
//     in the local process memory the most recent config in its queryable form.
//  3. A validation logic used by LUCI Config callback to validate configs in
//     config presubmit checks and used by the cron job to double check new
//     configs are good.
//
// This package is also responsible for assembling the deployable bot package
// based on the server configuration and the bot package in CIPD.
package cfg
