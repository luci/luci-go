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
	"fmt"

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
// important to **drop** this checker once the request is finished, to avoid
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
//
// Not safe for concurrent use without external synchronization.
type Checker struct {
	cfg    *cfg.Config       // swarming config
	db     authdb.DB         // auth DB with groups and permissions
	caller identity.Identity // authenticated identity of the caller

	// Cached results of CheckServerPerm.
	cachedServerPerms map[realms.Permission]CheckResult
	// Cached results of individual HasPermission checks.
	cachedRealmPerms map[realmAndPerm]CheckResult
}

// realmAndPerm is a key for cachedRealmPerms map.
type realmAndPerm struct {
	realm string
	perm  realms.Permission
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
// Extracted either from TaskRequest or from TaskResultSummary. All fields
// except TaskID are optional. Most of them are unset for internal tasks like
// TerminateBot tasks.
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

// Task can produce TaskAuthInfo on demand.
type Task interface {
	// TaskAuthInfo returns properties of a task that affect who can access it.
	//
	// Any error here is treated as an internal server error.
	TaskAuthInfo(ctx context.Context) (*TaskAuthInfo, error)
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
	if res, ok := chk.cachedServerPerms[perm]; ok {
		return res
	}

	serverGroups := chk.cfg.Settings().Auth

	var yes bool
	var err error

	switch perm {
	case PermTasksGet, PermPoolsListTasks:
		yes, err = chk.db.IsMember(ctx, chk.caller, []string{
			serverGroups.ViewAllTasksGroup,
			serverGroups.PrivilegedUsersGroup,
			serverGroups.AdminsGroup,
		})

	case PermPoolsListBots:
		yes, err = chk.db.IsMember(ctx, chk.caller, []string{
			serverGroups.ViewAllBotsGroup,
			serverGroups.PrivilegedUsersGroup,
			serverGroups.AdminsGroup,
		})

	case PermPoolsCreateBot:
		yes, err = chk.db.IsMember(ctx, chk.caller, []string{
			serverGroups.BotBootstrapGroup,
			serverGroups.AdminsGroup,
		})

	case PermTasksCancel, PermPoolsCancelTask, PermPoolsDeleteBot, PermPoolsTerminateBot:
		yes, err = chk.db.IsMember(ctx, chk.caller, []string{
			serverGroups.AdminsGroup,
		})

	default:
		// This permission is not defined on the global level, denied by default.
	}

	var res CheckResult
	switch {
	case err != nil:
		logging.Errorf(ctx, "Error when checking groups: %s", err)
		res = CheckResult{InternalError: true}
	case yes:
		res = CheckResult{Permitted: true}
	default:
		res = CheckResult{err: status.Errorf(
			codes.PermissionDenied,
			"the caller %q doesn't have server-level permission %q",
			chk.caller, perm),
		}
	}

	if chk.cachedServerPerms == nil {
		chk.cachedServerPerms = make(map[realms.Permission]CheckResult, 1)
	}
	chk.cachedServerPerms[perm] = res

	return res
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
		if res := chk.hasPermission(ctx, perm, cfg.Realm); res.Permitted || res.InternalError {
			return res
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
// Only accepts permissions targeting a single existing task: PermTasksGet and
// PermTasksCancel. Panics if asked to check any other permission.
//
// It checks individual task ACL (based on task realm), as well as task's pool
// ACL. The idea is that the caller can either "own" the task or "own" the bot
// pool it was scheduled to run on. E.g. for a task to be visible, the caller
// either needs PermTasksGet in the task's realm, or PermPoolsListTasks in the
// bot pool realm. This function checks both.
func (chk *Checker) CheckTaskPerm(ctx context.Context, task Task, perm realms.Permission) CheckResult {
	// Look up a matching pool level permission to check it in the task's pool.
	var poolPerm realms.Permission
	switch perm {
	case PermTasksGet:
		poolPerm = PermPoolsListTasks
	case PermTasksCancel:
		poolPerm = PermPoolsCancelTask
	default:
		panic(fmt.Sprintf("not a task-level permission %q", perm))
	}

	// If have a server-level permission, no need to check anything else. Note
	// that on the server level task<->pool permission pairs like PermTasksGet and
	// PermPoolsListTasks are treated identically, so it is sufficient to check
	// only `perm` (and skip checking `poolPerm`: the outcome will be the same).
	if res := chk.CheckServerPerm(ctx, perm); res.Permitted || res.InternalError {
		return res
	}

	// Get the details about the task.
	info, err := task.TaskAuthInfo(ctx)
	if err != nil {
		logging.Errorf(ctx, "Error getting TaskAuthInfo: %s", err)
		return CheckResult{InternalError: true}
	}

	// Whoever submitted the task has full control over it.
	if info.Submitter != "" && info.Submitter == chk.caller {
		return CheckResult{Permitted: true}
	}

	// Check if the caller has the permission in the task's own realm.
	if info.Realm != "" {
		if res := chk.hasPermission(ctx, perm, info.Realm); res.Permitted || res.InternalError {
			return res
		}
	}

	// Check if the caller has the matching permission in the task's assigned
	// pool. If the task has no pool assigned but instead was scheduled to run on
	// a concrete bot (happens for termination tasks), check if the caller has
	// the permission in this bot's pool.
	//
	// Note that when both Pool and BotID fields are set, Pool should take
	// precedence, since the pool is what we check when submitting tasks (i.e. for
	// a new task with dimensions `{"pool": ..., "bot": ...}` only "pool" is being
	// used in permission checks and "bot" is completely unrestricted). Checking
	// pool here as well results in more consistent behavior.
	//
	// Note that it is forbidden to submit arbitrary tasks without a pool through
	// the public API. They can be submitted only by the Swarming server
	// internally.
	var poolsToCheck []string
	if info.Pool != "" {
		poolsToCheck = []string{info.Pool}
	} else if info.BotID != "" {
		poolsToCheck = chk.cfg.BotGroup(info.BotID).Pools()
	}
	if len(poolsToCheck) != 0 {
		oneAllowed := false
		ok := chk.visitRealms(ctx, poolsToCheck, poolPerm, func(_ string, allowed bool) bool {
			oneAllowed = oneAllowed || allowed
			return !oneAllowed
		})
		switch {
		case !ok:
			return CheckResult{InternalError: true}
		case oneAllowed:
			return CheckResult{Permitted: true}
		}
	}

	// TODO(vadimsh): Make the error message more informative.
	return CheckResult{
		err: status.Errorf(
			codes.PermissionDenied,
			"the caller %q doesn't have permission %q for the task %q",
			chk.caller, perm, info.TaskID),
	}
}

// CheckBotPerm checks if the caller has a permission in a specific bot.
//
// It looks up a realm the bot belong to (based on "pool" dimension) and then
// checks the caller has the required permission in this realm.
func (chk *Checker) CheckBotPerm(ctx context.Context, botID string, perm realms.Permission) CheckResult {
	// If have a server-level permission, no need to fetch bot info.
	if res := chk.CheckServerPerm(ctx, perm); res.Permitted || res.InternalError {
		return res
	}

	// TODO(vadimsh): Python code used to fetch BotInfo or BotEvent from datastore
	// to look up bot pools. This matters for bots removed from configs. Avoid
	// this for now (fetch the bot info exclusively from the current config) to
	// see if it makes any observable difference for real use cases.
	pools := chk.cfg.BotGroup(botID).Pools()
	if len(pools) == 0 {
		panic("impossible due to the config validation and Pools() logic")
	}

	// Note: we can't just call CheckAnyPoolsPerm since it can potentially leak
	// pool name in its error message. In CheckBotPerm we don't know if the caller
	// is allowed to see bot => pool association and should not expose the pool
	// name in errors, only bot ID.

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
				"the caller %q doesn't have permission %q in the pool that contains bot %q or this bot doesn't exist",
				chk.caller, perm, botID),
		}
	}
}

