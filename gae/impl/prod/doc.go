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

// Package prod provides an implementation of infra/gae/libs/wrapper which
// backs to appengine.
package prod

// BUG(fyi): *datastore.Key objects have their AppID dropped when this package
//				   converts them internally to use with the underlying datastore. In
//				   practice this shouldn't be much of an issue, since you normally
//				   have no control over the AppID field of a Key anyway (aside from
//				   deserializing one directly from a proto).
