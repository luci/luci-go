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

package updater

import (
	"context"
	"time"

	"go.chromium.org/luci/cv/internal/changelist"
	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/gerrit"
	"go.chromium.org/luci/cv/internal/gerrit/gobmap"
)

// RegisterUpdater register a Gerrit backend with the CL Updater.
func RegisterUpdater(u *changelist.Updater, gFactory gerrit.Factory) {
	u.RegisterBackend(&updaterBackend{
		gFactory:  gFactory,
		clUpdater: u,
	})
}

// updaterBackend implements changelist.UpdaterBackend for Gerrit.
type updaterBackend struct {
	gFactory gerrit.Factory
	// clUpdater is necessary to schedule async tasks to fetch newly-discovered
	// dependencies of the currently updated CL.
	clUpdater *changelist.Updater
}

// Kind implements the changelist.UpdaterBackend.
func (u *updaterBackend) Kind() string {
	return "gerrit"
}

// LookupApplicableConfig implements the changelist.UpdaterBackend.
func (u *updaterBackend) LookupApplicableConfig(ctx context.Context, saved *changelist.CL) (*changelist.ApplicableConfig, error) {
	g := saved.Snapshot.GetGerrit()
	if g == nil {
		// Not enough info to decide.
		return nil, nil
	}
	ci := g.GetInfo()
	return gobmap.Lookup(ctx, g.GetHost(), ci.GetProject(), ci.GetRef())
}

// Fetch implements the changelist.UpdaterBackend.
func (u *updaterBackend) Fetch(ctx context.Context, cl *changelist.CL, luciProject string, updatedHint time.Time) (changelist.UpdateFields, error) {
	gHost, gChange, err := cl.ExternalID.ParseGobID()
	if err != nil {
		return changelist.UpdateFields{}, err
	}

	f := fetcher{
		gFactory:                     u.gFactory,
		scheduleRefresh:              u.clUpdater.ScheduleDelayed,
		resolveAndScheduleDepsUpdate: u.clUpdater.ResolveAndScheduleDepsUpdate,

		luciProject: luciProject,
		externalID:  cl.ExternalID,
		host:        gHost,
		change:      gChange,
		updatedHint: updatedHint,
	}
	if cl.ID > 0 {
		f.priorCL = cl
	}

	f.g, err = f.gFactory.MakeClient(ctx, f.host, f.luciProject)
	if err != nil {
		return changelist.UpdateFields{}, err
	}

	if err := f.fetch(ctx); err != nil {
		return changelist.UpdateFields{}, err
	}
	return f.toUpdate, nil
}

// TQErrorSpec implements the changelist.UpdaterBackend.
func (u *updaterBackend) TQErrorSpec() common.TQIfy {
	return common.TQIfy{
		// Don't log the entire stack trace of stale data, which is sadly an
		// hourly occurrence.
		KnownRetry: []error{gerrit.ErrStaleData, gerrit.ErrOutOfQuota, gerrit.ErrGerritDeadlineExceeded},
	}
}

func min(i, j int) int {
	if i < j {
		return i
	}
	return j
}
