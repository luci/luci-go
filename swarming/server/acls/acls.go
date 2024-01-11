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

// Package acls implements access control checks for Swarming APIs.
package acls

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authdb"
	"go.chromium.org/luci/server/auth/realms"

	"go.chromium.org/luci/swarming/server/cfg"
)

// Checker knows how to check Swarming ACLs inside a single RPC call.
//
// Its lifetime is scoped to a single request. It caches checks done within this
// request to avoid doing redundant work. This cache never expires, thus it is
// important to **drop** this checker once the request is finishes, to avoid
// using stale cached data.
//
// Resources are organized hierarchically: Server => Pool => Task and Bot.
// Permissions can potentially be granted on any level of this hierarchy, e.g.
// permissions granted on a pool level apply to all tasks and bots the belong
// to this pool.
//
// RPCs concerned with a specific task or bot should just check permission on
// the task/bot layer using CheckTaskPerm/CheckBotPerm.
//
// RPCs that do listing or other operations that touch many tasks and bots may
// use CheckPoolPerm and CheckServerPerm to do "prefiltering". They should also
// be used if RPCs results are an aggregation over a pool, and thus explicitly
// require pool-level permissions.
type Checker struct {
	cfg    *cfg.Config       // swarming config
	db     authdb.DB         // auth DB with groups and permissions
	caller identity.Identity // authenticated identity of the caller
}

// CheckResult is returned by all Checker methods.
type CheckResult struct {
	// Permitted is true if the permission check passed successfully.
	//
	// It is false if the caller doesn't have the requested permission or the
	// check itself failed. Look at InternalError field to distinguish these cases
	// if necessary.
	//
	// Use ToGrpcErr to convert a failure to a gRPC error. Note that CheckResult
	// explicitly **does not** implement `error` interface to make sure callers
	// are aware they need to return the gRPC error without any additional
	// wrapping via `return nil, res.ToGrpcErr()`.
	Permitted bool

	// InternalError indicates there were some internal error checking ACLs.
	//
	// An internal error means the check itself failed due to internal errors,
	// such as a timeout contacting the backend. This should abort the request
	// handler ASAP with Internal gRPC error. Use ToGrpcErr to get such error.
	//
	// If both Permitted and InternalError are false, it means the caller has no
	// requested permission. Use ToGrpcErr to get the error that must be returned
	// to the caller in that case.
	InternalError bool

	// err is a gRPC error to return.
	err error
}

// ToGrpcErr converts this failure to a gRPC error.
//
// To avoid accidentally leaking private information or implementation details,
// this error should be returned to the gRPC caller as is, without any
// additional wrapping. It is constructed to have all necessary information
// about the call already.
//
// If the check succeeded and the access is permitted, returns nil.
func (res *CheckResult) ToGrpcErr() error {
	switch {
	case res.InternalError:
		return status.Errorf(codes.Internal, "internal error when checking permissions")
	case res.Permitted:
		return nil
	case res.err == nil:
		panic("err is not populated")
	default:
		return res.err
	}
}

// TaskAuthInfo are properties of a task that affect who can access it.
//
// Extracted either from TaskRequest or from TaskResultSummary.
type TaskAuthInfo struct {
	// TaskID is ID of the task. Only for error messages and logs!
	TaskID string
	// Realm is the realm the task belongs to, as "<project>:<realm>" string.
	Realm string
	// Pool is task's pool extracted from "pool" dimension.
	Pool string
	// BotID is a bot the task is targeting via "id" dimension or "" if none.
	BotID string
	// Submitter is whoever submitted the task.
	Submitter identity.Identity
}

// NewChecker constructs an ACL checker that uses the given config snapshot.
func NewChecker(ctx context.Context, cfg *cfg.Config) *Checker {
	state := auth.GetState(ctx)
	return &Checker{
		cfg:    cfg,
		db:     state.DB(),
		caller: state.User().Identity,
	}
}

