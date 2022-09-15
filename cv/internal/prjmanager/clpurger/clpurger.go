// Copyright 2021 The LUCI Authors.
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

package clpurger

import (
	"context"
	"fmt"
	"time"

	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/tq"

	cfgpb "go.chromium.org/luci/cv/api/config/v2"
	"go.chromium.org/luci/cv/internal/changelist"
	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/configs/prjcfg"
	"go.chromium.org/luci/cv/internal/gerrit"
	"go.chromium.org/luci/cv/internal/gerrit/cancel"
	"go.chromium.org/luci/cv/internal/gerrit/trigger"
	"go.chromium.org/luci/cv/internal/prjmanager"
	"go.chromium.org/luci/cv/internal/prjmanager/prjpb"
	"go.chromium.org/luci/cv/internal/run"
	"go.chromium.org/luci/cv/internal/usertext"
)

// Purger purges CLs for Project Manager.
type Purger struct {
	pmNotifier *prjmanager.Notifier
	gFactory   gerrit.Factory
	clUpdater  clUpdater
	clMutator  *changelist.Mutator
}

// clUpdater is a subset of the *changelist.Updater which Purger needs.
type clUpdater interface {
	Schedule(context.Context, *changelist.UpdateCLTask) error
}

// New creates a Purger and registers it for handling tasks created by the given
// PM Notifier.
func New(n *prjmanager.Notifier, g gerrit.Factory, u clUpdater, clm *changelist.Mutator) *Purger {
	p := &Purger{n, g, u, clm}
	n.TasksBinding.PurgeProjectCL.AttachHandler(
		func(ctx context.Context, payload proto.Message) error {
			task := payload.(*prjpb.PurgeCLTask)
			ctx = logging.SetFields(ctx, logging.Fields{
				"project": task.GetLuciProject(),
				"cl":      task.GetPurgingCl().GetClid(),
			})
			err := p.PurgeCL(ctx, task)
			return common.TQifyError(ctx, err)
		},
	)
	return p
}

// Schedule enqueues a task to purge a CL for immediate execution.
func (p *Purger) Schedule(ctx context.Context, t *prjpb.PurgeCLTask) error {
	return p.pmNotifier.TasksBinding.TQDispatcher.AddTask(ctx, &tq.Task{
		Payload: t,
		// No DeduplicationKey as these tasks are created transactionally by PM.
		Title: fmt.Sprintf("%s/%d/%s", t.GetLuciProject(), t.GetPurgingCl().GetClid(), t.GetPurgingCl().GetOperationId()),
	})
}

// PurgeCL purges a CL and notifies PM on success or failure.
func (p *Purger) PurgeCL(ctx context.Context, task *prjpb.PurgeCLTask) error {
	now := clock.Now(ctx)

	if len(task.GetPurgeReasons()) == 0 && len(task.GetReasons()) == 0 {
		return errors.Reason("no reasons given in %s", task).Err()
	}
	pr := &prjpb.PurgeReason{}
	switch t := task.GetTrigger(); run.Mode(t.GetMode()) {
	case "":
		pr.ApplyTo = &prjpb.PurgeReason_AllActiveTriggers{AllActiveTriggers: true}
	case run.NewPatchsetRun:
		pr.ApplyTo = &prjpb.PurgeReason_Triggers{Triggers: &run.Triggers{
			NewPatchsetRunTrigger: t,
		}}
	case run.DryRun, run.QuickDryRun, run.FullRun:
		pr.ApplyTo = &prjpb.PurgeReason_Triggers{Triggers: &run.Triggers{
			NewPatchsetRunTrigger: t,
		}}
	default:
		panic(fmt.Errorf("unexpected trigger mode %q", t.GetMode()))
	}

	if len(task.GetReasons()) > 0 && len(task.GetPurgeReasons()) == 0 {
		for _, r := range task.GetReasons() {
			task.PurgeReasons = append(task.PurgeReasons, &prjpb.PurgeReason{
				ClError: r,
				ApplyTo: pr.GetApplyTo(),
			})
		}
	}

	d := task.GetPurgingCl().GetDeadline()
	if d == nil {
		return errors.Reason("no deadline given in %s", task).Err()
	}
	switch dt := d.AsTime(); {
	case dt.Before(now):
		logging.Warningf(ctx, "purging task running too late (deadline %s, now %s)", dt, now)
	default:
		dctx, cancel := clock.WithDeadline(ctx, dt)
		defer cancel()
		if err := p.purgeWithDeadline(dctx, task); err != nil {
			return err
		}
	}
	return p.pmNotifier.NotifyPurgeCompleted(ctx, task.GetLuciProject(), task.GetPurgingCl().GetOperationId())
}

