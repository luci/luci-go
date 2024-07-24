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
	"sort"
	"testing"

	"github.com/google/go-cmp/cmp"

	"go.chromium.org/luci/common/tsmon/monitor"

	apipb "go.chromium.org/luci/swarming/proto/api_v2"
	"go.chromium.org/luci/swarming/server/model"
)

func TestActiveJobsReporter(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	mon := &monitor.Fake{
		CS: 10000, // send all values via one chunk
	}

	r := &ActiveJobsReporter{
		ServiceName: "service",
		JobName:     "job",
		Monitor:     mon,
	}

	r.Prepare(ctx)

	fakeTasks := []struct {
		State      apipb.TaskState
		Spec       string
		Builder    string
		Exp        bool
		Project    string
		Subproject string
		Pool       string
		RBE        string
		Terminate  bool
	}{
		// Using "spec_name".
		{
			State:   apipb.TaskState_RUNNING,
			Spec:    "spec0",
			Builder: "ignored",
		},
		{
			State: apipb.TaskState_RUNNING,
			Spec:  "spec0",
		},
		{
			State: apipb.TaskState_RUNNING,
			Spec:  "spec1",
		},
		{
			State: apipb.TaskState_RUNNING,
			Spec:  "spec1",
		},
		{
			State: apipb.TaskState_PENDING,
			Spec:  "spec0",
		},
		{
			State: apipb.TaskState_PENDING,
			Spec:  "spec0",
		},

		// Using builder info.
		{
			State:   apipb.TaskState_RUNNING,
			Builder: "builder0",
		},
		{
			State:   apipb.TaskState_RUNNING,
			Builder: "builder0",
			Exp:     true,
		},

		// Termination task.
		{
			State:     apipb.TaskState_RUNNING,
			Terminate: true,
		},

		// Various other recognized tags.
		{
			State:      apipb.TaskState_RUNNING,
			Builder:    "b",
			Project:    "proj0",
			Subproject: "sub",
			Pool:       "pool",
			RBE:        "rbe",
		},
		{
			State:      apipb.TaskState_RUNNING,
			Builder:    "b",
			Project:    "proj1",
			Subproject: "sub",
			Pool:       "pool",
			RBE:        "rbe",
		},
	}

	for _, t := range fakeTasks {
		tags := []string{"ignore:me"}
		if t.Spec != "" {
			tags = append(tags, "spec_name:"+t.Spec)
		}
		if t.Builder != "" {
			tags = append(tags, "buildername:"+t.Builder)
		}
		if t.Exp {
			tags = append(tags, "build_is_experimental:true")
		} else {
			tags = append(tags, "build_is_experimental:false")
		}
		if t.Project != "" {
			tags = append(tags, "project:"+t.Project)
		}
		if t.Subproject != "" {
			tags = append(tags, "subproject:"+t.Subproject)
		}
		if t.Pool != "" {
			tags = append(tags, "pool:"+t.Pool)
		}
		if t.RBE != "" {
			tags = append(tags, "rbe:"+t.RBE)
		}
		if t.Terminate {
			tags = append(tags, "swarming.terminate:1")
		}
		sort.Strings(tags)
		r.Visit(ctx, &model.TaskResultSummary{
			TaskResultCommon: model.TaskResultCommon{State: t.State},
			Tags:             tags,
		})
	}

	if err := r.Finalize(ctx, nil); err != nil {
		t.Fatal(err)
	}

	want := []string{
		"b:proj0:sub:pool:rbe:running:1",
		"b:proj1:sub:pool:rbe:running:1",
		"builder0::::none:running:1",
		"builder0:experimental::::none:running:1",
		"spec0::::none:pending:2",
		"spec0::::none:running:2",
		"spec1::::none:running:2",
		"swarming:terminate::::none:running:1",
	}
	got := GlobalValues(t, mon.Cells, "jobs/active")
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("jobs/active (-want +got):\n%s", diff)
	}
}
