// Copyright 2022 The LUCI Authors.
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

// Package cancelanalysis handles cancelation of existing analyses.
package cancelanalysis

import (
	"context"
	"testing"

	"github.com/golang/mock/gomock"
	. "github.com/smartystreets/goconvey/convey"

	bbpb "go.chromium.org/luci/buildbucket/proto"

	"go.chromium.org/luci/bisection/internal/buildbucket"
	"go.chromium.org/luci/bisection/model"
	pb "go.chromium.org/luci/bisection/proto"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
)

func TestCancelAnalysis(t *testing.T) {
	t.Parallel()
	c := memory.Use(context.Background())
	cl := testclock.New(testclock.TestTimeUTC)
	c = clock.Set(c, cl)

	ctl := gomock.NewController(t)
	defer ctl.Finish()
	mc := buildbucket.NewMockedClient(c, ctl)
	c = mc.Ctx

	Convey("Cancel Analysis", t, func() {
		cfa := &model.CompileFailureAnalysis{
			Id:        123,
			Status:    pb.AnalysisStatus_RUNNING,
			RunStatus: pb.AnalysisRunStatus_STARTED,
		}
		So(datastore.Put(c, cfa), ShouldBeNil)
		datastore.GetTestable(c).CatchupIndexes()

		nsa := &model.CompileNthSectionAnalysis{
			Status:         pb.AnalysisStatus_RUNNING,
			RunStatus:      pb.AnalysisRunStatus_STARTED,
			ParentAnalysis: datastore.KeyForObj(c, cfa),
		}
		So(datastore.Put(c, nsa), ShouldBeNil)

		rb1 := &model.CompileRerunBuild{
			Id: 997,
		}
		rb2 := &model.CompileRerunBuild{
			Id: 998,
		}
		rb3 := &model.CompileRerunBuild{
			Id: 999,
		}
		So(datastore.Put(c, rb1), ShouldBeNil)
		So(datastore.Put(c, rb2), ShouldBeNil)
		So(datastore.Put(c, rb3), ShouldBeNil)
		datastore.GetTestable(c).CatchupIndexes()

		rr1 := &model.SingleRerun{
			RerunBuild: datastore.KeyForObj(c, rb1),
			Analysis:   datastore.KeyForObj(c, cfa),
			Status:     pb.RerunStatus_IN_PROGRESS,
		}
		rr2 := &model.SingleRerun{
			RerunBuild: datastore.KeyForObj(c, rb2),
			Analysis:   datastore.KeyForObj(c, cfa),
			Status:     pb.RerunStatus_FAILED,
		}
		rr3 := &model.SingleRerun{
			RerunBuild: datastore.KeyForObj(c, rb3),
			Analysis:   datastore.KeyForObj(c, cfa),
			Status:     pb.RerunStatus_IN_PROGRESS,
		}
		So(datastore.Put(c, rr1), ShouldBeNil)
		So(datastore.Put(c, rr2), ShouldBeNil)
		So(datastore.Put(c, rr3), ShouldBeNil)
		datastore.GetTestable(c).CatchupIndexes()

		mc.Client.EXPECT().CancelBuild(gomock.Any(), gomock.Any(), gomock.Any()).Return(&bbpb.Build{}, nil).Times(2)
		e := CancelAnalysis(c, 123)
		So(e, ShouldBeNil)

		datastore.GetTestable(c).CatchupIndexes()
		So(datastore.Get(c, cfa), ShouldBeNil)
		So(datastore.Get(c, nsa), ShouldBeNil)
		So(cfa.Status, ShouldEqual, pb.AnalysisStatus_NOTFOUND)
		So(cfa.RunStatus, ShouldEqual, pb.AnalysisRunStatus_CANCELED)
		So(nsa.Status, ShouldEqual, pb.AnalysisStatus_NOTFOUND)
		So(nsa.RunStatus, ShouldEqual, pb.AnalysisRunStatus_CANCELED)
	})
}
