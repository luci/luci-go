// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package blobstore

// Key is a key for a blobstore blob.
//
// Blobstore is NOT YET supported by gae, but may be supported later. Its
// inclusion here is so that the RawDatastore can interact (and round-trip)
// correctly with other datastore API implementations.
type Key string
