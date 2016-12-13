// Copyright 2016 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

// Package datastorecache implements a managed versatile datastore caching.
// Each datastorecache client obtains its own Cache instance for its specific
// cache type. That cache instance is given a "name" and managed independently
// from other cache types.
//
// Each cache instance additionally requires the configuration of a management
// cron task to handle that specific cache "name". This instance will require a
// handler to be registered for that cache name and a cron task configured to
// periodically hit that handler.
//
// Periodically, the management cron task will iterate through all cache entries
// and use their Handler to refresh those that are near expiration and delete
// those that haven't been used in a while.
//
// Manager Task
//
// The manager task runs periodically, triggered by cron. Each pass, it queries
// for all currently-registered cache entries and chooses an action:
//	- If the entry hasn't been accessed in a while, it will be deleted.
//	- If the entry references a Handler that isn't registered, it will be
//	  deleted eventually.
//	- If the entry's "last refresh" timestamp is past its refresh period, it
//	  will be refreshed via its Handler.
//	- Otherwise, the entry is left alone for the next pass.
//
// TODO: Each datastorecache cache is designed to be shard-able if the manager
// refresh ever becomes too burdensome for a single cron session. However,
// sharding isn't currently implemented.
package datastorecache
