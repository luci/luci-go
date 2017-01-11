// Copyright 2015 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

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
