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
	"go.chromium.org/luci/cv/internal/eventbox"
	"go.chromium.org/luci/cv/internal/run"
	"go.chromium.org/luci/cv/internal/run/eventpb"

	. "github.com/smartystreets/goconvey/convey"
)

// Runs returns list of runs from tasks for Run Manager.
func Runs(in tqtesting.TaskList) (runs common.RunIDs) {
	for _, t := range in.SortByETA() {
		switch v := t.Payload.(type) {
		case *eventpb.PokeRunTask:
			runs = append(runs, common.RunID(v.GetRunId()))
		case *eventpb.KickPokeRunTask:
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

// Tasks returns all Run tasks sorted by ETA and then task class name.
func Tasks(in tqtesting.TaskList) []proto.Message {
	tasks := in[:]
	sort.SliceStable(tasks, func(i, j int) bool {
		l, r := tasks[i], tasks[j]
		if l.ETA.Equal(r.ETA) {
			return l.Class < r.Class
		}
		return l.ETA.Before(r.ETA)
	})

	ret := make([]proto.Message, 0, len(tasks))
	for _, t := range tasks {
		switch v := t.Payload.(type) {
		case *eventpb.PokeRunTask, *eventpb.KickPokeRunTask:
			ret = append(ret, v)
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

// AssertNotInEventbox asserts none of the target events exists in the Eventbox.
func AssertNotInEventbox(ctx context.Context, runID common.RunID, targets ...*eventpb.Event) {
	var hits []*eventpb.Event
	iterEventBox(ctx, runID, func(evt *eventpb.Event) {
		for i, target := range targets {
			if proto.Equal(evt, target) {
				hits = append(hits, target)
				targets[i] = targets[len(targets)-1]
				targets[len(targets)-1] = nil
				targets = targets[:len(targets)-1]
				return
			}
		}
	})
	So(hits, ShouldBeEmpty)
}

// AssertInEventbox asserts all target events exist in the Eventbox.
func AssertInEventbox(ctx context.Context, runID common.RunID, targets ...*eventpb.Event) {
	targets = targets[:]
	iterEventBox(ctx, runID, func(evt *eventpb.Event) {
		for i, target := range targets {
			if proto.Equal(evt, target) {
				targets[i] = targets[len(targets)-1]
				targets[len(targets)-1] = nil
				targets = targets[:len(targets)-1]
				return
			}
		}
	})
	So(targets, ShouldBeEmpty)
}
