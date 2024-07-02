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

package scan

import (
	"context"
	"slices"
	"testing"
	"time"

	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"

	apipb "go.chromium.org/luci/swarming/proto/api_v2"
	"go.chromium.org/luci/swarming/server/model"
)

func TestActiveTasks(t *testing.T) {
	t.Parallel()

	ctx := memory.Use(context.Background())
	datastore.GetTestable(ctx).AutoIndex(true)
	datastore.GetTestable(ctx).Consistent(true)

	var count int64

	putTask := func(name string, state apipb.TaskState) {
		reqKey, _ := model.TimestampToRequestKey(
			ctx,
			time.Date(2023, time.January, 1, 2, 3, 4, 0, time.UTC),
			count,
		)
		count++
		err := datastore.Put(ctx, &model.TaskResultSummary{
			Key:              model.TaskResultSummaryKey(ctx, reqKey),
			RequestName:      name,
			TaskResultCommon: model.TaskResultCommon{State: state},
		})
		if err != nil {
			t.Fatalf("Storing task: %s", err)
		}
	}

	putTask("running 0", apipb.TaskState_RUNNING)
	putTask("pending 0", apipb.TaskState_PENDING)

	putTask("expired", apipb.TaskState_EXPIRED)
	putTask("timed out", apipb.TaskState_TIMED_OUT)
	putTask("bot died", apipb.TaskState_BOT_DIED)
	putTask("canceled", apipb.TaskState_CANCELED)
	putTask("completed", apipb.TaskState_COMPLETED)
	putTask("killed", apipb.TaskState_KILLED)
	putTask("no resource", apipb.TaskState_NO_RESOURCE)
	putTask("client error", apipb.TaskState_CLIENT_ERROR)

	putTask("running 1", apipb.TaskState_RUNNING)
	putTask("pending 1", apipb.TaskState_PENDING)

	v1 := &testTaskVisitor{t: t}
	v2 := &testTaskVisitor{t: t}

	err := ActiveTasks(ctx, []TaskVisitor{v1, v2})
	if err != nil {
		t.Fatalf("%s", err)
	}

	// Sorted by (state, key).
	expected := []string{
		"running 1",
		"running 0",
		"pending 1",
		"pending 0",
	}
	v1.checkVisited(expected)
	v2.checkVisited(expected)
}

type testTaskVisitor struct {
	t         *testing.T
	prepared  bool
	finalized bool
	visited   []string
}

func (v *testTaskVisitor) Prepare(ctx context.Context) {
	if v.prepared {
		v.t.Error("Prepare called twice")
	}
	v.prepared = true
}

func (v *testTaskVisitor) Visit(ctx context.Context, task *model.TaskResultSummary) {
	if !v.prepared {
		v.t.Fatal("Prepare not called")
	}
	v.visited = append(v.visited, task.RequestName)
}

func (v *testTaskVisitor) Finalize(ctx context.Context, scanErr error) error {
	if v.finalized {
		v.t.Error("Finalize called twice")
	}
	v.finalized = true
	return nil
}

func (v *testTaskVisitor) checkVisited(expected []string) {
	if !v.finalized {
		v.t.Fatal("Finalize not called")
	}
	if !slices.Equal(v.visited, expected) {
		v.t.Errorf("Expected %v, got %v", expected, v.visited)
	}
}
