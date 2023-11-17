// Copyright 2023 The LUCI Authors.
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

package cltriggerer

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/common/sync/dispatcher"
	"go.chromium.org/luci/common/sync/dispatcher/buffer"
	"go.chromium.org/luci/server/tq"

	"go.chromium.org/luci/cv/internal/changelist"
	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/common/lease"
	"go.chromium.org/luci/cv/internal/gerrit"
	"go.chromium.org/luci/cv/internal/prjmanager"
	"go.chromium.org/luci/cv/internal/prjmanager/prjpb"
	"go.chromium.org/luci/cv/internal/run"
)

const maxConcurrency = 16

// Triggerer triggers given CLs.
type Triggerer struct {
	pmNotifier *prjmanager.Notifier
	gFactory   gerrit.Factory
	clUpdater  clUpdater
	clMutator  *changelist.Mutator
}

// clUpdater is a subset of the *changelist.Updater which Triggerer needs.
type clUpdater interface {
	Schedule(context.Context, *changelist.UpdateCLTask) error
}

// New creates a Triggerer.
func New(n *prjmanager.Notifier, gf gerrit.Factory, clu clUpdater, clm *changelist.Mutator) *Triggerer {
	v := &Triggerer{
		pmNotifier: n,
		gFactory:   gf,
		clUpdater:  clu,
		clMutator:  clm,
	}
	n.TasksBinding.TriggerProjectCLDeps.AttachHandler(
		func(ctx context.Context, payload proto.Message) error {
			task := payload.(*prjpb.TriggeringCLDepsTask)
			ctx = logging.SetField(ctx, "project", task.GetLuciProject())
			return common.TQifyError(ctx,
				errors.Annotate(v.process(ctx, task), "triggerer.process").Err())
		},
	)
	return v
}

// Schedule schedules a task for CQVoteTask.
func (tr *Triggerer) Schedule(ctx context.Context, t *prjpb.TriggeringCLDepsTask) error {
	payload := t.GetTriggeringClDeps()
	if len(payload.GetDepClids()) == 0 {
		return nil
	}
	return tr.pmNotifier.TasksBinding.TQDispatcher.AddTask(ctx, &tq.Task{
		Payload: t,
		Title: fmt.Sprintf("%s/%s/%d-%d",
			t.GetLuciProject(), payload.GetOperationId(),
			payload.GetOriginClid(), len(payload.GetDepClids())),
		// Not allowed in a transaction
		DeduplicationKey: "",
	})
}

func (tr *Triggerer) makeDispatcherChannel(ctx context.Context, task *prjpb.TriggeringCLDepsTask) dispatcher.Channel {
	concurrency := min(len(task.GetTriggeringClDeps().GetDepClids()), maxConcurrency)
	prj := task.GetLuciProject()
	dc, err := dispatcher.NewChannel(ctx, &dispatcher.Options{
		ErrorFn: func(failedBatch *buffer.Batch, err error) (retry bool) {
			_, isLeaseErr := lease.IsAlreadyInLeaseErr(err)
			return isLeaseErr || transient.Tag.In(err)
		},
		DropFn: dispatcher.DropFnQuiet,
		Buffer: buffer.Options{
			MaxLeases:     concurrency,
			BatchItemsMax: 1,
			FullBehavior: &buffer.BlockNewItems{
				MaxItems: concurrency,
			},
			Retry: makeRetryFactory(),
		},
	}, func(data *buffer.Batch) error {
		op, ok := data.Data[0].Item.(*triggerDepOp)
		if !ok {
			panic(fmt.Errorf("unexpected batch data item type %T", data.Data[0].Item))
		}
		ctx := logging.SetFields(ctx, logging.Fields{"cl": op.depCLID})
		return op.execute(ctx, tr.gFactory, prj, tr.clMutator, tr.clUpdater)
	})
	if err != nil {
		panic(fmt.Errorf("cltriggerer: unexpected failure in dispatcher creation"))
	}
	return dc
}

