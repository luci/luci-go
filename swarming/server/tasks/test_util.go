// Copyright 2024 The LUCI Authors.
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

package tasks

import (
	"context"
	"fmt"
	"sort"
	"sync"

	"go.chromium.org/luci/common/errors"

	"go.chromium.org/luci/swarming/server/cfg"
	"go.chromium.org/luci/swarming/server/model"
)

// LifecycleTasksForTests mocks tq tasks for task lifecycle management, only used for tests.
type LifecycleTasksForTests struct {
	m             sync.Mutex
	fakeTaskQueue map[string][]string
}

// EnqueueBatchCancel enqueues a tq task to cancel tasks in batch.
func (lt *LifecycleTasksForTests) EnqueueBatchCancel(ctx context.Context, batch []string, killRunning bool, purpose string, retries int32) error {
	sort.Strings(batch)
	lt.m.Lock()
	defer lt.m.Unlock()
	lt.fakeTaskQueue["cancel-tasks-go"] = append(lt.fakeTaskQueue["cancel-tasks-go"], fmt.Sprintf("%q, purpose: %s, retry # %d", batch, purpose, retries))
	return nil
}

func (lt *LifecycleTasksForTests) enqueueChildCancellation(ctx context.Context, taskID string) error {
	lt.m.Lock()
	defer lt.m.Unlock()
	lt.fakeTaskQueue["cancel-children-tasks-go"] = append(lt.fakeTaskQueue["cancel-children-tasks-go"], taskID)
	return nil
}

func (lt *LifecycleTasksForTests) enqueueRBECancel(ctx context.Context, tr *model.TaskRequest, ttr *model.TaskToRun) error {
	lt.m.Lock()
	defer lt.m.Unlock()
	lt.fakeTaskQueue["rbe-cancel"] = append(lt.fakeTaskQueue["rbe-cancel"], fmt.Sprintf("%s/%s", tr.RBEInstance, ttr.RBEReservation))
	return nil
}

func (lt *LifecycleTasksForTests) enqueueRBENew(ctx context.Context, tr *model.TaskRequest, ttr *model.TaskToRun, _ *cfg.Config) error {
	lt.m.Lock()
	defer lt.m.Unlock()
	lt.fakeTaskQueue["rbe-new"] = append(lt.fakeTaskQueue["rbe-new"], fmt.Sprintf("%s/%s", tr.RBEInstance, ttr.RBEReservation))
	return nil
}

func (lt *LifecycleTasksForTests) sendOnTaskUpdate(ctx context.Context, tr *model.TaskRequest, trs *model.TaskResultSummary) error {
	taskID := model.RequestKeyToTaskID(tr.Key, model.AsRequest)
	if tr.PubSubTopic == "fail-the-task" {
		return errors.New("sorry, was told to fail it")
	}

	lt.m.Lock()
	defer lt.m.Unlock()
	if tr.PubSubTopic != "" {
		lt.fakeTaskQueue["pubsub-go"] = append(lt.fakeTaskQueue["pubsub-go"], taskID)
	}
	if tr.HasBuildTask {
		lt.fakeTaskQueue["buildbucket-notify-go"] = append(lt.fakeTaskQueue["buildbucket-notify-go"], taskID)
	}

	return nil
}

// PeekTasks returns all the tasks in the queue without popping them.
func (lt *LifecycleTasksForTests) PeekTasks(queue string) []string {
	lt.m.Lock()
	defer lt.m.Unlock()
	return append([]string(nil), lt.fakeTaskQueue[queue]...)
}

// PopTask pops a task from the queue.
func (lt *LifecycleTasksForTests) PopTask(queue string) string {
	res := lt.PopNTasks(queue, 1)
	if len(res) != 1 {
		return ""
	}
	return res[0]
}

// PopNTasks pops n tasks from the queue.
//
// If there are fewer than n tasks in the queue, just pop all of them.
func (lt *LifecycleTasksForTests) PopNTasks(qName string, n int) (res []string) {
	lt.m.Lock()
	defer lt.m.Unlock()
	l := len(lt.fakeTaskQueue[qName])
	if n >= l {
		n = l
	}
	res = lt.fakeTaskQueue[qName][:n]
	lt.fakeTaskQueue[qName] = lt.fakeTaskQueue[qName][n:]
	return
}

// MockTQTasks returns TestTQTasks with mocked tq functions for managing a task's lifecycle.
func MockTQTasks() *LifecycleTasksForTests {
	return &LifecycleTasksForTests{
		fakeTaskQueue: make(map[string][]string, 5),
	}
}
