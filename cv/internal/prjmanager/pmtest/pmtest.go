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

// Package pmtest implements tests for working with Project Manager.
package pmtest

import (
	"context"

	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/tq/tqtesting"

	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/eventbox"
	"go.chromium.org/luci/cv/internal/prjmanager"
	"go.chromium.org/luci/cv/internal/prjmanager/prjpb"

	. "github.com/smartystreets/goconvey/convey"
)

// Projects returns list of projects from tasks for PM.
func Projects(in tqtesting.TaskList) (projects []string) {
	for _, t := range in.SortByETA() {
		switch v := t.Payload.(type) {
		case *prjpb.PokePMTask:
			projects = append(projects, v.GetLuciProject())
		case *prjpb.KickPokePMTask:
			projects = append(projects, v.GetLuciProject())
		}
	}
	return projects
}

func iterEventBox(ctx context.Context, project string, cb func(*prjpb.Event)) {
	projKey := datastore.MakeKey(ctx, prjmanager.ProjectKind, project)
	events, err := eventbox.List(ctx, projKey)
	So(err, ShouldBeNil)
	for _, item := range events {
		evt := &prjpb.Event{}
		So(proto.Unmarshal(item.Value, evt), ShouldBeNil)
		cb(evt)
	}
}

func matchEventBox(ctx context.Context, project string, targets []*prjpb.Event) (matched, remaining []*prjpb.Event) {
	remaining = make([]*prjpb.Event, len(targets))
	copy(remaining, targets)
	iterEventBox(ctx, project, func(evt *prjpb.Event) {
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
func AssertNotInEventbox(ctx context.Context, project string, targets ...*prjpb.Event) {
	matched, _ := matchEventBox(ctx, project, targets)
	So(matched, ShouldBeEmpty)
}

// AssertInEventbox asserts all events exist in the project Eventbox.
func AssertInEventbox(ctx context.Context, project string, targets ...*prjpb.Event) {
	_, remaining := matchEventBox(ctx, project, targets)
	So(remaining, ShouldBeEmpty)
}

// AssertReceivedRunFinished asserts a RunFinished event has been delivered
// tor project's eventbox for the given Run.
func AssertReceivedRunFinished(ctx context.Context, runID common.RunID) {
	AssertInEventbox(ctx, runID.LUCIProject(), &prjpb.Event{
		Event: &prjpb.Event_RunFinished{
			RunFinished: &prjpb.RunFinished{
				RunId: string(runID),
			},
		},
	})
}
