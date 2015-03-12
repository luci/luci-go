// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package context contains some simple interfaces wrapping goon's "Goon"
// object. In particular it:
//   * Provides granular typesafe interfaces for restricting the types of
//     operations one might do with the goon context.
//   * Makes goon's context compatible with infra's 'Logger' interface.
//   * Provides a typesafe escape hatch for grabbing the underlying Context
//     when necessary (for memcache, etc. interaction)
//     * TODO: encapsulate other appengine APIs nicely as well.
package context
