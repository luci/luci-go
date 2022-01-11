// Copyright 2020 The LUCI Authors.
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

package engine

import (
	"context"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/realms"

	"go.chromium.org/luci/scheduler/appengine/acl"
)

const adminGroup = "administrators"

var (
	permJobsGet     = realms.RegisterPermission("scheduler.jobs.get")
	permJobsPause   = realms.RegisterPermission("scheduler.jobs.pause")
	permJobsResume  = realms.RegisterPermission("scheduler.jobs.resume")
	permJobsAbort   = realms.RegisterPermission("scheduler.jobs.abort")
	permJobsTrigger = realms.RegisterPermission("scheduler.jobs.trigger")
)

// Mapping from Realms permissions to legacy roles.
//
// Will be removed once legacy ACLs are gone.
var permToRole = map[realms.Permission]acl.Role{
	permJobsGet:     acl.Reader,
	permJobsPause:   acl.Owner,
	permJobsResume:  acl.Owner,
	permJobsAbort:   acl.Owner,
	permJobsTrigger: acl.Triggerer,
}

// checkPermission returns nil if the caller has the given permission for the
// job or ErrNoPermission otherwise.
//
// May also return transient errors.
//
// Currently checks both legacy ACLs and Realm ACLs, compares them (logging
// the differences) and picks an outcome of legacy ACLs check as the result.
func checkPermission(ctx context.Context, job *Job, perm realms.Permission) error {
	ctx = logging.SetField(ctx, "JobID", job.JobID)

	// Fallback to the @legacy realm if the job entity isn't updated yet.
	realm := job.RealmID
	if realm == "" {
		realm = realms.Join(job.ProjectID, realms.LegacyRealm)
	}

	switch enforce, err := auth.ShouldEnforceRealmACL(ctx, realm); {
	case err != nil:
		return err
	case enforce:
		return checkRealmACL(ctx, perm, realm)
	default:
		return checkLegacyACL(ctx, job, perm, realm)
	}
}

// checkRealmACL uses Realms ACLs, totally ignoring legacy ACLs.
func checkRealmACL(ctx context.Context, perm realms.Permission, realm string) error {
	// Admins have implicit access to everything.
	// TODO(vadimsh): We should probably remove this.
	switch yes, err := auth.IsMember(ctx, adminGroup); {
	case err != nil:
		return err
	case yes:
		return nil
	}

	// Else fallback to checking permissions.
	switch yes, err := auth.HasPermission(ctx, perm, realm, nil); {
	case err != nil:
		return err
	case !yes:
		return ErrNoPermission
	default:
		return nil
	}
}

// checkLegacyACL uses legacy ACLs, but also compares them to realm ACLs.
func checkLegacyACL(ctx context.Context, job *Job, perm realms.Permission, realm string) error {
	// Covert the permission to a legacy role and check job's AclSet for it.
	role, ok := permToRole[perm]
	if !ok {
		return errors.Reason("unknown job permission %q", perm).Err()
	}
	legacyResult, err := job.Acls.CallerHasRole(ctx, role)
	if err != nil {
		return err
	}

	// Check realms ACL and compare it to `legacyResult`. The result and errors
	// (if any) are recorded inside.
	auth.HasPermissionDryRun{
		ExpectedResult: legacyResult,
		TrackingBug:    "crbug.com/1070761",
		AdminGroup:     adminGroup,
	}.Execute(ctx, perm, realm, nil)

	if !legacyResult {
		return ErrNoPermission
	}
	return nil
}
