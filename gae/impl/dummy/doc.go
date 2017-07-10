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

// Package dummy provides panicking dummy implementations of all service
// Interfaces.
//
// In particular, this includes:
//   * datastore.Interface
//   * memcache.Interface
//   * taskqueue.Interface
//   * info.Interface
//   * module.Interface
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