// visitRealms does a permission check for every pool, sequentially.
//
// It calls the callback with the outcome of the check. If the callback returns
// true, the iteration continues. Otherwise it stops and visitRealms returns
// true. Returns false only on internal problems with the check.
func (chk *Checker) visitRealms(ctx context.Context, pools []string, perm realms.Permission, cb func(pool string, allowed bool) bool) (ok bool) {
	for _, pool := range pools {
		cfg := chk.cfg.Pool(pool)
		if cfg == nil {
			// Missing pools assumed to have no permissions in them.
			logging.Warningf(ctx, "Unknown pool when checking ACLs: %s", pool)
			if !cb(pool, false) {
				return true
			}
			continue
		}
		res := chk.hasPermission(ctx, perm, cfg.Realm)
		if res.InternalError {
			return false
		}
		if !cb(pool, res.Permitted) {
			return true
		}
	}
	return true
}

// Use a constant error to avoid creating errors on a hot path (e.g. when
// batch checking permissions of hundreds of tasks). This error is always
// replaced with a more concrete error later.
var errGenericPerm = status.Errorf(
	codes.PermissionDenied,
	"generic permission denied error, should never be seen",
)

// hasPermission checks if the caller has a permission in a realm.
//
// Caches the outcome. Logs internal errors.
func (chk *Checker) hasPermission(ctx context.Context, perm realms.Permission, realm string) CheckResult {
	key := realmAndPerm{realm, perm}
	if res, ok := chk.cachedRealmPerms[key]; ok {
		return res
	}

	var res CheckResult
	switch yes, err := chk.db.HasPermission(ctx, chk.caller, perm, realm, nil); {
	case err != nil:
		logging.Errorf(ctx, "Error in HasPermission(%q, %q): %s", perm, realm, err)
		res = CheckResult{InternalError: true}
	case yes:
		res = CheckResult{Permitted: true}
	default:
		res = CheckResult{err: errGenericPerm}
	}

	if chk.cachedRealmPerms == nil {
		chk.cachedRealmPerms = make(map[realmAndPerm]CheckResult, 1)
	}
	chk.cachedRealmPerms[key] = res

	return res
}
