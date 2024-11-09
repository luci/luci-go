// Copyright 2015 The LUCI Authors.
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

// Package noop implements tasks that do nothing at all. Used for testing
// only.
package noop

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/golang/protobuf/proto"
	"google.golang.org/api/pubsub/v1"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/config/validation"

	api "go.chromium.org/luci/scheduler/api/scheduler/v1"
	"go.chromium.org/luci/scheduler/appengine/internal"
	"go.chromium.org/luci/scheduler/appengine/messages"
	"go.chromium.org/luci/scheduler/appengine/task"
)

// TaskManager implements task.Manager interface for tasks defined with
// NoopTask proto message.
type TaskManager struct {
}

// Name is part of Manager interface.
func (m TaskManager) Name() string {
	return "noop"
}

// ProtoMessageType is part of Manager interface.
func (m TaskManager) ProtoMessageType() proto.Message {
	return (*messages.NoopTask)(nil)
}

// ValidateProtoMessage is part of Manager interface.
func (m TaskManager) ValidateProtoMessage(c *validation.Context, msg proto.Message, realmID string) {}

// Traits is part of Manager interface.
func (m TaskManager) Traits() task.Traits {
	return task.Traits{
		Multistage: false, // we don't use task.StatusRunning state
	}
}

// LaunchTask is part of Manager interface.
func (m TaskManager) LaunchTask(c context.Context, ctl task.Controller) error {
	cfg := ctl.Task().(*messages.NoopTask)

	sleepFor := time.Duration(cfg.SleepMs) * time.Millisecond
	if sleepFor == 0 {
		sleepFor = 20 * time.Second
	}
	ctl.DebugLog("Running noop task for %s", sleepFor)
	clock.Sleep(c, sleepFor)

	for i := int64(0); i < cfg.TriggersCount; i++ {
		ctl.EmitTrigger(c, &internal.Trigger{
			Id:      fmt.Sprintf("noop:%d:%d", ctl.InvocationID(), i),
			Payload: &internal.Trigger_Noop{Noop: &api.NoopTrigger{}},
		})
	}

	ctl.State().Status = task.StatusSucceeded
	return nil
}

// AbortTask is part of Manager interface.
func (m TaskManager) AbortTask(c context.Context, ctl task.Controller) error {
	return nil
}

// ExamineNotification is part of Manager interface.
func (m TaskManager) ExamineNotification(c context.Context, msg *pubsub.PubsubMessage) string {
	return ""
}

// HandleNotification is part of Manager interface.
func (m TaskManager) HandleNotification(c context.Context, ctl task.Controller, msg *pubsub.PubsubMessage) error {
	return errors.New("not implemented")
}

// HandleTimer is part of Manager interface.
func (m TaskManager) HandleTimer(c context.Context, ctl task.Controller, name string, payload []byte) error {
	return nil
}

// GetDebugState is part of Manager interface.
func (m TaskManager) GetDebugState(c context.Context, ctl task.ControllerReadOnly) (*internal.DebugManagerState, error) {
	return nil, fmt.Errorf("no debug state")
}
