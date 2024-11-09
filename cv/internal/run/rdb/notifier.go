// Copyright 2023 The LUCI Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package rdb

import (
	"context"
	"fmt"
	"time"

	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/common/sync/parallel"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/tq"

	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/run"
)

type Notifier struct {
	tqd *tq.Dispatcher
}

const notifierTaskClass = "rdb-notifier"

// NewNotifier creates a new Notifier, registering it to the given TQ dispatcher.
func NewNotifier(tqd *tq.Dispatcher, clientFactory RecorderClientFactory) *Notifier {
	notifier := &Notifier{tqd}

	tqd.RegisterTaskClass(tq.TaskClass{
		ID:        notifierTaskClass,
		Prototype: &MarkInvocationSubmittedTask{},
		// TODO(yiwzhang): migrate this workflow to longops that works similar to post action
		// This reuses the bq-export instead of creating a new one for rdb-notifier
		// as they should both be migrated to longops.
		Queue: "bq-export",
		Quiet: true,
		Kind:  tq.Transactional,
		Handler: func(ctx context.Context, payload proto.Message) error {
			task := payload.(*MarkInvocationSubmittedTask)
			err := MarkInvocationSubmitted(ctx, clientFactory, common.RunID(task.GetRunId()))
			return common.TQifyError(ctx, err)
		},
	})

	return notifier
}

// Schedule enqueues a task to mark the builds in the given run submitted to ResultDB.
func (n *Notifier) Schedule(ctx context.Context, id common.RunID) error {
	return n.tqd.AddTask(ctx, &tq.Task{
		Title:   string(id),
		Payload: &MarkInvocationSubmittedTask{RunId: string(id)},
	})
}

// MarkInvocationSubmitted marks all builds under the given run submitted through ResultDB.
func MarkInvocationSubmitted(ctx context.Context, clientFactory RecorderClientFactory, id common.RunID) error {
	r := &run.Run{ID: id}
	// Verify that the run exists.
	switch err := datastore.Get(ctx, r); {
	case err == datastore.ErrNoSuchEntity:
		// no action to take if the run doesn't exist.
		logging.Warningf(ctx, "run %s not exist to mark invocation submitted", id)
		return nil
	case err != nil:
		return errors.Annotate(err, "failed to fetch Run").Tag(transient.Tag).Err()
	}

	retryFactory := transient.Only(func() retry.Iterator {
		return &retry.ExponentialBackoff{
			Limited: retry.Limited{
				Delay:    1 * time.Second,
				MaxTotal: 5 * time.Minute,
			},
			Multiplier: 2,
		}
	})

	taskGenerator := func(work chan<- func() error) {
		for _, exec := range r.Tryjobs.GetState().GetExecutions() {
			for _, attempt := range exec.GetAttempts() {
				rdb := attempt.GetResult().GetBuildbucket().GetInfra().GetResultdb()
				if rdb.GetHostname() == "" || rdb.GetInvocation() == "" {
					logging.Infof(ctx, "rdb hostname %s and rdb invocation %s missing", rdb.GetHostname(), rdb.GetInvocation())
					continue
				}

				work <- func() error {
					err := retry.Retry(ctx, retryFactory, func() error {
						client, err := clientFactory.MakeClient(ctx, rdb.GetHostname())
						if err != nil {
							return err
						}

						if err := client.MarkInvocationSubmitted(ctx, rdb.GetInvocation()); err != nil {
							return errors.Annotate(err, "failed to mark invocation submitted").Err()
						}

						return nil
					}, retry.LogCallback(ctx, fmt.Sprintf("mark submitted [rdb hostname=%s, invocation=%s]", rdb.GetHostname(), rdb.GetInvocation())))

					return err
				}
			}
		}
	}

	if err := parallel.WorkPool(10, taskGenerator); err != nil {
		// Retries are handled in task through a 5 minute exponential backoff,
		// so unless we have issues reading the run, we can remove transient tags.
		return transient.Tag.Off().Apply(err)
	}
	return nil
}
