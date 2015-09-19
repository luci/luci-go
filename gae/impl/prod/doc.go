// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package prod provides an implementation of infra/gae/libs/wrapper which
// backs to appengine.
package prod

// BUG(fyi): *datastore.Key objects have their AppID dropped when this package
//				   converts them internally to use with the underlying datastore. In
//				   practice this shouldn't be much of an issue, since you normally
//				   have no control over the AppID field of a Key anyway (aside from
//				   deserializing one directly from a proto).
