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

	"google.golang.org/protobuf/proto"

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
func (u *updaterBackend) Fetch(ctx context.Context, in *changelist.FetchInput) (changelist.UpdateFields, error) {
	gHost, gChange, err := in.CL.ExternalID.ParseGobID()
	if err != nil {
		return changelist.UpdateFields{}, err
	}

	f := fetcher{
		gFactory:                     u.gFactory,
		scheduleRefresh:              u.clUpdater.ScheduleDelayed,
		resolveAndScheduleDepsUpdate: u.clUpdater.ResolveAndScheduleDepsUpdate,

		project:    in.Project,
		externalID: in.CL.ExternalID,
		host:       gHost,
		change:     gChange,
		hint:       in.Hint,
		requester:  in.Requester,
	}
	if in.CL.ID > 0 {
		f.priorCL = in.CL
	}

	f.g, err = f.gFactory.MakeClient(ctx, f.host, f.project)
	if err != nil {
		return changelist.UpdateFields{}, err
	}

	if err := f.fetch(ctx); err != nil {
		return changelist.UpdateFields{}, err
	}
	return f.toUpdate, nil
}

// HasChanged implements the changelist.UpdaterBackend.
func (u *updaterBackend) HasChanged(cvCurrent, backendCurrent *changelist.Snapshot) bool {
	cvInfo := cvCurrent.GetGerrit().GetInfo()
	backendInfo := backendCurrent.GetGerrit().GetInfo()
	switch {
	case backendInfo.GetUpdated().AsTime().After(cvInfo.GetUpdated().AsTime()):
		return true
	case cvInfo.GetUpdated().AsTime().After(backendInfo.GetUpdated().AsTime()):
		// LUCI CV has more recent data. Most likely Gerrit is returning stale data.
		return false
	case !proto.Equal(cvInfo.GetOwner(), backendInfo.GetOwner()):
		// TODO - crbug/1523301: Remove after Feb. 5 2024. This is temporarily added
		// here to backfill owner's tag introduced https://crrev.com/c/5250717 so
		// that LUCI CV will try to backfill CL entity with the owner's tag as much
		// as possible.
		return true
	default:
		// Gerrit doesn't support fractional second precision. Therefore, even if
		// two change info have the exact same update timestamp. They could still
		// be different. See a real example in https://crbug.com/1294440.
		// Use meta_rev_id which is the sha1 of go/NoteDB to detect whether the
		// snapshot has changed in the Gerrit backend.
		return cvInfo.GetMetaRevId() != backendInfo.GetMetaRevId()
	}
}

// TQErrorSpec implements the changelist.UpdaterBackend.
func (u *updaterBackend) TQErrorSpec() common.TQIfy {
	return common.TQIfy{
		// Don't log the entire stack trace of stale data, which is sadly an
		// hourly occurrence.
		KnownRetry: []error{gerrit.ErrStaleData, gerrit.ErrOutOfQuota, gerrit.ErrGerritDeadlineExceeded},
	}
}
