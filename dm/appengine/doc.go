// Copyright 2016 The LUCI Authors.
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

// Package appengine provides the appengine service implementation for DM.
//
// This contains the following subpackages:
//   model - These objects are the datastore model objects for DM.
//   mutate - Tumble mutations for DM, a.k.a. DM's state machine. Each mutation
//     represents a single node in DM's state machine.
//   deps - The dependency management pRPC service.
//   frontend - The deployable appengine app. For Technical Reasons (tm), almost
//     zero code lives here, it just calls through to code in deps.
//   distributor - Definition of the Distributor interface, and implementations
//     (such as swarming_v1).
//
// For more information on DM itself, check out https://go.chromium.org/luci/wiki/Design-Documents
package appengine
