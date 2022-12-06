// Copyright 2022 The LUCI Authors.
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

// Package provenance provides API definitions and simple libraries to interface
// with the APIs.
//
// It supports
// 1) All possible provenance information that are supported currently: API
// defined in api/snooperpb/v1.
// 2) api/spikepb/ids: Intrusion Detection System (IDS): API defined in
// api/speepb/v1 and api/spikepb/ids.
package provenance
