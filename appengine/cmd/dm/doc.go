// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package dm provides the implementation for the Dungeon Master (DM)
// distributed dependency scheduling service. It is split into the following
// subpackages:
//   types - Types common to the other packages.
//   model - These objects are the datastore model objects for DM.
//   mutate - Tumble mutations for DM, a.k.a. DM's state machine. Each mutation
//     represents a single node in DM's state machine.
//   display - Objects that are returned as JSON from the various service
//     endpoints. These objects are intended to be consumed by machine clients.
//   service - The actual Cloud Endpoints service.
//   frontend - The deployable appengine app. For Technical Reasons (tm), almost
//     zero code lives here, it just calls through to code in service.
//
// For more information on DM itself, check out https://github.com/luci/luci-go/wiki/Design-Documents
package dm
