// Copyright 2025 The LUCI Authors.
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

package tqtasks

import (
	"context"
	"fmt"
	"slices"

	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/gae/filter/txndefer"
	"go.chromium.org/luci/server/tq"
	"go.chromium.org/luci/server/tq/tqtesting"

	internalspb "go.chromium.org/luci/swarming/proto/internals"
	notificationspb "go.chromium.org/luci/swarming/server/notifications/taskspb"
	"go.chromium.org/luci/swarming/server/tasks/taskspb"
)

// TestingState allows to examine pending TQ tasks.
type TestingState struct {
	*Tasks

	sched *tqtesting.Scheduler
}

// TestingContext sets up a fake TQ implementation and registers all task
// handlers in it.
func TestingContext(ctx context.Context) (context.Context, *TestingState) {
	// This is needed by tq.Dispatcher.AddTask.
	ctx = txndefer.FilterRDS(ctx)

	// A test-local dispatcher that would just collect pending TQ tasks.
	disp := &tq.Dispatcher{}
	ctx, sched := tq.TestingContext(ctx, disp)

	return ctx, &TestingState{
		// Need to register all potentially used TQ tasks, since only registered
		// tasks can be submitted. They won't have implementations attached to them
		// by default (but tests can attach them if necessary).
		Tasks: Register(disp),
		sched: sched,
	}
}

// Pending returns a representation of pending tasks of the given class.
//
// Usually called like e.g. `s.Pending(s.PubSubNotify)`.
func (s *TestingState) Pending(ref tq.TaskClassRef) []string {
	cls := ref.Definition().ID
	var out []string
	for _, tsk := range s.sched.Tasks().Pending() {
		if tsk.Class == cls {
			out = append(out, taskPayloadToStr(tsk.Payload))
		}
	}
	return out
}

// PendingAll returns a representation of all pending tasks across all classes.
func (s *TestingState) PendingAll() []string {
	var out []string
	for _, tsk := range s.sched.Tasks().Pending() {
		out = append(out, taskPayloadToStr(tsk.Payload))
	}
	return out
}

// Payloads returns protobuf payloads of all pending tasks across all classes.
func (s *TestingState) Payloads() []proto.Message {
	return s.sched.Tasks().Payloads()
}

func taskPayloadToStr(m proto.Message) string {
	switch m := m.(type) {
	case *internalspb.CancelRBETask:
		return fmt.Sprintf("%s/%s", m.RbeInstance, m.ReservationId)
	case *internalspb.EnqueueRBETask:
		return fmt.Sprintf("%s/%s", m.RbeInstance, m.Payload.ReservationId)
	case *taskspb.CancelChildrenTask:
		return m.TaskId
	case *taskspb.BatchCancelTask:
		return fmt.Sprintf("%q, purpose: %s, retry # %d", slices.Sorted(slices.Values(m.Tasks)), m.Purpose, m.Retries)
	case *notificationspb.PubSubNotifyTask:
		return m.TaskId
	case *notificationspb.BuildbucketNotifyTask:
		return m.TaskId
	case *taskspb.FinalizeTask:
		return m.TaskId
	default:
		return fmt.Sprintf("unexpected type %T", m)
	}
}
