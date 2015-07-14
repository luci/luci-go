// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package dummy comtains dummy implementations of:
//   * gae.RawDatastore
//   * gae.Memcache
//   * gae.TaskQueue
//   * gae.GlobalInfo
//   * gae.DSQuery
//
// These dummy implementations panic with an appropriate error message when
// any of their methods are called. The message looks something like:
//   dummy: method Interface.Method is not implemented
//
// The dummy implementations are useful when implementing the interfaces
// themselves, or when implementing filters, since it allows your stub
// implementation to embed the dummy version and then just implement the methods
// that you care about.
package dummy
