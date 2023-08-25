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
	"strings"

	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/server/tq"

	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/prjmanager"
	"go.chromium.org/luci/cv/internal/prjmanager/prjpb"
)

// Triggerer triggers given CLs.
type Triggerer struct {
	pmNotifier *prjmanager.Notifier
}

// New creates a Triggerer.
func New(n *prjmanager.Notifier) *Triggerer {
	v := &Triggerer{
		pmNotifier: n,
	}
	n.TasksBinding.TriggerProjectCL.AttachHandler(
		func(ctx context.Context, payload proto.Message) error {
			task := payload.(*prjpb.TriggeringCLsTask)
			ctx = logging.SetFields(ctx, logging.Fields{
				"project": task.GetLuciProject(),
			})
			return common.TQifyError(ctx,
				errors.Annotate(v.trigger(ctx, task), "triggerer.trigger").Err())
		},
	)
	return v
}

// Schedule schedules a task for CQVoteTask.
func (tr *Triggerer) Schedule(ctx context.Context, t *prjpb.TriggeringCLsTask) error {
	cls := t.GetTriggeringCls()
	if len(cls) == 0 {
		return nil
	}
	var title strings.Builder
	fmt.Fprintf(&title, "%s/%s/%d-%d", t.GetLuciProject(), cls[0].GetOperationId(), len(cls), cls[0].GetClid())
	for _, cl := range cls[1:] {
		fmt.Fprintf(&title, ".%d", cl.GetClid())
	}

	return tr.pmNotifier.TasksBinding.TQDispatcher.AddTask(ctx, &tq.Task{
		Payload: t,
		Title:   title.String(),
		// Not allowed in a transaction
		DeduplicationKey: "",
	})
}

// trigger triggers the CLs.
func (tr *Triggerer) trigger(ctx context.Context, task *prjpb.TriggeringCLsTask) error {
	// TODO: implement me
	return nil
}
