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

package tasktest

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/golang/protobuf/proto"

	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/realms"

	"go.chromium.org/luci/scheduler/appengine/internal"
	"go.chromium.org/luci/scheduler/appengine/task"
)

// TimerSpec corresponds to single AddTimer call.
type TimerSpec struct {
	Delay   time.Duration
	Name    string
	Payload []byte
}

// TestController implements task.Controller and can be used in unit tests.
type TestController struct {
	OverrideJobID   string // return value of JobID() if not ""
	OverrideInvID   int64  // return value of InvocationID() if not 0
	OverrideRealmID string // return value of RealmID if not ""

	Req task.Request // return value of Request

	TaskMessage proto.Message // return value of Task
	TaskState   task.State    // return value of State(), mutated in place
	Client      *http.Client  // return value by GetClient()
	Log         []string      // individual log lines passed to DebugLog()

	SaveCallback         func() error                         // mock for Save()
	PrepareTopicCallback func(string) (string, string, error) // mock for PrepareTopic()

	Timers   []TimerSpec
	Triggers []*internal.Trigger
}

// JobID is part of Controller interface.
func (c *TestController) JobID() string {
	if c.OverrideJobID != "" {
		return c.OverrideJobID
	}
	return "some-project/some-job"
}

// InvocationID is part of Controller interface.
func (c *TestController) InvocationID() int64 {
	if c.OverrideInvID != 0 {
		return c.OverrideInvID
	}
	return 1
}

// RealmID is part of Controller interface.
func (c *TestController) RealmID() string {
	if c.OverrideRealmID != "" {
		return c.OverrideRealmID
	}
	return realms.Join("some-project", "some-realm")
}

// Request is part of Controller interface.
func (c *TestController) Request() task.Request {
	return c.Req
}

// Task is part of Controller interface.
func (c *TestController) Task() proto.Message {
	return c.TaskMessage
}

// State is part of Controller interface.
func (c *TestController) State() *task.State {
	return &c.TaskState
}

// AddTimer is part of Controller interface.
func (c *TestController) AddTimer(ctx context.Context, delay time.Duration, name string, payload []byte) {
	c.Timers = append(c.Timers, TimerSpec{
		Delay:   delay,
		Name:    name,
		Payload: payload,
	})
}

// DebugLog is part of Controller interface.
func (c *TestController) DebugLog(format string, args ...interface{}) {
	c.Log = append(c.Log, fmt.Sprintf(format, args...))
}

// Save is part of Controller interface.
func (c *TestController) Save(ctx context.Context) error {
	if c.SaveCallback != nil {
		return c.SaveCallback()
	}
	return errors.New("Save must not be called (not mocked)")
}

// PrepareTopic is part of Controller interface.
func (c *TestController) PrepareTopic(ctx context.Context, publisher string) (topic string, token string, err error) {
	if c.PrepareTopicCallback != nil {
		return c.PrepareTopicCallback(publisher)
	}
	return "", "", errors.New("PrepareTopic must not be called (not mocked)")
}

// GetClient is part of Controller interface.
func (c *TestController) GetClient(ctx context.Context, opts ...auth.RPCOption) (*http.Client, error) {
	if c.Client != nil {
		return c.Client, nil
	}
	return nil, errors.New("GetClient must not be called (not mocked)")
}

func (c *TestController) EmitTrigger(ctx context.Context, trigger *internal.Trigger) {
	c.Triggers = append(c.Triggers, trigger)
}

var _ task.Controller = (*TestController)(nil)
