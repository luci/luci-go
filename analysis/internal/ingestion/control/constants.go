// Copyright 2024 The LUCI Authors.
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

package control

import (
	"fmt"

	"go.chromium.org/luci/resultdb/pbutil"
)

// BuildInvocationName returns the invocation name corresponding to a
// buildbucket build.
// The pattern is originally defined here:
// https://source.chromium.org/chromium/infra/infra/+/main:appengine/cr-buildbucket/resultdb.py;l=75?q=build-%20resultdb&type=cs
func BuildInvocationName(buildID int64) string {
	return pbutil.InvocationName(fmt.Sprintf("build-%v", buildID))
}
