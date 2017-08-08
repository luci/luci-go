// Copyright 2015 The LUCI Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package gae provides a fakable wrapped interface for the appengine SDK's
// APIs. This means that it's possible to mock all of the supported appengine
// APIs for testing (or potentially implement a different backend for them).
//
// Features
//
// gae currently provides interfaces for:
//   - Datastore
//   - Memcache
//   - TaskQueue
//   - Info (e.g. Namespace, AppID, etc.)
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
//   - Separate service and user-facing interfaces
//     - Allows easier filter and service implementation while retaining the
//       benefits of a user-friendly interface.
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
//     "go.chromium.org/gae/impl/prod"
//     "go.chromium.org/gae/service/datastore"
//     "golang.org/x/net/context"
//   )
//
//   func handler(w http.ResponseWriter, r *http.Request) {
//     c := prod.UseRequest(r)
//     // add production filters, etc. here
//     innerHandler(c, w)
//   }
//
//   type CoolStruct struct {
//     ID `gae:"$id"`
//
//     Value string
//   }
//
//   func innerHandler(c context.Context, w http.ResponseWriter) {
//     obj := &CoolStruct{Value: "hello"}
//     if err := rds.Put(c, obj); err != nil {
//       http.Error(w, err.String(), http.StatusInternalServerError)
//     }
//     fmt.Fprintf(w, "I wrote: %s", ds.KeyForObj(obj))
//   }
//
// And in your test do:
//
//   import (
//     "testing"
//     "fmt"
//     "net/http"
//
//     "go.chromium.org/gae/impl/memory"
//     "go.chromium.org/gae/service/datastore"
//     "golang.org/x/net/context"
//   )
//
//   func TestHandler(t *testing.T) {
//     t.Parallel()
//     c := memory.Use(context.Background())
//     // use datastore here to monkey with the database, install testing
//     // filters like featureBreaker to test error conditions in innerHandler,
//     // etc.
//     innerHandler(c, ...)
//   }
//
// Service Definitions
//
// A service defintion lives under the `service` subfolder, and defines the
// user-facing interface for a service. Each service has a few common types and
// functions. Common types are:
//
//    service.Interface    - the main user-friendly service interface.
//
//    service.RawInterface - the internal service interface used by service
//                           and filter implementations. Note that some services
//                           like Info don't distinguish between the service
//                           interface and the user interface. This interface is
//                           typically a bit lower level than Interface and
//                           lacks convenience methods.
//
//    service.Testable     - any additional methods that a 'testing'
//                           implementation should provide. This can be accessed
//                           via the Testable method on RawInterface. If the
//                           current implementation is not testable, it will
//                           return nil. This is only meant to be accessed when
//                           testing.
//
//    service.RawFactory   - a function returning a RawInterface
//
//    service.RawFilter    - a function returning a new RawInterface based on
//                           the previous filtered interface. Filters chain
//                           together to allow behavioral service features
//                           without needing to agument the underlying service
//                           implementations directly.
//
// And common functions are:
//    service.GetRaw        - Retrieve the current, filtered RawInterface
//                            implementation from the context. This is less
//                            frequently used, but can be useful if you want to
//                            avoid some of the overhead of the user-friendly
//                            Interface, which can do sometimes-unnecessary amounts
//                            of reflection or allocation. The RawInterface and
//                            Interface for a service are fully interchangable and
//                            usage of them can be freely mixed in an application.
//
//    service.AddRawFilters - adds one or more RawFilters to the context.
//
//    service.SetRawFactory - adds a RawFactory to the context
//
//    service.SetRaw        - adds an implementation of RawInterface to the context
//                            (shorthand for SetRawFactory, useful for testing)
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
// 'dummy' provides a bunch of implementations of the various RawInterfaces.
// These implementations just panic with an appropriate message, depending on
// which API method was called. They're useful to embed in filter or service
// implementations as stubs while you're implementing the filter.
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
//     SomeOtherFunction(c, ...)
//
//     // because you might need to:
//     ds.RunInTransaction(c, func (c context.Context) error {
//       SomeOtherFunction(c, ...)  // c contains transactional versions of everything
//     }, nil)
//   }
//
// Filters
//
// Each service also supports "filters". Filters are proxy objects which have
// the same interface as the service they're filtering, and pass data through to
// the previous filter in the stack. Conceptually, a filtered version of, for
// example, the Datastore, could look like:
//    User code
//    <count filter (counts how many times each API is called by the user)>
//    <dscache filter (attempts to use memcache as a cache for datastore)>
//    <count filter (counts how many times each API is actually hit)>
//    memory datastore.RawInterface implementation
//
// Filters may or may not have state, it's up to the filter itself. In the case
// of the count filter, it returns its state from the Filter<Service> method,
// and the state can be observed to see how many times each API was invoked.
// Since filters stack, we can compare counts from rawCount versus userCount to
// see how many calls to the actual real datastore went through, vs. how many
// went to memcache, for example.
//
// Note that Filters apply only to the service.RawInterface. All implementations
// of service.Interface boil down to calls to service.RawInterface methods, but
// it's possible that bad calls to the service.Interface methods could return
// an error before ever reaching the filters or service implementation.
package gae
