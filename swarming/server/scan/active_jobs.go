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
	"strings"

	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/tsmon"
	"go.chromium.org/luci/common/tsmon/monitor"

	apipb "go.chromium.org/luci/swarming/proto/api_v2"
	"go.chromium.org/luci/swarming/server/metrics"
	"go.chromium.org/luci/swarming/server/model"
)

// ActiveJobsReporter is TaskVisitor that reports the number of active jobs per
// combination of dimensions to monitoring.
type ActiveJobsReporter struct {
	// ServiceName is a service name to put into metrics' target.
	ServiceName string
	// JobName is a job name to put into metrics' target.
	JobName string
	// Monitor to use to flush metrics.
	Monitor monitor.Monitor

	counts map[taskCounterKey]int64
}

var _ TaskVisitor = (*ActiveJobsReporter)(nil)

// Prepare prepares the visitor state.
//
// Part of TaskVisitor interface.
func (r *ActiveJobsReporter) Prepare(ctx context.Context) {
	r.counts = make(map[taskCounterKey]int64, 1000)
}

// Visit is called for every visited task.
//
// Part of TaskVisitor interface.
func (r *ActiveJobsReporter) Visit(ctx context.Context, trs *model.TaskResultSummary) {
	tagsMap := tagListToMap(trs.Tags)
	key := taskCounterKey{
		specName:     specName(tagsMap),
		projectID:    tagsMap["project"],
		subprojectID: tagsMap["subproject"],
		pool:         tagsMap["pool"],
		rbe:          tagsMap["rbe"],
		status:       taskResultSummaryStatus(trs.State),
	}
	if key.rbe == "" {
		key.rbe = "none"
	}
	r.counts[key] += 1
}

// Finalize is called once the scan is done.
//
// Part of TaskVisitor interface.
func (r *ActiveJobsReporter) Finalize(ctx context.Context, scanErr error) error {
	if scanErr != nil {
		return nil
	}
	logging.Infof(ctx, "Total number of points to report: %d", len(r.counts))
	state := newTSMonState(r.ServiceName, r.JobName, r.Monitor)
	mctx := tsmon.WithState(ctx, state)
	for key, val := range r.counts {
		metrics.JobsActives.Set(mctx, val, key.specName, key.projectID, key.subprojectID, key.pool, key.rbe, key.status)
	}
	return flushTSMonState(ctx, state)
}

////////////////////////////////////////////////////////////////////////////////

type taskCounterKey struct {
	specName     string // name of a job specification.
	projectID    string // e.g. "chromium".
	subprojectID string // e.g. "blink". Set to empty string if not used.
	pool         string // e.g. "Chrome".
	rbe          string // RBE instance of the task or literal "none".
	status       string // "pending", or "running".
}

// TODO(vadimsh): Stop allocating a map each time. We know exactly what keys we
// are going to read. We can just pick them out one by one in O(N) scan without
// allocating a map. This matters because this function is called O(1M) times in
// a tight loop and optimizing it may potentially noticeably reduce the overall
// loop duration.
func tagListToMap(tags []string) (tagsMap map[string]string) {
	tagsMap = make(map[string]string, len(tags))
	for _, tag := range tags {
		key, val, _ := strings.Cut(tag, ":")
		tagsMap[key] = val
	}
	return tagsMap
}

func specName(tagsMap map[string]string) string {
	if s := tagsMap["spec_name"]; s != "" {
		return s
	}
	b := tagsMap["buildername"]
	if tagsMap["build_is_experimental"] == "true" {
		b += ":experimental"
	}
	if b == "" {
		if tagsMap["terminate"] == "1" || tagsMap["swarming.terminate"] == "1" {
			return "swarming:terminate"
		}
	}
	return b
}

func taskResultSummaryStatus(s apipb.TaskState) string {
	switch s {
	case apipb.TaskState_RUNNING:
		return "running"
	case apipb.TaskState_PENDING:
		return "pending"
	default:
		return ""
	}
}
