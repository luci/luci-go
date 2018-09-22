// Copyright 2018 The LUCI Authors.
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

// Package mapper implements a simple Datastore mapper.
//
// It provides a way to apply some function to all datastore entities of some
// particular kind, in parallel, but with bounded concurrency (to avoid burning
// through all CPU/Datastore quota at once). This may be useful when examining
// or mutating large amounts of datastore entities.
//
// It works by splitting the range of keys into N shards, and launching N worker
// tasks that each sequentially processes a shard assigned to it, slice by
// slice.
package mapper
