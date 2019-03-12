// Copyright 2017 The LUCI Authors.
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

package gaemiddleware

import (
	"go.chromium.org/luci/common/tsmon/versions"
)

// Version is a semantic version of base luci-go GAE library.
//
// It is bumped whenever we add new features or fix important bugs. It is
// reported to monitoring as 'luci/components/version' string metric with
// 'component' field set to 'go.chromium.org/luci/appengine/gaemiddleware'.
//
// It allows to track what GAE apps use what version of the library, so it's
// easier to detect stale code running in production.
const Version = "1.1.0"

func init() {
	versions.Register("go.chromium.org/luci/appengine/gaemiddleware", Version)
}