// CheckServerPerm checks if the caller has a permission on a server level.
//
// Having a permission on a server level means it applies to all pools, tasks
// and bots in this instance of Swarming. Server level permissions are defined
// via "auth { ... }" stanza with group names in the server's settings.cfg.
func (chk *Checker) CheckServerPerm(ctx context.Context, perm realms.Permission) CheckResult {
	serverGroups := chk.cfg.Settings().Auth

	var allowedGroups []string

	switch perm {
	case PermTasksGet, PermPoolsListTasks:
		allowedGroups = []string{
			serverGroups.ViewAllTasksGroup,
			serverGroups.PrivilegedUsersGroup,
			serverGroups.AdminsGroup,
		}

	case PermPoolsListBots:
		allowedGroups = []string{
			serverGroups.ViewAllBotsGroup,
			serverGroups.PrivilegedUsersGroup,
			serverGroups.AdminsGroup,
		}

	case PermPoolsCreateBot:
		allowedGroups = []string{
			serverGroups.BotBootstrapGroup,
			serverGroups.AdminsGroup,
		}

	case PermTasksCancel, PermPoolsCancelTask, PermPoolsDeleteBot, PermPoolsTerminateBot:
		allowedGroups = []string{
			serverGroups.AdminsGroup,
		}
	}

	if len(allowedGroups) != 0 {
		switch yes, err := chk.db.IsMember(ctx, chk.caller, allowedGroups); {
		case err != nil:
			logging.Errorf(ctx, "Error when checking groups: %s", err)
			return CheckResult{InternalError: true}
		case yes:
			return CheckResult{Permitted: true}
		}
	}

	return CheckResult{
		err: status.Errorf(
			codes.PermissionDenied,
			"the caller %q doesn't have server-level permission %q",
			chk.caller, perm),
	}
}

// CheckPoolPerm checks if the caller has a permission on a pool level.
//
// Having a permission on a pool level means it applies for all tasks and bots
// in that pool. CheckPoolPerm implicitly calls CheckServerPerm.
func (chk *Checker) CheckPoolPerm(ctx context.Context, pool string, perm realms.Permission) CheckResult {
	// If have a server-level permission, no need to check the pool. Server-level
	// permissions are also the only way to deal with deleted pools.
	if res := chk.CheckServerPerm(ctx, perm); res.Permitted || res.InternalError {
		return res
	}

	if cfg := chk.cfg.Pool(pool); cfg != nil {
		switch yes, err := chk.db.HasPermission(ctx, chk.caller, perm, cfg.Realm, nil); {
		case err != nil:
			logging.Errorf(ctx, "Error in HasPermission(%q, %q): %s", perm, cfg.Realm, err)
			return CheckResult{InternalError: true}
		case yes:
			return CheckResult{Permitted: true}
		}
	}

	// TODO(vadimsh): Make the error message more informative.
	return CheckResult{
		err: status.Errorf(
			codes.PermissionDenied,
			"the caller %q doesn't have permission %q in the pool %q or the pool doesn't exist",
			chk.caller, perm, pool),
	}
}

// FilterPoolsByPerm filters the list of pools keeping only ones in which the
// caller has the permission.
//
// If the caller doesn't have the permission in any of the pools, returns nil
// slice and no error. Returns a gRPC status error if the check failed due to
// some internal issues.
func (chk *Checker) FilterPoolsByPerm(ctx context.Context, pools []string, perm realms.Permission) ([]string, error) {
	// If have a server-level permission, no need to check individual pools.
	switch res := chk.CheckServerPerm(ctx, perm); {
	case res.InternalError:
		return nil, res.ToGrpcErr()
	case res.Permitted:
		return pools, nil
	}

	var filtered []string

	ok := chk.visitRealms(ctx, pools, perm, func(pool string, allowed bool) bool {
		if allowed {
			filtered = append(filtered, pool)
		}
		return true
	})

	if !ok {
		return nil, (&CheckResult{InternalError: true}).ToGrpcErr()
	}
	return filtered, nil
}

