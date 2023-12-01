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
	cfg *cfg.Config
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
	return &Checker{cfg: cfg}
}

// CheckServerPerm checks if the caller has a permission on a server level.
//
// Having a permission on a server level means it applies to all pools, tasks
// and bots in this instance of Swarming.
//
// Returns nil if the caller has the permission. Returns a gRPC error if the
// caller doesn't have the permission or the check itself failed. The error can
// be returned to the caller as is. Use IsDeniedErr to see if the error
// represents "permission denied" response. Note that IsDeniedErr(err) will be
// false for internal errors (e.g. timeouts).
func (chk *Checker) CheckServerPerm(ctx context.Context, perm realms.Permission) error {
	// TODO(vadimsh): Implement.
	return nil
}

// CheckPoolPerm checks if the caller has a permission on a pool level.
//
// Having a permission on a pool level means it applies for all tasks and bots
// in that pool. CheckPoolPerm implicitly calls CheckServerPerm.
//
// Returns nil if the caller has the permission. Returns a gRPC error if the
// caller doesn't have the permission or the check itself failed. The error can
// be returned to the caller as is. Use IsDeniedErr to see if the error
// represents "permission denied" response. Note that IsDeniedErr(err) will be
// false for internal errors (e.g. timeouts).
func (chk *Checker) CheckPoolPerm(ctx context.Context, pool string, perm realms.Permission) error {
	// TODO(vadimsh): Implement.
	return chk.CheckServerPerm(ctx, perm)
}

// CheckAllPoolsPerm checks if the caller has a permission in *all* given pools.
//
// The list of pools must not be empty. Panics if it is.
//
// Returns nil if the caller has the permission. Returns a gRPC error if the
// caller doesn't have the permission or the check itself failed. The error can
// be returned to the caller as is. Use IsDeniedErr to see if the error
// represents "permission denied" response. Note that IsDeniedErr(err) will be
// false for internal errors (e.g. timeouts).
func (chk *Checker) CheckAllPoolsPerm(ctx context.Context, pools []string, perm realms.Permission) error {
	if len(pools) == 0 {
		panic("empty list of pools in CheckAllPoolsPerm")
	}
	// If have a server-level permission, no need to check individual pools.
	switch err := chk.CheckServerPerm(ctx, perm); {
	case err == nil:
		return nil
	case !IsDeniedErr(err):
		return err
	}
	// TODO(vadimsh): Optimize.
	for _, pool := range pools {
		if err := chk.CheckPoolPerm(ctx, pool, perm); err != nil {
			return err
		}
	}
	return nil
}

// CheckAnyPoolsPerm checks if the caller has a permission in *any* given pool.
//
// The list of pools must not be empty. Panics if it is.
//
// Returns nil if the caller has the permission. Returns a gRPC error if the
// caller doesn't have the permission or the check itself failed. The error can
// be returned to the caller as is. Use IsDeniedErr to see if the error
// represents "permission denied" response. Note that IsDeniedErr(err) will be
// false for internal errors (e.g. timeouts).
func (chk *Checker) CheckAnyPoolsPerm(ctx context.Context, pools []string, perm realms.Permission) error {
	if len(pools) == 0 {
		panic("empty list of pools in CheckAnyPoolsPerm")
	}
	// If have a server-level permission, no need to check individual pools.
	switch err := chk.CheckServerPerm(ctx, perm); {
	case err == nil:
		return nil
	case !IsDeniedErr(err):
		return err
	}
	// TODO(vadimsh): Optimize.
	for _, pool := range pools {
		switch err := chk.CheckPoolPerm(ctx, pool, perm); {
		case err == nil:
			return nil
		case !IsDeniedErr(err):
			return err
		}
	}
	// TODO(vadimsh): Improve the error message to mention concrete pools if the
	// caller has permissions to see them at all.
	return status.Errorf(codes.PermissionDenied, "no %q permission in required pools", perm)
}

// CheckTaskPerm checks if the caller has a permission in a specific task.
//
// It checks individual task ACL (based on task realm), as well as task's pool
// permissions (via CheckPoolPerm).
//
// Returns nil if the caller has the permission. Returns a gRPC error if the
// caller doesn't have the permission or the check itself failed. The error can
// be returned to the caller as is. Use IsDeniedErr to see if the error
// represents "permission denied" response. Note that IsDeniedErr(err) will be
// false for internal errors (e.g. timeouts).
func (chk *Checker) CheckTaskPerm(ctx context.Context, task TaskAuthInfo, perm realms.Permission) error {
	// TODO(vadimsh): Implement.
	return chk.CheckServerPerm(ctx, perm)
}

// CheckBotPerm checks if the caller has a permission in a specific bot.
//
// It checks bot's pool permissions via CheckPoolPerm.
//
// Returns nil if the caller has the permission. Returns a gRPC error if the
// caller doesn't have the permission or the check itself failed. The error can
// be returned to the caller as is. Use IsDeniedErr to see if the error
// represents "permission denied" response. Note that IsDeniedErr(err) will be
// false for internal errors (e.g. timeouts).
func (chk *Checker) CheckBotPerm(ctx context.Context, botID string, perm realms.Permission) error {
	// TODO(vadimsh): Implement.
	return chk.CheckServerPerm(ctx, perm)
}

// IsDeniedErr returns true if the error means "permission denied" or similar.
//
// It returns false if the error represents some internal error. It panics if
// `err` is nil.
//
// To avoid leaking existence of private resources, permission errors can
// sometimes surface as "not found" errors to the end user. This function
// recognizes "not found" as a "permission denied" error as well.
func IsDeniedErr(err error) bool {
	switch status.Code(err) {
	case codes.OK:
		panic("unexpected non-error in IsDeniedErr")
	case codes.PermissionDenied, codes.Unauthenticated, codes.NotFound:
		return true
	default:
		return false
	}
}
