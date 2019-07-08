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

// Package server implements authentication for inbound HTTP requests on GAE.
// It provides adapters for GAE Users and OAuth2 APIs to make them usable by
// server/auth package.
//
// It also provides GAE-specific implementation of some other interface used
// by server/auth package, such as SessionStore.
//
// By default, gaeauth must have its handlers installed into the "default"
// AppEngine module, and must be running on an instance with read/write
// datastore access.
package server
