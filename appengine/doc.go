// Copyright 2021 The LUCI Authors.
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

// Package appengine contains helper packages targeting GAE first-gen runtime.
//
// This entire package tree is DEPRECATED and many package won't even work at
// all on GAE second-gen runtime or on GKE.
//
// See packages under go.chromium.org/luci/server for replacements that work
// on GAE second-gen and outside of GAE.
package appengine
