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
)

// maxLoadCLBatchSize limits how many CL entities are loaded at once for
// notifying PM.
const maxLoadCLBatchSize = 100

func (p *Poller) notifyOnMatchedCLs(ctx context.Context, luciProject, host string, changes []*gerritpb.ChangeInfo, forceNotifyPM bool, requester changelist.UpdateCLTask_Requester) error {
	if len(changes) == 0 {
		return nil
	}
	// TODO(tandrii): optimize by checking if CV is interested in the
	// (host,project,ref) of these changes from before triggering tasks.
	logging.Debugf(ctx, "scheduling %d CLUpdate tasks (forceNotifyPM: %t)", len(changes), forceNotifyPM)

	if forceNotifyPM {
		changeNumbers := make([]int64, len(changes))
		for i, c := range changes {
			changeNumbers[i] = c.GetNumber()
		}
		if err := p.notifyPMifKnown(ctx, luciProject, host, changeNumbers, maxLoadCLBatchSize); err != nil {
			return err
		}
	}

	errs := parallel.WorkPool(min(10, len(changes)), func(work chan<- func() error) {
		for _, c := range changes {
			payload := &changelist.UpdateCLTask{
				LuciProject: luciProject,
				ExternalId:  string(changelist.MustGobID(host, c.GetNumber())),
				Hint:        &changelist.UpdateCLTask_Hint{ExternalUpdateTime: c.GetUpdated()},
				Requester:   requester,
			}
			work <- func() error {
				return p.clUpdater.Schedule(ctx, payload)
			}
		}
	})
	return common.MostSevereError(errs)
}

func (p *Poller) notifyOnUnmatchedCLs(ctx context.Context, luciProject, host string, changes []int64, requester changelist.UpdateCLTask_Requester) error {
	if len(changes) == 0 {
		return nil
	}
	logging.Debugf(ctx, "notifying CL Updater and PM on %d no longer matched CLs", len(changes))

	if err := p.notifyPMifKnown(ctx, luciProject, host, changes, maxLoadCLBatchSize); err != nil {
		return err
	}
	errs := parallel.WorkPool(min(10, len(changes)), func(work chan<- func() error) {
		for i, c := range changes {
			payload := &changelist.UpdateCLTask{
				LuciProject: luciProject,
				ExternalId:  string(changelist.MustGobID(host, c)),
				Requester:   requester,
			}
			// Distribute these tasks in time to avoid high peaks (e.g. see
			// https://crbug.com/1211057).
			delay := (fullPollInterval * time.Duration(i)) / time.Duration(len(changes))
			work <- func() error {
				return p.clUpdater.ScheduleDelayed(ctx, payload, delay)
			}
		}
	})
	return common.MostSevereError(errs)
}

// notifyPMifKnown notifies PM to update its CLs for each Gerrit Change
// with existing CL entity.
//
// For Gerrit Changes without a CL entity, either:
//   - the Gerrit CL Updater will create it in the future and hence also notify
//     the PM;
//   - or if the Gerrit CL updater doesn't do it, then there is no point
//     notifying the PM anyway.
//
// Obtains EVersion of each CL before notify a PM. Unfortunately, this loads a
// lot of information we don't need, such as Snapshot. So, load CLs in batches
// to avoid large memory footprint.
//
// In practice, most of these CLs would be already dscache-ed, so loading them
// is fast.
func (p *Poller) notifyPMifKnown(ctx context.Context, luciProject, host string, changes []int64, maxBatchSize int) error {
	eids := make([]changelist.ExternalID, len(changes))
	for i, c := range changes {
		eids[i] = changelist.MustGobID(host, c)
	}
	clids, err := changelist.Lookup(ctx, eids)
	if err != nil {
		return err
	}

	cls := make([]*changelist.CL, 0, maxBatchSize)
	flush := func() error {
		if err := datastore.Get(ctx, cls); err != nil {
			return transient.Tag.Apply(errors.WrapIf(common.MostSevereError(err), "failed to load CLs"))
		}
		return p.pm.NotifyCLsUpdated(ctx, luciProject, changelist.ToUpdatedEvents(cls...))
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
