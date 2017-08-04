// Copyright 2017 The LUCI Authors.
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

// Package readonly implements a filter that enforces read-only accesses to
// services.
//
// This can be useful in hybrid environments where one cluster wants to read
// from a cache-backed datastore, but cannot modify the cache, so reads are safe
// and direct, but writes would create a state where the cached values are
// invalid.
//
// This happens when mixing AppEngine datastore/memcache with Cloud Datastore
// readers.
package readonly
