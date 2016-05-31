// Copyright 2015 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package blobstore

// Key is a key for a blobstore blob.
//
// Blobstore is NOT YET supported by gae, but may be supported later. Its
// inclusion here is so that the RawDatastore can interact (and round-trip)
// correctly with other datastore API implementations.
type Key string
