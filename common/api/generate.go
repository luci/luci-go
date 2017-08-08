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

//go:generate go install go.chromium.org/luci/tools/cmd/apigen
//go:generate apigen -api-subproject "luci_config" -service "https://luci-config.appspot.com" -api "config:v1"
//go:generate apigen -api-subproject "swarming" -service "https://chromium-swarm.appspot.com" -api "swarming:v1"
//go:generate apigen -api-subproject "isolate" -service "https://isolateserver.appspot.com" -api "isolateservice:v1"
//go:generate apigen -api-subproject "buildbucket" -service "https://cr-buildbucket.appspot.com" -api "buildbucket:v1" -api "swarmbucket:v1"

package api
