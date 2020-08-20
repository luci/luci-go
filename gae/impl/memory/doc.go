// Copyright 2015 The LUCI Authors.
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

// Package memory provides an implementation of infra/gae/libs/wrapper which
// backs to local memory ONLY. This is useful for unittesting, and is also used
// for the nested-transaction filter implementation.
//
// Debug EnvVars
//
// To debug backend store memory access for a binary that uses this memory
// implementation, you may set the flag:
//   -luci.gae.store_trace_folder
// to `/path/to/some/folder`. Every memory store will be assigned a numbered
// file in that folder, and all access to that store will be logged to that
// file. Setting this to "-" will cause the trace information to dump to stdout.
package memory
