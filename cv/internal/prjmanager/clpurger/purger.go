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

	"go.chromium.org/luci/cv/internal/changelist"
	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/gerrit/cancel"
	"go.chromium.org/luci/cv/internal/gerrit/trigger"
	"go.chromium.org/luci/cv/internal/gerrit/updater"
	"go.chromium.org/luci/cv/internal/migration/migrationcfg"
	"go.chromium.org/luci/cv/internal/prjmanager"
	"go.chromium.org/luci/cv/internal/prjmanager/prjpb"
)

func init() {
	prjpb.DefaultTaskRefs.PurgeProjectCL.AttachHandler(
		func(ctx context.Context, payload proto.Message) error {
			task := payload.(*prjpb.PurgeCLTask)
			err := PurgeCL(ctx, task)
			return common.TQifyError(ctx, err)
		},
	)
}

func Schedule(ctx context.Context, t *prjpb.PurgeCLTask) error {
	return tq.AddTask(ctx, &tq.Task{
		Payload: t,
		// No DeduplicationKey as these tasks are created transactionally by PM.
		Title: fmt.Sprintf("%s/%d/%s", t.GetLuciProject(), t.GetPurgingCl().GetClid(), t.GetPurgingCl().GetOperationId()),
	})
}

func PurgeCL(ctx context.Context, task *prjpb.PurgeCLTask) error {
	ctx = logging.SetField(ctx, "project", task.GetLuciProject())
	ctx = logging.SetField(ctx, "cl", task.GetPurgingCl().GetClid())
	now := clock.Now(ctx)

	switch yes, err := migrationcfg.IsCQDUsingMyRuns(ctx, task.GetLuciProject()); {
	case err != nil:
		return err
	case !yes:
		logging.Warningf(ctx, "this app isn't managing Runs for the project")
		// Don't notify PM immediately to give CQD more time do the same CL purge,
		// otherwise PM will re-create purge task.
		return notifyPM(ctx, task, now.Add(time.Minute))
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
		if err := purgeWithDeadline(dctx, task); err != nil {
			return err
		}
	}
	return notifyPM(ctx, task, time.Time{} /*wake up PM ASAP*/)
}

func notifyPM(ctx context.Context, task *prjpb.PurgeCLTask, eta time.Time) error {
	return prjmanager.NotifyPurgeCompleted(ctx, task.GetLuciProject(), task.GetPurgingCl().GetOperationId(), eta)
}

func purgeWithDeadline(ctx context.Context, task *prjpb.PurgeCLTask) error {
	cl := &changelist.CL{ID: common.CLID(task.GetPurgingCl().GetClid())}
	if err := datastore.Get(ctx, cl); err != nil {
		return errors.Annotate(err, "failed to load %d", cl.ID).Tag(transient.Tag).Err()
	}
	if !needsPurging(ctx, cl, task) {
		return nil
	}

	msg, err := formatMessage(ctx, task, cl)
	if err != nil {
		return errors.Annotate(err, "CL %d of project %q", cl.ID, task.GetLuciProject()).Err()
	}
	logging.Debugf(ctx, "procceding to purge CL due to\n%s", msg)
	err = cancel.Cancel(ctx, cancel.Input{
		LUCIProject:      task.GetLuciProject(),
		CL:               cl,
		LeaseDuration:    time.Minute,
		Notify:           cancel.VOTERS | cancel.OWNER,
		Requester:        "prjmanager/clpurger",
		Trigger:          task.GetTrigger(),
		Message:          msg,
		RunCLExternalIDs: nil, // there is no Run.
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

	// Refresh a CL.
	return updater.Refresh(ctx, &updater.RefreshGerritCL{
		LuciProject: task.GetLuciProject(),
		Host:        cl.Snapshot.GetGerrit().GetHost(),
		Change:      cl.Snapshot.GetGerrit().GetInfo().GetNumber(),
		ClidHint:    int64(cl.ID),
	})
}

func needsPurging(ctx context.Context, cl *changelist.CL, task *prjpb.PurgeCLTask) bool {
	if cl.Snapshot == nil {
		logging.Warningf(ctx, "CL without Snapshot can't be purged %s", task.GetReason())
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
	switch t := trigger.Find(ci); {
	case t == nil:
		logging.Debugf(ctx, "CL is no longer triggered")
		return false
	case !proto.Equal(t, task.GetTrigger()):
		logging.Debugf(ctx, "CL has different trigger \n%s\n, but expected\n %s", t, task.GetTrigger())
		return false
	}
	return true
}
