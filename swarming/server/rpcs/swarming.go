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

// SwarmingServer implements Swarming gRPC service.
//
// It is a collection of various RPCs that didn't fit other services.
type SwarmingServer struct {
	apipb.UnimplementedSwarmingServer
}

// GetPermissions implements the corresponding RPC method.
func (srv *SwarmingServer) GetPermissions(ctx context.Context, req *apipb.PermissionsRequest) (*apipb.ClientPermissions, error) {
	state := State(ctx)

	var pools []string
	for _, tag := range req.Tags {
		if pool, ok := strings.CutPrefix(tag, "pool:"); ok {
			pools = append(pools, pool)
		}
	}

	var taskInfo acls.TaskAuthInfo
	if req.TaskId != "" {
		// TODO(vadimsh): Fetch task info.
	}

	var internalErr error

	checkPoolsPerm := func(perm realms.Permission) bool {
		if internalErr != nil {
			return false
		}
		var err error
		if len(pools) == 0 {
			err = state.ACL.CheckServerPerm(ctx, perm)
		} else {
			err = state.ACL.CheckAllPoolsPerm(ctx, pools, perm)
		}
		switch {
		case err == nil:
			return true
		case acls.IsDeniedErr(err):
			return false
		default:
			internalErr = err
			return false
		}
	}

	checkTaskPerm := func(perm realms.Permission) bool {
		if internalErr != nil {
			return false
		}
		var err error
		if req.TaskId == "" {
			err = state.ACL.CheckServerPerm(ctx, perm)
		} else {
			err = state.ACL.CheckTaskPerm(ctx, taskInfo, perm)
		}
		switch {
		case err == nil:
			return true
		case acls.IsDeniedErr(err):
			return false
		default:
			internalErr = err
			return false
		}
	}

	checkBotPerm := func(perm realms.Permission) bool {
		if internalErr != nil {
			return false
		}
		var err error
		if req.BotId == "" {
			err = state.ACL.CheckServerPerm(ctx, perm)
		} else {
			err = state.ACL.CheckBotPerm(ctx, req.BotId, perm)
		}
		switch {
		case err == nil:
			return true
		case acls.IsDeniedErr(err):
			return false
		default:
			internalErr = err
			return false
		}
	}

	resp := &apipb.ClientPermissions{
		DeleteBot:         checkBotPerm(acls.PermPoolsDeleteBot),
		DeleteBots:        checkPoolsPerm(acls.PermPoolsDeleteBot),
		TerminateBot:      checkBotPerm(acls.PermPoolsTerminateBot),
		GetConfigs:        false, // TODO
		PutConfigs:        false, // TODO
		CancelTask:        checkTaskPerm(acls.PermPoolsCancelTask),
		GetBootstrapToken: false, // TODO
		CancelTasks:       checkPoolsPerm(acls.PermPoolsCancelTask),
		ListBots:          []string{}, // TODO
		ListTasks:         []string{}, // TODO
	}

	if internalErr != nil {
		return nil, internalErr
	}
	return resp, nil
}