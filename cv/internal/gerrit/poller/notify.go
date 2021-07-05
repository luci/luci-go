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

package poller

import (
	"context"
	"time"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	gerritpb "go.chromium.org/luci/common/proto/gerrit"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/common/sync/parallel"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/cv/internal/changelist"
	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/gerrit/updater"
)

// maxLoadCLBatchSize limits how many CL entities are loaded at once for
// notifying PM.
const maxLoadCLBatchSize = 100

func (p *Poller) notifyOnMatchedCLs(ctx context.Context, luciProject, gerritHost string, changes []*gerritpb.ChangeInfo, forceNotifyPM bool) error {
	if len(changes) == 0 {
		return nil
	}
	// TODO(tandrii): optimize by checking if CV is interested in the
	// (host,project,ref) of these changes from before triggering tasks.
	logging.Debugf(ctx, "scheduling %d CLUpdate tasks (forceNotifyPM: %t)", len(changes), forceNotifyPM)

	var clids []common.CLID
	if forceNotifyPM {
		// Objective: make PM aware of all CLs.
		// Optimization: avoid RefreshGerritCL with forceNotifyPM=true if possible,
		// as this removes ability to de-duplicate.
		var err error
		eids := make([]changelist.ExternalID, len(changes))
		for i, c := range changes {
			if eids[i], err = changelist.GobID(gerritHost, c.GetNumber()); err != nil {
				return err
			}
		}
		clids, err = changelist.Lookup(ctx, eids)
		if err != nil {
			return err
		}
		if err := p.notifyPMifKnown(ctx, luciProject, clids, maxLoadCLBatchSize); err != nil {
			return err
		}
	}

	errs := parallel.WorkPool(min(10, len(changes)), func(work chan<- func() error) {
		for i, c := range changes {
			payload := &updater.RefreshGerritCL{
				LuciProject: luciProject,
				Host:        gerritHost,
				Change:      c.GetNumber(),
				UpdatedHint: c.GetUpdated(),
				ForceNotify: forceNotifyPM && (clids[i] == 0),
			}
			work <- func() error {
				return p.clUpdater.Schedule(ctx, payload)
			}
		}
	})
	return common.MostSevereError(errs)
}

func (p *Poller) notifyOnUnmatchedCLs(ctx context.Context, luciProject, host string, changes []int64) error {
	if len(changes) == 0 {
		return nil
	}
	logging.Debugf(ctx, "notifying CL Updater and PM on %d no longer matched CLs", len(changes))
	var err error
	eids := make([]changelist.ExternalID, len(changes))
	for i, c := range changes {
		if eids[i], err = changelist.GobID(host, c); err != nil {
			return err
		}
	}
	// Objective: make PM aware of all CLs.
	// Optimization: avoid RefreshGerritCL with forceNotifyPM=true if possible,
	// as this removes ability to de-duplicate.
	// Since all no longer matched CLs are typically already stored in the
	// Datastore, get their internal CL IDs and notify PM directly.
	clids, err := changelist.Lookup(ctx, eids)
	if err != nil {
		return err
	}

	if err := p.notifyPMifKnown(ctx, luciProject, clids, maxLoadCLBatchSize); err != nil {
		return err
	}

	errs := parallel.WorkPool(min(10, len(changes)), func(work chan<- func() error) {
		for i, c := range changes {
			payload := &updater.RefreshGerritCL{
				LuciProject: luciProject,
				Host:        host,
				Change:      c,
				ForceNotify: 0 == clids[i], // notify iff CL ID isn't yet known.
			}
			// Distribute these tasks in time to avoid high peaks (e.g. see
			// https://crbug.com/1211057).
			delay := (fullPollInterval * time.Duration(i)) / time.Duration(len(clids))
			work <- func() error {
				return p.clUpdater.ScheduleDelayed(ctx, payload, delay)
			}
		}
	})
	return common.MostSevereError(errs)
}

// notifyPMifKnown notifies PM to update its CLs for each non-0 CLID.
//
// Obtains EVersion of each CL before notify a PM. Unfortunately, this loads a
// lot of information we don't need, such as Snapshot. So, loads CLs in batches
// to avoid large memory footprint.
//
// In practice, most of these CLs would be already dscache-ed, so loading them
// is fast.
func (p *Poller) notifyPMifKnown(ctx context.Context, luciProject string, clids []common.CLID, maxBatchSize int) error {
	cls := make([]*changelist.CL, 0, maxBatchSize)
	flush := func() error {
		if err := datastore.Get(ctx, cls); err != nil {
			return errors.Annotate(common.MostSevereError(err), "failed to load CLs").Tag(transient.Tag).Err()
		}
		return p.pm.NotifyCLsUpdated(ctx, luciProject, cls)
	}

	for _, clid := range clids {
		switch l := len(cls); {
		case clid == 0:
		case l == maxBatchSize:
			if err := flush(); err != nil {
				return err
			}
			cls = cls[:0]
		default:
			cls = append(cls, &changelist.CL{ID: clid})
		}
	}
	if len(cls) > 0 {
		return flush()
	}
	return nil
}
