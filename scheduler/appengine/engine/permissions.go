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

	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/realms"
)

const adminGroup = "administrators"

var (
	PermJobsGet     = realms.RegisterPermission("scheduler.jobs.get")
	PermJobsPause   = realms.RegisterPermission("scheduler.jobs.pause")
	PermJobsResume  = realms.RegisterPermission("scheduler.jobs.resume")
	PermJobsAbort   = realms.RegisterPermission("scheduler.jobs.abort")
	PermJobsTrigger = realms.RegisterPermission("scheduler.jobs.trigger")
)

// CheckPermission returns nil if the caller has the given permission for the
// job or ErrNoPermission otherwise.
//
// May also return transient errors.
func CheckPermission(ctx context.Context, job *Job, perm realms.Permission) error {
	ctx = logging.SetField(ctx, "JobID", job.JobID)

	// Fallback to the @legacy realm if the job entity isn't updated yet.
	realm := job.RealmID
	if realm == "" {
		realm = realms.Join(job.ProjectID, realms.LegacyRealm)
	}

	// Check realm's bindings (including conditional ones).
	attrs := realms.Attrs{
		"scheduler.job.name": job.JobName(),
	}
	switch yes, err := auth.HasPermission(ctx, perm, realm, attrs); {
	case err != nil:
		return err
	case yes:
		return nil
	}

	// Admins have implicit access to everything.
	// TODO(vadimsh): We should probably remove this.
	switch yes, err := auth.IsMember(ctx, adminGroup); {
	case err != nil:
		return err
	case yes:
		logging.Warningf(ctx, "ADMIN_FALLBACK: perm=%q job=%q caller=%q",
			perm, job.JobID, auth.CurrentIdentity(ctx))
		return nil
	default:
		return ErrNoPermission
	}
}
