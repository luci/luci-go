// Copyright 2020 The LUCI Authors.
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

package pmtest

import (
	"context"
	"sort"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/testing/truth"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/server/tq/tqtesting"

	"go.chromium.org/luci/cv/internal/changelist"
	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/common/eventbox"
	"go.chromium.org/luci/cv/internal/cvtesting"
	"go.chromium.org/luci/cv/internal/prjmanager"
	"go.chromium.org/luci/cv/internal/prjmanager/prjpb"
	"go.chromium.org/luci/cv/internal/run"
)

// Projects returns list of projects from tasks for PM.
func Projects(in tqtesting.TaskList) (projects []string) {
	for _, t := range in.SortByETA() {
		switch v := t.Payload.(type) {
		case *prjpb.ManageProjectTask:
			projects = append(projects, v.GetLuciProject())
		case *prjpb.KickManageProjectTask:
			projects = append(projects, v.GetLuciProject())
		}
	}
	return projects
}

func iterEventBox(t testing.TB, ctx context.Context, project string, cb func(*prjpb.Event)) {
	t.Helper()

	events, err := eventbox.List(ctx, prjmanager.EventboxRecipient(ctx, project))
	assert.NoErr(t, err)
	for _, item := range events {
		evt := &prjpb.Event{}
		assert.Loosely(t, proto.Unmarshal(item.Value, evt), should.BeNil, truth.LineContext())
		cb(evt)
	}
}

// ETAsOf returns sorted list of ETAs for a given project.
//
// Includes ETAs encoded in KickManageProjectTask tasks.
func ETAsOF(in tqtesting.TaskList, luciProject string) []time.Time {
	var out []time.Time
	for _, t := range in {
		switch v := t.Payload.(type) {
		case *prjpb.ManageProjectTask:
			if v.GetLuciProject() == luciProject {
				out = append(out, t.ETA)
			}
		case *prjpb.KickManageProjectTask:
			if v.GetLuciProject() == luciProject {
				out = append(out, v.GetEta().AsTime())
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Before(out[j]) })
	return out
}

// LatestETAof returns time.Time of the latest task for a given project.
//
// Includes ETAs encoded in KickManageProjectTask tasks.
// If none, returns Zero time.Time{}.
func LatestETAof(in tqtesting.TaskList, luciProject string) time.Time {
	ts := ETAsOF(in, luciProject)
	if len(ts) == 0 {
		return time.Time{}
	}
	return ts[len(ts)-1]
}

// ETAsWithin returns sorted list of ETAs for a given project in t+-d range.
func ETAsWithin(in tqtesting.TaskList, luciProject string, d time.Duration, t time.Time) []time.Time {
	out := ETAsOF(in, luciProject)
	for len(out) > 0 && out[0].Before(t.Add(-d)) {
		out = out[1:]
	}
	for len(out) > 0 && out[len(out)-1].After(t.Add(d)) {
		out = out[:len(out)-1]
	}
	return out
}

func matchEventBox(t testing.TB, ctx context.Context, project string, targets []*prjpb.Event) (matched, remaining []*prjpb.Event) {
	t.Helper()

	remaining = make([]*prjpb.Event, len(targets))
	copy(remaining, targets)
	iterEventBox(t, ctx, project, func(evt *prjpb.Event) {
		for i, r := range remaining {
			if proto.Equal(evt, r) {
				matched = append(matched, r)
				remaining[i] = remaining[len(remaining)-1]
				remaining[len(remaining)-1] = nil
				remaining = remaining[:len(remaining)-1]
				return
			}
		}
	})
	return
}

// AssertNotInEventbox asserts none of the events exists in the project
// Eventbox.
func AssertNotInEventbox(t testing.TB, ctx context.Context, project string, targets ...*prjpb.Event) {
	t.Helper()

	matched, _ := matchEventBox(t, ctx, project, targets)
	assert.Loosely(t, matched, should.BeEmpty, truth.LineContext())
}

// AssertInEventbox asserts all events exist in the project Eventbox.
func AssertInEventbox(t testing.TB, ctx context.Context, project string, targets ...*prjpb.Event) {
	t.Helper()

	_, remaining := matchEventBox(t, ctx, project, targets)
	assert.Loosely(t, remaining, should.BeEmpty, truth.LineContext())
}

// AssertReceivedRunFinished asserts a RunFinished event has been delivered
// tor project's eventbox for the given Run.
func AssertReceivedRunFinished(t testing.TB, ctx context.Context, runID common.RunID, status run.Status) {
	t.Helper()

	AssertInEventbox(t, ctx, runID.LUCIProject(), &prjpb.Event{
		Event: &prjpb.Event_RunFinished{
			RunFinished: &prjpb.RunFinished{
				RunId:  string(runID),
				Status: status,
			},
		},
	})
}

// AssertCLsUpdated asserts all events exist in the project Eventbox.
func AssertReceivedCLsNotified(t testing.TB, ctx context.Context, project string, cls []*changelist.CL) {
	t.Helper()

	AssertInEventbox(t, ctx, project, &prjpb.Event{
		Event: &prjpb.Event_ClsUpdated{
			ClsUpdated: changelist.ToUpdatedEvents(cls...),
		},
	})
}

// MockDispatch installs and returns MockDispatcher for PM.
func MockDispatch(ctx context.Context) (context.Context, MockDispatcher) {
	m := MockDispatcher{&cvtesting.DispatchRecorder{}}
	ctx = prjpb.InstallMockDispatcher(ctx, m.Dispatch)
	return ctx, m
}

// MockDispatcher records in memory what would have resulted in task enqueues
// for a PM.
type MockDispatcher struct {
	*cvtesting.DispatchRecorder
}

// Projects returns sorted list of Projects.
func (m *MockDispatcher) Projects() []string { return m.Targets() }

// PopProjects returns sorted list of Projects and resets the state.
func (m *MockDispatcher) PopProjects() []string { return m.PopTargets() }
