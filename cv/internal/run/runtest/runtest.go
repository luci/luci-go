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

// Package runtest implements tests for working with Run Manager.
package runtest

import (
	"context"
	"sort"

	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/tq/tqtesting"

	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/cvtesting"
	"go.chromium.org/luci/cv/internal/eventbox"
	"go.chromium.org/luci/cv/internal/run"
	"go.chromium.org/luci/cv/internal/run/eventpb"

	. "github.com/smartystreets/goconvey/convey"
)

// Runs returns list of runs from tasks for Run Manager.
func Runs(in tqtesting.TaskList) (runs common.RunIDs) {
	for _, t := range in.SortByETA() {
		switch v := t.Payload.(type) {
		case *eventpb.ManageRunTask:
			runs = append(runs, common.RunID(v.GetRunId()))
		case *eventpb.KickManageRunTask:
			runs = append(runs, common.RunID(v.GetRunId()))
		}
	}
	return runs
}

// SortedRuns returns sorted list of runs from tasks for Run Manager.
func SortedRuns(in tqtesting.TaskList) common.RunIDs {
	runs := Runs(in)
	sort.Sort(runs)
	return runs
}

// Tasks returns all Run tasks sorted by ETA.
func Tasks(in tqtesting.TaskList) tqtesting.TaskList {
	ret := make(tqtesting.TaskList, 0, len(in))
	for _, t := range in.SortByETA() {
		switch t.Payload.(type) {
		case *eventpb.ManageRunTask, *eventpb.KickManageRunTask:
			ret = append(ret, t)
		}
	}
	return ret
}

func iterEventBox(ctx context.Context, runID common.RunID, cb func(*eventpb.Event)) {
	runKey := datastore.MakeKey(ctx, run.RunKind, string(runID))
	events, err := eventbox.List(ctx, runKey)
	So(err, ShouldBeNil)
	for _, item := range events {
		evt := &eventpb.Event{}
		So(proto.Unmarshal(item.Value, evt), ShouldBeNil)
		cb(evt)
	}
}

func matchEventBox(ctx context.Context, runID common.RunID, targets []*eventpb.Event) (matched, remaining []*eventpb.Event) {
	remaining = make([]*eventpb.Event, len(targets))
	copy(remaining, targets)
	iterEventBox(ctx, runID, func(evt *eventpb.Event) {
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

// AssertNotInEventbox asserts none of the target events exists in the Eventbox.
func AssertNotInEventbox(ctx context.Context, runID common.RunID, targets ...*eventpb.Event) {
	matched, _ := matchEventBox(ctx, runID, targets)
	So(matched, ShouldBeEmpty)
}

// AssertInEventbox asserts all target events exist in the Eventbox.
func AssertInEventbox(ctx context.Context, runID common.RunID, targets ...*eventpb.Event) {
	_, remaining := matchEventBox(ctx, runID, targets)
	So(remaining, ShouldBeEmpty)
}

// AssertReceivedCLUpdate asserts Run has received CLUpdated event for
// given CLID + EVersion.
func AssertReceivedCLUpdate(ctx context.Context, runID common.RunID, clid common.CLID, eversion int) {
	AssertInEventbox(ctx, runID, &eventpb.Event{
		Event: &eventpb.Event_ClUpdated{
			ClUpdated: &eventpb.CLUpdated{
				Clid:     int64(clid),
				EVersion: int64(eversion),
			},
		},
	})
}

// MockDispatch installs and returns MockDispatcher for Run Manager.
func MockDispatch(ctx context.Context) (context.Context, MockDispatcher) {
	m := MockDispatcher{&cvtesting.DispatchRecorder{}}
	ctx = eventpb.InstallMockDispatcher(ctx, m.Dispatch)
	return ctx, m
}

// MockDispatcher records in memory what would have resulted in task enqueues
// for a Run Manager.
type MockDispatcher struct {
	*cvtesting.DispatchRecorder
}

// Runs returns sorted list of Run IDs.
func (m *MockDispatcher) Runs() common.RunIDs {
	return common.MakeRunIDs(m.Targets()...)
}

// PopRuns returns sorted list of Run IDs and resets the state.
func (m *MockDispatcher) PopRuns() common.RunIDs {
	return common.MakeRunIDs(m.PopTargets()...)
}
