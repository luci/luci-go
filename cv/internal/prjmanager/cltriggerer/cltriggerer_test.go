// Copyright 2023 The LUCI Authors.
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

package cltriggerer

import (
	"context"
	"testing"

	"go.chromium.org/luci/cv/internal/cvtesting"
	"go.chromium.org/luci/cv/internal/prjmanager"
	"go.chromium.org/luci/cv/internal/prjmanager/prjpb"
	"go.chromium.org/luci/cv/internal/run"
	"go.chromium.org/luci/gae/service/datastore"
	"google.golang.org/protobuf/types/known/timestamppb"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestTriggerer(t *testing.T) {
	t.Parallel()

	const (
		project = "chromium"
		voter   = "voter@example.org"
		clid1   = 1
	)

	Convey("Triggerer", t, func() {
		ct := cvtesting.Test{}
		ctx, cancel := ct.SetUp(t)
		defer cancel()

		pmNotifier := prjmanager.NewNotifier(ct.TQDispatcher)
		triggerer := New(pmNotifier)
		task := &prjpb.TriggeringCLsTask{LuciProject: project}
		schedule := func() error {
			return datastore.RunInTransaction(ctx, func(tctx context.Context) error {
				return triggerer.Schedule(tctx, task)
			}, nil)
		}

		Convey("schedule works", func() {
			task.TriggeringCls = append(task.TriggeringCls, &prjpb.TriggeringCL{
				Clid:        clid1,
				OperationId: "deadbeef-123-defaeasd",
				Deadline:    timestamppb.New(ct.Clock.Now().Add(prjpb.MaxTriggeringCLDuration)),
				Trigger: &run.Trigger{
					Email: voter,
					Mode:  string(run.FullRun),
				},
			})
			So(schedule(), ShouldBeNil)
			So(ct.TQ.Tasks(), ShouldHaveLength, 1)
			So(ct.TQ.Tasks()[0].Payload, ShouldResembleProto, task)
		})
	})
}
