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
	// BuildsAddAsChild allows to set ScheduleBuildRequest.ParentBuildId
	// when schduling new builds in a bucket.
	BuildsAddAsChild = realms.RegisterPermission("buildbucket.builds.addAsChild")
	// BuildsCreate allows to create new builds in a bucket.
	BuildsCreate = realms.RegisterPermission("buildbucket.builds.create")
	// BuildsGet allows to see all information about a build.
	BuildsGet = realms.RegisterPermission("buildbucket.builds.get")
	// BuildsGetLimited allows to see a limited set of information about a build.
	BuildsGetLimited = realms.RegisterPermission("buildbucket.builds.getLimited")
	// BuildsIncludeChild allows to update a build in a bucket to include a
	// new build as its child.
	BuildsIncludeChild = realms.RegisterPermission("buildbucket.builds.includeChild")
	// BuildsList allows to list and search builds in a bucket.
	// Note that the ability to search on certain fields may leak information that
	// would otherwise be redacted if the user only had this permission (and not
	// BuildsGet or BuildsGetLimited). Since there's nothing too sensitive in the
	// fields that can be used as predicates, and it's expensive to check permissions
	// upfront on every project/bucket being searched, we just live with this flaw
	// for now.
	BuildsList = realms.RegisterPermission("buildbucket.builds.list")
	// BuildsCancel allows to cancel a build.
	BuildsCancel = realms.RegisterPermission("buildbucket.builds.cancel")

	// BuildersGet allows to see details of a builder (but not its builds).
	BuildersGet = realms.RegisterPermission("buildbucket.builders.get")
	// BuildersList allows to list and search builders (but not builds).
	BuildersList = realms.RegisterPermission("buildbucket.builders.list")
	// BuildersSetHealth allows to set the health of a builder.
	BuildersSetHealth = realms.RegisterPermission("buildbucket.builders.setHealth")
)
