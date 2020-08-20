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

// Package meta contains some methods for interacting with GAE's metadata APIs.
// It only contains an implementation for those metadata APIs we've needed so
// far, but should be extended to support new ones in the case that we use them.
//
// See metadata docs: https://cloud.google.com/appengine/docs/python/datastore/metadataentityclasses#EntityGroup
package meta