// CheckAllPoolsPerm checks if the caller has a permission in *all* given pools.
//
// The list of pools must not be empty. Panics if it is.
func (chk *Checker) CheckAllPoolsPerm(ctx context.Context, pools []string, perm realms.Permission) CheckResult {
	switch len(pools) {
	case 0:
		panic("empty list of pools in CheckAllPoolsPerm")
	case 1:
		// Use a single pool check for better error messages.
		return chk.CheckPoolPerm(ctx, pools[0], perm)
	}

	// If have a server-level permission, no need to check individual pools.
	if res := chk.CheckServerPerm(ctx, perm); res.Permitted || res.InternalError {
		return res
	}

	allAllowed := true

	ok := chk.visitRealms(ctx, pools, perm, func(_ string, allowed bool) bool {
		allAllowed = allAllowed && allowed
		return allAllowed
	})

	switch {
	case !ok:
		return CheckResult{InternalError: true}
	case allAllowed:
		return CheckResult{Permitted: true}
	default:
		// TODO(vadimsh): Make the error message more informative.
		return CheckResult{
			err: status.Errorf(
				codes.PermissionDenied,
				"the caller %q doesn't have permission %q in some of the requested pools",
				chk.caller, perm),
		}
	}
}

// CheckAnyPoolsPerm checks if the caller has a permission in *any* given pool.
//
// The list of pools must not be empty. Panics if it is.
func (chk *Checker) CheckAnyPoolsPerm(ctx context.Context, pools []string, perm realms.Permission) CheckResult {
	switch len(pools) {
	case 0:
		panic("empty list of pools in CheckAnyPoolsPerm")
	case 1:
		// Use a single pool check for better error messages.
		return chk.CheckPoolPerm(ctx, pools[0], perm)
	}

	// If have a server-level permission, no need to check individual pools.
	if res := chk.CheckServerPerm(ctx, perm); res.Permitted || res.InternalError {
		return res
	}

	oneAllowed := false

	ok := chk.visitRealms(ctx, pools, perm, func(_ string, allowed bool) bool {
		oneAllowed = oneAllowed || allowed
		return !oneAllowed
	})

	switch {
	case !ok:
		return CheckResult{InternalError: true}
	case oneAllowed:
		return CheckResult{Permitted: true}
	default:
		// TODO(vadimsh): Make the error message more informative.
		return CheckResult{
			err: status.Errorf(
				codes.PermissionDenied,
				"the caller %q doesn't have permission %q in any of the requested pools",
				chk.caller, perm),
		}
	}
}

// CheckTaskPerm checks if the caller has a permission in a specific task.
//
// It checks individual task ACL (based on task realm), as well as task's pool
// permissions (via CheckPoolPerm).
func (chk *Checker) CheckTaskPerm(ctx context.Context, task TaskAuthInfo, perm realms.Permission) CheckResult {
	// TODO(vadimsh): Implement.
	// Check submitter == caller.
	// Check the server level permission.
	// Check the task realm permission.
	// Check the assigned pool permission.
	// Check bot's pool permissions.
	return chk.CheckServerPerm(ctx, perm)
}

// CheckBotPerm checks if the caller has a permission in a specific bot.
//
// It checks bot's pool permissions via CheckPoolPerm.
func (chk *Checker) CheckBotPerm(ctx context.Context, botID string, perm realms.Permission) CheckResult {
	// TODO(vadimsh): Implement.
	// Check the server lever permissions.
	// Lookup a list of bot realms based on the config and/or history.
	// CheckAnyPoolsPerm.
	return chk.CheckServerPerm(ctx, perm)
}

// visitRealms does a permission check for every pool, sequentially.
//
// It calls the callback with the outcome of the check. If the callback returns
// true, the iteration continues. Otherwise it stops and visitRealms returns
// true. Returns false only on internal problems with the check.
func (chk *Checker) visitRealms(ctx context.Context, pools []string, perm realms.Permission, cb func(pool string, allowed bool) bool) (ok bool) {
	checkedRealms := map[string]bool{}
	for _, pool := range pools {
		cfg := chk.cfg.Pool(pool)
		if cfg == nil {
			// Missing pools assumed to have no permissions in them.
			if !cb(pool, false) {
				return true
			}
			continue
		}
		if _, checked := checkedRealms[cfg.Realm]; !checked {
			outcome, err := chk.db.HasPermission(ctx, chk.caller, perm, cfg.Realm, nil)
			if err != nil {
				logging.Errorf(ctx, "Error in HasPermission(%q, %q): %s", perm, cfg.Realm, err)
				return false
			}
			checkedRealms[cfg.Realm] = outcome
		}
		if !cb(pool, checkedRealms[cfg.Realm]) {
			return true
		}
	}
	return true
}
