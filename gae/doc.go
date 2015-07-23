// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package gae provides a fakable wrapped interface for the appengine SDK's
// APIs. This means that it's possible to mock all of the supported appengine
// APIs for testing (or potentially implement a different backend for them).
//
// Features
//
// gae currently provides interfaces for:
//   - RawDatastore (a less reflection-magic version of Datastore)
//   - Memcache
//   - TaskQueue
//   - GlobalInfo (e.g. Namespace, AppID, etc.)
//
// In addition, it provides a 'Datastore' service which adds all the reflection
// magic of the original SDK's datastore package on top of RawDatastore.
//
// Additional features include:
//   - true service interfaces (not package-level functions)
//   - methods don't need explicit context passed to them, increasing readability.
//   - service filters allow for composition of functionality. For example:
//     - transparent memcaching of datastore access
//     - transparent transaction buffering
//     - statistics-gathering shims
//     - deterministic and probabalistic API failure simulation
//     - transparent retries
//   - truly parallel in-memory testing implementation. No more need for
//     dev_appserver.py subprocesses :).
//
// Package Organization
//
// The gae library is organized into several subpackages:
//   - service/*   supported service definitions
//   - impl/*      implementations of the services
//   - filter/*    extra filter functionality for the services, agnostic to the
//                 underlying implementation.
//
// TLDR
//
// In production, do:
//
//   import (
//     "fmt"
//     "net/http"
//
//     "github.com/luci/gae/impl/prod"
//     "github.com/luci/gae/service/rawdatastore"
//     "golang.org/x/net/context"
//   )
//
//   func handler(w http.ResponseWriter, r *http.Request) {
//     c := prod.UseRequest(r)
//     // add production filters, etc. here
//     innerHandler(c, w)
//   }
//
//   func innerHandler(c context.Context, w http.ResponseWriter) {
//     rds := rawdatastore.Get(c)
//     data := rawdatastore.PropertyMap{
//       "Value": {rawdatastore.MkProperty("hello")},
//     }
//     newKey, err := rds.Put(rds.NewKey("Kind", "", 0, nil), data)
//     if err != nil {
//       http.Error(w, err.String(), http.StatusInternalServerError)
//     }
//     fmt.Fprintf(w, "I wrote: %s", newKey)
//   }
//
// And in your test do:
//
//   import (
//     "testing"
//     "fmt"
//     "net/http"
//
//     "github.com/luci/gae/impl/memory"
//     "github.com/luci/gae/service/rawdatastore"
//     "golang.org/x/net/context"
//   )
//
//   func TestHandler(t *testing.T) {
//     t.Parallel()
//     c := memory.Use(context.Background())
//     // use rawdatastore here to monkey with the database, install
//     // testing filters like featureBreaker to test error conditions in
//     // innerHandler, etc.
//     innerHandler(c, ...)
//   }
//
// Service Definitions
//
// A service defintion lives under the `service` subfolder, and defines the
// user-facing interface for a service. Each service has a few common types and
// functions. Common types are:
//
//    service.Interface - the main service interface
//    service.Testable  - any additional methods that a 'testing' implementation
//                        should provide. It's expected that tests will cast
//                        the Interface from Get() to Testable in order to
//                        access these methods.
//    service.Factory   - a function returning an Interface
//    service.Filter    - a function returning a new Interface based on the
//                        previous filtered interface.
//
// And common functions are:
//    service.Get        - Retrieve the current, filtered Interface
//                         implementation from the context. This is the most
//                         frequently used service function by far.
//    service.AddFilters - adds one or more Filters to the context.
//    service.SetFactory - adds a Factory to the context
//    service.Set        - adds an implementation of Interface to the context
//                         (shorthand for SetFactory, useful for testing)
//
// Implementations
//
// The impl subdirectory contains a couple different service implementations,
// depending on your needs.
//
// 'prod' is the production (e.g. real appengine-backed) implementation. It
// calls through to the original appengine SDK.
//
// 'memory' is a truly parallel in-memory testing implementation. It should
// be functionally the same as the production appengine services, implementing
// many of the real-world quirks of the actual services. It also implements
// the services' Testable interface, for those services which define those
// interfaces.
//
// Usage
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
// RawDatastore struct serialization is provided by the `rawdatastore`
// subpackage. All supported struct types and interfaces are provided in this
// package, however.  You can operate without any struct
// serizialization/reflection by exclusively using PropertyMap. A goon-style
// Datastore interface is also provided in the `datastore` service package.
//
// Filters
//
// Each service also supports "filters". Filters are proxy objects which have
// the same interface as the service they're filtering, and pass data through to
// the previous filter in the stack. Conceptually, a filtered version of, for
// example, the RawDatastore, could look like:
//    User code
//    <count filter (counts how many times each API is called by the user)>
//    <mcache filter (attempts to use memcache as a cache for rawdatastore)>
//    <count filter (counts how many times each API is actually hit)>
//    memory RawDatastore implementation
//
// So rawdatastore.Get would return the full stack. In code, this would look
// like:
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
// see how many calls to the actual real datastore went through, vs. how many
// went to memcache, for example.
package gae
