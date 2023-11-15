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
	"strings"
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

// NoNotification indicates that no notification should be sent for the purging
// task on a given CL.
//
// this is just for readability.
var NoNotification = &prjpb.PurgingCL_Notification{}

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

	if len(task.GetPurgeReasons()) == 0 {
		return errors.Reason("no reasons given in %s", task).Err()
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
	return p.pmNotifier.NotifyPurgeCompleted(ctx, task.GetLuciProject(), task.GetPurgingCl())
}

func (p *Purger) purgeWithDeadline(ctx context.Context, task *prjpb.PurgeCLTask) error {
	cl := &changelist.CL{ID: common.CLID(task.GetPurgingCl().GetClid())}
	if err := datastore.Get(ctx, cl); err != nil {
		return errors.Annotate(err, "failed to load %d", cl.ID).Tag(transient.Tag).Err()
	}

	configGroups, err := loadConfigGroups(ctx, task)
	if err != nil {
		return nil
	}
	purgeTriggers, msg, err := triggersToPurge(ctx, configGroups[0].Content, cl, task)
	switch {
	case err != nil:
		return errors.Annotate(err, "CL %d of project %q", cl.ID, task.GetLuciProject()).Err()
	case purgeTriggers == nil:
		return nil
	}

	var atteWhoms gerrit.Whoms
	var notiWhoms gerrit.Whoms
	if notification := task.GetPurgingCl().GetNotification(); notification == nil {
		var whoms gerrit.Whoms
		if cqMode := purgeTriggers.GetCqVoteTrigger().GetMode(); cqMode != "" {
			whoms = append(whoms, run.Mode(cqMode).GerritNotifyTargets()...)
		}
		if nprMode := purgeTriggers.GetNewPatchsetRunTrigger().GetMode(); nprMode != "" {
			whoms = append(whoms, run.Mode(nprMode).GerritNotifyTargets()...)
		}
		if len(whoms) == 0 {
			panic(fmt.Errorf("expected the trigger(s) to purge to have a RunMode"))
		}
		whoms.Dedupe()
		atteWhoms = whoms
		notiWhoms = whoms
	} else {
		for _, whom := range notification.GetNotify() {
			notiWhoms = append(notiWhoms, gerrit.Whom(whom))
		}
		notiWhoms.Dedupe()
		for _, whom := range notification.GetAttention() {
			atteWhoms = append(atteWhoms, gerrit.Whom(whom))
		}
		atteWhoms.Dedupe()
	}

	logging.Debugf(ctx, "proceeding to purge CL due to\n%s", msg)
	err = trigger.Reset(ctx, trigger.ResetInput{LUCIProject: task.GetLuciProject(),
		CL:                cl,
		LeaseDuration:     time.Minute,
		Notify:            notiWhoms,
		AddToAttentionSet: atteWhoms,
		AttentionReason:   "CV can't start a new Run as requested",
		Requester:         "prjmanager/clpurger",
		Triggers:          purgeTriggers,
		Message:           msg,
		ConfigGroups:      configGroups,
		GFactory:          p.gFactory,
		CLMutator:         p.clMutator})

	switch {
	case err == nil:
		logging.Debugf(ctx, "purging done")
	case trigger.ErrResetPreconditionFailedTag.In(err):
		logging.Debugf(ctx, "cancel is not necessary: %s", err)
	case trigger.ErrResetPermanentTag.In(err):
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

func triggersToPurge(ctx context.Context, cg *cfgpb.ConfigGroup, cl *changelist.CL, task *prjpb.PurgeCLTask) (*run.Triggers, string, error) {
	if cl.Snapshot == nil {
		logging.Warningf(ctx, "CL without Snapshot can't be purged\n%s", task)
		return nil, "", nil
	}
	if p := cl.Snapshot.GetLuciProject(); p != task.GetLuciProject() {
		logging.Warningf(ctx, "CL now belongs to different project %q", p)
		return nil, "", nil
	}
	if cl.Snapshot.GetGerrit() == nil {
		panic(fmt.Errorf("CL %d has non-Gerrit snapshot", cl.ID))
	}
	ci := cl.Snapshot.GetGerrit().GetInfo()
	currentTriggers := trigger.Find(&trigger.FindInput{ChangeInfo: ci, ConfigGroup: cg, TriggerNewPatchsetRunAfterPS: cl.TriggerNewPatchsetRunAfterPS})
	if currentTriggers == nil {
		return nil, "", nil
	}

	ret := &run.Triggers{}
	var sb strings.Builder
	for _, pr := range task.GetPurgeReasons() {
		taskNPRTrigger := pr.GetTriggers().GetNewPatchsetRunTrigger()
		taskCQVTrigger := pr.GetTriggers().GetCqVoteTrigger()
		var clErrorMode run.Mode
		switch {
		case pr.GetAllActiveTriggers():
			ret = currentTriggers
			switch {
			// If multiple triggers are being purged, the mode of the CQ Vote
			// trigger takes precedence for the purposes of formatting.
			case ret.GetCqVoteTrigger() != nil:
				clErrorMode = run.Mode(ret.GetCqVoteTrigger().GetMode())
			case ret.GetNewPatchsetRunTrigger() != nil:
				clErrorMode = run.Mode(ret.GetNewPatchsetRunTrigger().GetMode())
			}
		// If we are purging a specific trigger, only proceed if the trigger to
		// purge has not been updated since the task was scheduled.
		// Note that we can't entirely avoid races with users modifying the CL.
		case taskNPRTrigger != nil && proto.Equal(currentTriggers.GetNewPatchsetRunTrigger().GetTime(), taskNPRTrigger.GetTime()):
			ret.NewPatchsetRunTrigger = taskNPRTrigger
			clErrorMode = run.Mode(taskNPRTrigger.GetMode())
		case taskCQVTrigger != nil && proto.Equal(currentTriggers.GetCqVoteTrigger().GetTime(), taskCQVTrigger.GetTime()):
			ret.CqVoteTrigger = taskCQVTrigger
			clErrorMode = run.Mode(taskCQVTrigger.GetMode())
		default:
			continue
		}
		if sb.Len() > 0 {
			sb.WriteRune('\n')
		}
		if err := usertext.FormatCLError(ctx, pr.GetClError(), cl, clErrorMode, &sb); err != nil {
			return nil, "", err
		}
	}
	if ret.CqVoteTrigger == nil && ret.NewPatchsetRunTrigger == nil {
		return nil, "", nil
	}
	return ret, sb.String(), nil
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