func (p *Purger) purgeWithDeadline(ctx context.Context, task *prjpb.PurgeCLTask) error {
	cl := &changelist.CL{ID: common.CLID(task.GetPurgingCl().GetClid())}
	if err := datastore.Get(ctx, cl); err != nil {
		return errors.Annotate(err, "failed to load %d", cl.ID).Tag(transient.Tag).Err()
	}
	if !needsPurging(ctx, cl, task) {
		return nil
	}

	configGroups, err := loadConfigGroups(ctx, task)
	if err != nil {
		return nil
	}
	// TODO(robertocn): Use the trigger information in the purge reason.
	// Perform one cancellation per trigger in parallel.
	msg, err := usertext.SFormatPurgeReasons(ctx, task.GetPurgeReasons(), cl, run.Mode(task.GetTrigger().GetMode()))
	if err != nil {
		return errors.Annotate(err, "CL %d of project %q", cl.ID, task.GetLuciProject()).Err()
	}
	logging.Debugf(ctx, "proceeding to purge CL due to\n%s", msg)

	err = cancel.Cancel(ctx, cancel.Input{
		LUCIProject:       task.GetLuciProject(),
		CL:                cl,
		LeaseDuration:     time.Minute,
		Notify:            gerrit.Whoms{gerrit.Owner, gerrit.CQVoters},
		AddToAttentionSet: gerrit.Whoms{gerrit.Owner, gerrit.CQVoters},
		AttentionReason:   "CV can't start a new Run as requested",
		Requester:         "prjmanager/clpurger",
		Trigger:           task.GetTrigger(),
		Message:           msg,
		RunCLExternalIDs:  nil, // there is no Run.
		ConfigGroups:      configGroups,
		GFactory:          p.gFactory,
		CLMutator:         p.clMutator,
	})
	switch {
	case err == nil:
		logging.Debugf(ctx, "purging done")
	case cancel.ErrPreconditionFailedTag.In(err):
		logging.Debugf(ctx, "cancel is not necessary: %s", err)
	case cancel.ErrPermanentTag.In(err):
		logging.Errorf(ctx, "permanently failed to purge CL: %s", err)
	default:
		return errors.Annotate(err, "failed to purge CL %d of project %q", cl.ID, task.GetLuciProject()).Err()
	}

	// Schedule a refresh of a CL.
	// TODO(crbug.com/1284393): use Gerrit's consistency-on-demand when available.
	return p.clUpdater.Schedule(ctx, &changelist.UpdateCLTask{
		LuciProject: task.GetLuciProject(),
		ExternalId:  string(cl.ExternalID),
		Id:          int64(cl.ID),
		Requester:   changelist.UpdateCLTask_CL_PURGER,
	})
}

func needsPurging(ctx context.Context, cl *changelist.CL, task *prjpb.PurgeCLTask) bool {
	if cl.Snapshot == nil {
		logging.Warningf(ctx, "CL without Snapshot can't be purged\n%s", task)
		return false
	}
	if p := cl.Snapshot.GetLuciProject(); p != task.GetLuciProject() {
		logging.Warningf(ctx, "CL now belongs to different project %q", p)
		return false
	}
	if cl.Snapshot.GetGerrit() == nil {
		panic(fmt.Errorf("CL %d has non-Gerrit snapshot", cl.ID))
	}
	ci := cl.Snapshot.GetGerrit().GetInfo()
	// We can't avoid entirely races with users modifying the CL (e.g. removing CQ
	// votes). But do a best effort check that Trigger's timestamp is still the
	// same to the best of CV's knowledge. Note that this ignores potentially
	// configured additional modes such as run.QuickDryRun.
	switch t := trigger.Find(&trigger.FindInput{ChangeInfo: ci, ConfigGroup: &cfgpb.ConfigGroup{}}).GetCqVoteTrigger(); {
	case t == nil:
		logging.Debugf(ctx, "CL is no longer triggered")
		return false
	case !proto.Equal(t.GetTime(), task.GetTrigger().GetTime()):
		logging.Debugf(
			ctx,
			"CL has different trigger time \n%s\n, but expected\n %s",
			t.GetTime().AsTime(),
			task.GetTrigger().GetTime().AsTime(),
		)
		return false
	}
	return true
}

func loadConfigGroups(ctx context.Context, task *prjpb.PurgeCLTask) ([]*prjcfg.ConfigGroup, error) {
	// There is usually exactly 1 config group.
	res := make([]*prjcfg.ConfigGroup, len(task.GetConfigGroups()))
	for i, id := range task.GetConfigGroups() {
		cg, err := prjcfg.GetConfigGroup(ctx, task.GetLuciProject(), prjcfg.ConfigGroupID(id))
		if err != nil {
			return nil, errors.Annotate(err, "failed to load a ConfigGroup").Tag(transient.Tag).Err()
		}
		res[i] = cg
	}
	return res, nil
}
