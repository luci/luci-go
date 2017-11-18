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

package tsmon

import (
	"errors"

	"golang.org/x/net/context"
)

// ErrNoTaskNumber is returned by NotifyTaskIsAlive if the task wasn't given
// a number yet.
var ErrNoTaskNumber = errors.New("no task number assigned yet")

// TaskNumAllocator is responsible for maintaining global mapping between task
// IDs and task numbers.
//
// The mapping is dynamic. Once a task dies (i.e. stops periodically call
// NotifyTaskIsAlive), its task number may be reused by some other (new) task.
type TaskNumAllocator interface {
	// NotifyTaskIsAlive is called periodically to make the allocator know the
	// given task is still up. The allocator responds with the currently assigned
	// task number or ErrNoTaskNumber if not yet assigned. Any other error should
	// be considered transient.
	NotifyTaskIsAlive(c context.Context, taskID string) (int, error)
}
