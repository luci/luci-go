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

// Binary orchestrator provides a service to create/manage/view builds.
// It is broadly comparable to Buildbucket although not intended to match
// it's API or functionality completely.
// For example, orchestrator separately handles scheduling and executing work.
// Buildbucket calling Swarming causes builds to be scheduled AND executed.
package main
