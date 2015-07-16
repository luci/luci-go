// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package gae provides a fakable wrapped interface for the appengine SDK's
// APIs. This means that it's possible to mock all of the supported appengine
// APIs for testing (or potentially implement a different backend for them).
//
// gae currently provides interfaces for:
//   * RawDatastore (a less reflection-magic version of Datastore)
//   * Memcache
//   * TaskQueue
//   * GlobalInfo (e.g. Namespace, AppID, etc.)
//
// A package which implements the gae is expected to provide the following:
//   * A package function `Use(c context.Context, ...) context.Context`
//     This function is expected to add any information to c which is necessary
//     for the rest of its implementations to work. This may be something like
//     an `appengine.Context` or some connection information for an external
//     server. The `...` in the function signature may be any additional data
//     needed (or it may be empty).
//
//   * Partially-implemented interfaces should embed one of the dummy
//     implementations in the `dummy` subpackage which will panic with
//     an appropriate error for unimplemented methods.
//
// see "infra/gae/libs/gae/prod" for an appengine-backed implementation.
// see "infra/gae/libs/gae/memory" for an in-memory implementation.
//
// You will typically access one of the service interfaces in your code like:
//   // This is the 'production' code
//   func HTTPHandler(r *http.Request) {
//     c := prod.Use(appengine.NewContext(r))
//     CoolFunc(c)
//   }
//
//   // This is the 'testing' code
//   func TestCoolFunc(t *testing.T) {
//     c := memory.Use(context.Background())
//     CoolFunc(c)
//   }
//
//   func CoolFunc(c context.Context, ...) {
//     rds := gae.GetRDS(c)  // returns a RawDatastore object
//     mc := gae.GetMC(c)    // returns a Memcache object
//     // use them here
//
//     // don't pass rds/mc/etc. directly, pass the context instead.
//     SomeOtherFunction(c, ...)
//
//     // because you might need to:
//     rds.RunInTransaction(func (c context.Context) error {
//       SomeOtherFunction(c, ...)  // c contains transaction versions of everything
//     }, nil)
//   }
//
// RawDatastore struct serialization is provided by the `helper` subpackage. All
// supported struct types and interfaces are provided in this package, however.
// You can operate without any struct serizialization/reflection by exclusively
// using DSPropertyMap. A goon-style Datastore interface is also provided in the
// `helper` subpackage.
//
// Each service also supports "filters". Filters are proxy objects which have
// the same interface as the service they're filtering, and pass data through to
// the previous filter in the stack. Conceptually, a filtered version of, for
// example, the RawDatastore, could look like:
//    User
//    <mcache filter (attempts to use memcache as a cache for datastore)>
//    <count filter (keeps a conter for how many times each API is used)>
//    Memory RawDatastore (contains actual data)
//
// So GetRDS would return the full stack, and GetRDSUnfiltered would only
// return the bottom layer. In code, this would look like:
//   func HTTPHandler(r *http.Request) {
//     c := prod.Use(appengine.NewContext(r)) // production datastore
//     c, rawCount := count.FilterRDS()       // add count filter
//     c = mcache.FilterRDS(c)                // add mcache filter
//     c, userCount := count.FilterRDS()      // add another count filter
//   }
//
// Filters may or may not have state, it's up to the filter itself. In the case
// of the count filter, it returns its state from the Filter<Service> method,
// and the state can be observed to see how many times each API was invoked.
// Since filters stack, we can compare counts from rawCount versus userCount to
// see how many calls to the actual real datastore went through, v. how many
// went to memcache, for example.
package gae
