// Copyright 2020 The LUCI Authors.
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

// Package ttq defines public types in use by implementations of the
// transactional task enqueueing on Google Cloud.
//
// Applications should use a specific implementation:
//  * http://go.chromium.org/luci/ttq/ttqdatastore
//  * http://go.chromium.org/luci/ttq/ttqspanner
package ttq
