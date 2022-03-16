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

// Package bbperms contains a list of registered buildbucket Realm permissions.
package bbperms

import "go.chromium.org/luci/server/auth/realms"

var (
	// BuildsAdd allows to schedule new builds in a bucket.
	BuildsAdd = realms.RegisterPermission("buildbucket.builds.add")
	// BuildsGet allows to see all information about a build.
	BuildsGet = realms.RegisterPermission("buildbucket.builds.get")
	// BuildsList allows to list and search builds in a bucket.
	BuildsList = realms.RegisterPermission("buildbucket.builds.list")
	// BuildsCancel allows to cancel a build.
	BuildsCancel = realms.RegisterPermission("buildbucket.builds.cancel")

	// BuildersGet allows to see details of a builder (but not its builds).
	BuildersGet = realms.RegisterPermission("buildbucket.builders.get")
	// BuildersList allows to list and search builders (but not builds).
	BuildersList = realms.RegisterPermission("buildbucket.builders.list")
)
