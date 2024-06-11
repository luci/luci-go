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

package rpcs

import (
	"context"
	"strings"

	"go.chromium.org/luci/server/auth/realms"

	apipb "go.chromium.org/luci/swarming/proto/api_v2"
	"go.chromium.org/luci/swarming/server/acls"
)

// GetPermissions implements the corresponding RPC method.
func (srv *SwarmingServer) GetPermissions(ctx context.Context, req *apipb.PermissionsRequest) (*apipb.ClientPermissions, error) {
	state := State(ctx)

	var pools []string
	for _, tag := range req.Tags {
		if pool, ok := strings.CutPrefix(tag, "pool:"); ok {
			pools = append(pools, pool)
		}
	}

	var internalErr error

	checkServerPerm := func(perm realms.Permission) bool {
		if internalErr != nil {
			return false
		}
		res := state.ACL.CheckServerPerm(ctx, perm)
		if res.InternalError {
			internalErr = res.ToGrpcErr()
		}
		return res.Permitted
	}

	checkPoolsPerm := func(perm realms.Permission) bool {
		if internalErr != nil {
			return false
		}
		if len(pools) == 0 {
			return checkServerPerm(perm)
		}
		res := state.ACL.CheckAllPoolsPerm(ctx, pools, perm)
		if res.InternalError {
			internalErr = res.ToGrpcErr()
		}
		return res.Permitted
	}

	checkTaskPerm := func(task acls.Task, perm realms.Permission) bool {
		if internalErr != nil {
			return false
		}
		res := state.ACL.CheckTaskPerm(ctx, task, perm)
		if res.InternalError {
			internalErr = res.ToGrpcErr()
		}
		return res.Permitted
	}

	checkBotPerm := func(perm realms.Permission) bool {
		if internalErr != nil {
			return false
		}
		if req.BotId == "" {
			return checkServerPerm(perm)
		}
		res := state.ACL.CheckBotPerm(ctx, req.BotId, perm)
		if res.InternalError {
			internalErr = res.ToGrpcErr()
		}
		return res.Permitted
	}

	poolsWithPerm := func(perm realms.Permission) []string {
		if internalErr != nil {
			return nil
		}
		filtered, err := state.ACL.FilterPoolsByPerm(ctx, state.Config.Pools(), perm)
		if err != nil {
			internalErr = err
			return nil
		}
		return filtered
	}

	resp := &apipb.ClientPermissions{
		DeleteBot:         checkBotPerm(acls.PermPoolsDeleteBot),
		DeleteBots:        checkPoolsPerm(acls.PermPoolsDeleteBot),
		TerminateBot:      checkBotPerm(acls.PermPoolsTerminateBot),
		GetBootstrapToken: checkServerPerm(acls.PermPoolsCreateBot),
		GetConfigs:        false, // deprecated and unused
		PutConfigs:        false, // deprecated and unused
		CancelTask:        false, // see below
		CancelTasks:       checkPoolsPerm(acls.PermPoolsCancelTask),
		ListBots:          poolsWithPerm(acls.PermPoolsListBots),
		ListTasks:         poolsWithPerm(acls.PermPoolsListTasks),
	}

	// If we are given a task ID, we need to check if we can cancel this specific
	// task. If there's no task ID, we need to check we can cancel *any* task in
	// the specified pools (and we already did this check, see CancelTasks). These
	// are two different permissions (PermTasksCancel vs PermPoolsCancelTask)
	// since mass-canceling needs to be more restricted compared to canceling
	// tasks one by one.
	if req.TaskId != "" {
		taskRequest, err := FetchTaskRequest(ctx, req.TaskId)
		if err != nil {
			return nil, err
		}
		resp.CancelTask = checkTaskPerm(taskRequest, acls.PermTasksCancel)
	} else {
		resp.CancelTask = resp.CancelTasks
	}

	if internalErr != nil {
		return nil, internalErr
	}
	return resp, nil
}
