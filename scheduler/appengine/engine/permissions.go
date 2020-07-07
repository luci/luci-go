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

	// Covert the permission to a legacy role and check job's AclSet for it.
	role, ok := permToRole[perm]
	if !ok {
		return errors.Reason("unknown job permission %q", perm).Err()
	}
	legacyResult, err := job.Acls.CallerHasRole(ctx, role)
	if err != nil {
		return err
	}

	// Fallback to the @legacy realm if the job entity isn't updated yet.
	realm := job.RealmID
	if realm == "" {
		realm = realms.Join(job.ProjectID, realms.LegacyRealm)
	}

	// Check realms ACL and compare it to `legacyResult`. The result and errors
	// (if any) are recorded inside.
	auth.HasPermissionDryRun{
		ExpectedResult: legacyResult,
		TrackingBug:    "crbug.com/1070761",
	}.Execute(ctx, perm, []string{realm})

	// Use the legacy result for now.
	if !legacyResult {
		return ErrNoPermission
	}
	return nil
}
