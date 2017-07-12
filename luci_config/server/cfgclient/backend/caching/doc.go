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

// Package caching implements a config.Interface that uses a caching layer to
// store its configuration values.
//
// The Backend in this package is generic, and does not implement that actual
// caching interface. Instead, the user should pair it with a cache
// implementation by implementing its CacheGet method.
//
// Some example caches include:
//	- Process in-memory cache (proccache).
//	- memcache, or AppEngine's memcache service.
//	- A local storage cache (e.g., on-disk)
//	- A datastore cache.
package caching
