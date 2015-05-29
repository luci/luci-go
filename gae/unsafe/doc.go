// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package unsafe contains all the ugly unsafe adaptations that are necessary
// to impersonate the normal GAE SDK API. Currently it only contains
// implementation for accessing casID in memcache.Item.
package unsafe