func (tr *Triggerer) process(ctx context.Context, task *prjpb.TriggeringCLDepsTask) error {
	payload := task.GetTriggeringClDeps()
	evt := &prjpb.TriggeringCLDepsCompleted{
		OperationId: payload.GetOperationId(),
		Origin:      payload.GetOriginClid(),
	}
	taskCtx, cancel := clock.WithDeadline(ctx, payload.GetDeadline().AsTime())
	defer cancel()

	// It's necessary to find the originating CL before executing any ops.
	originCL := &changelist.CL{ID: common.CLID(payload.GetOriginClid())}
	switch err := changelist.LoadCLs(taskCtx, []*changelist.CL{originCL}); errors.Unwrap(err) {
	case nil:
	case context.Canceled, context.DeadlineExceeded:
		evt.Incompleted = append(evt.Incompleted, payload.GetDepClids()...)
		// ctx instead of taskCtx.
		return tr.pmNotifier.NotifyTriggeringCLDepsCompleted(ctx, task.GetLuciProject(), evt)
	default:
		// always return a transient to retry fetching the originating CL
		// until the deadline exceeds.
		return transient.Tag.Apply(err)
	}

	// trigger votes in parallel while constantly checking the vote status
	// of the originating CL.
	var isCanceled atomic.Bool
	ops := makeTriggerDepOps(originCL.ExternalID.MustURL(), payload, &isCanceled)
	if ensureOriginCLVote(taskCtx, originCL) {
		go checkVoteStatus(taskCtx, payload.GetOriginClid(), &isCanceled)
		dc := tr.makeDispatcherChannel(taskCtx, task)
		for _, item := range ops {
			dc.C <- item
		}
		dc.Close()
		<-dc.DrainC
	} else {
		// no need, but just for the sake.
		isCanceled.Store(true)
	}

	for _, op := range ops {
		switch {
		case op.isSucceeded():
			// It's possible that the origin CQ vote no longer exists.
			// If so, OnTriggeringCLDepsCompleted() will check the origin vote
			// status, and schedule PurgingCLTask for the successfully voted
			// deps.
			evt.Succeeded = append(evt.Succeeded, op.depCLID)
		case op.isPermanentlyFailed():
			evt.Failed = append(evt.Failed, op.getCLError())
		default:
			evt.Incompleted = append(evt.Incompleted, op.depCLID)
		}
	}
	// ctx instead of taskCtx to send a notification even if the deadline
	// exceeds.
	return tr.pmNotifier.NotifyTriggeringCLDepsCompleted(ctx, task.GetLuciProject(), evt)
}

func makeRetryFactory() retry.Factory {
	return transient.Only(func() retry.Iterator {
		return &retry.ExponentialBackoff{
			Limited: retry.Limited{
				Delay:   100 * time.Millisecond,
				Retries: -1, // unlimited
			},
			Multiplier: 2,
			MaxDelay:   30 * time.Second,
		}
	})
}

func ensureOriginCLVote(ctx context.Context, originCL *changelist.CL) bool {
	switch mode := findCQTriggerMode(originCL); mode {
	case string(run.FullRun):
		return true
	case "":
		logging.Infof(ctx, "the origin CL %d no longer has CQ vote; stop voting", originCL.ID)
		return false
	default:
		// The originating CL now has CQ+1. This can only happen in the
		// following scenario.
		// - at t1, the origin CL gets CQ+2 and TriggeringCLDepsTask is created.
		// - at t2, the origin CL gets CQ+1, while or before the task process.
		//
		// This should be considered as cancelling the CQ vote chain
		// process. It's OK to skip all the vote ops for the dep CLs.
		// Then, PM will retriage the originating CL, as necessary.
		logging.Infof(ctx, "the origin CL %d now has a CQ vote for %q; stop voting", mode)
		return false
	}
}

func checkVoteStatus(ctx context.Context, originCLID int64, isCanceled *atomic.Bool) {
	originCL := &changelist.CL{ID: common.CLID(originCLID)}
	for {
		select {
		case <-ctx.Done():
			return
		case tr := <-clock.After(ctx, 4*time.Second):
			if tr.Err != nil {
				return
			}
		}
		if err := changelist.LoadCLs(ctx, []*changelist.CL{originCL}); err == nil {
			isCanceled.Store(ensureOriginCLVote(ctx, originCL))
		}
	}
}
