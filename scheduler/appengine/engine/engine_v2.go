// Copyright 2017 The LUCI Authors.
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

package engine

import (
	"errors"

	"golang.org/x/net/context"

	"go.chromium.org/luci/scheduler/appengine/task"
)

// jobControllerV2 implements jobController using v2 data structures.
type jobControllerV2 struct {
	eng *engineImpl
}

func (ctl *jobControllerV2) onJobScheduleChange(c context.Context, job *Job) error {
	return nil
}

func (ctl *jobControllerV2) onJobEnabled(c context.Context, job *Job) error {
	return nil
}

func (ctl *jobControllerV2) onJobDisabled(c context.Context, job *Job) error {
	return nil
}

func (ctl *jobControllerV2) onJobAbort(c context.Context, job *Job) (invs []int64, err error) {
	return nil, nil
}

func (ctl *jobControllerV2) onJobForceInvocation(c context.Context, job *Job) (FutureInvocation, error) {
	return nil, errors.New("not implemented")
}

func (ctl *jobControllerV2) onInvUpdating(c context.Context, old, fresh *Invocation, timers []invocationTimer, triggers []task.Trigger) error {
	return nil
}
