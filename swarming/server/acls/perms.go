// Copyright 2023 The LUCI Authors.
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

package acls

import (
	"go.chromium.org/luci/server/auth/realms"
)

var (
	// PermTasksGet controls who can see the task known by its ID.
	PermTasksGet = realms.RegisterPermission("swarming.tasks.get")
	// PermTasksCancel controls who can cancel the task known by its ID.
	PermTasksCancel = realms.RegisterPermission("swarming.tasks.cancel")
	// PermTasksActAs controls what service accounts the task can run as.
	PermTasksActAs = realms.RegisterPermission("swarming.tasks.actAs")
	// PermTasksCreateInRealm controls who can associate tasks with the realm.
	PermTasksCreateInRealm = realms.RegisterPermission("swarming.tasks.createInRealm")
	// PermPoolsListBots controls who can see bots in the pool.
	PermPoolsListBots = realms.RegisterPermission("swarming.pools.listBots")
	// PermPoolsListTasks controls who can see tasks in the pool.
	PermPoolsListTasks = realms.RegisterPermission("swarming.pools.listTasks")
	// PermPoolsCreateBot controls who can add bots to the pool.
	PermPoolsCreateBot = realms.RegisterPermission("swarming.pools.createBot")
	// PermPoolsDeleteBot controls who can remove bots from the pool.
	PermPoolsDeleteBot = realms.RegisterPermission("swarming.pools.deleteBot")
	// PermPoolsTerminateBot controls who can terminate bots in the pool.
	PermPoolsTerminateBot = realms.RegisterPermission("swarming.pools.terminateBot")
	// PermPoolsCreateTask controls who can submit tasks into the pool.
	PermPoolsCreateTask = realms.RegisterPermission("swarming.pools.createTask")
	// PermPoolsCancelTask controls who can cancel tasks in the pool.
	PermPoolsCancelTask = realms.RegisterPermission("swarming.pools.cancelTask")
)
