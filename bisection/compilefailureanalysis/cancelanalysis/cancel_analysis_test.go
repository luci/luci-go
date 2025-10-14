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

	bbpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/bisection/internal/buildbucket"
	"go.chromium.org/luci/bisection/model"
	pb "go.chromium.org/luci/bisection/proto/v1"
	"go.chromium.org/luci/bisection/util/testutil"
)

func TestCancelAnalysis(t *testing.T) {
	t.Parallel()
	c := memory.Use(context.Background())
	testutil.UpdateIndices(c)
	cl := testclock.New(testclock.TestTimeUTC)
	c = clock.Set(c, cl)

	ctl := gomock.NewController(t)
	defer ctl.Finish()
	mc := buildbucket.NewMockedClient(c, ctl)
	c = mc.Ctx

	ftt.Run("Cancel Analysis", t, func(t *ftt.Test) {
		cfa := &model.CompileFailureAnalysis{
			Id:        123,
			Status:    pb.AnalysisStatus_RUNNING,
			RunStatus: pb.AnalysisRunStatus_STARTED,
		}
		assert.Loosely(t, datastore.Put(c, cfa), should.BeNil)
		datastore.GetTestable(c).CatchupIndexes()

		nsa := &model.CompileNthSectionAnalysis{
			Status:         pb.AnalysisStatus_RUNNING,
			RunStatus:      pb.AnalysisRunStatus_STARTED,
			ParentAnalysis: datastore.KeyForObj(c, cfa),
		}
		assert.Loosely(t, datastore.Put(c, nsa), should.BeNil)

		rb1 := &model.CompileRerunBuild{
			Id: 997,
		}
		rb2 := &model.CompileRerunBuild{
			Id: 998,
		}
		rb3 := &model.CompileRerunBuild{
			Id: 999,
		}
		assert.Loosely(t, datastore.Put(c, rb1), should.BeNil)
		assert.Loosely(t, datastore.Put(c, rb2), should.BeNil)
		assert.Loosely(t, datastore.Put(c, rb3), should.BeNil)
		datastore.GetTestable(c).CatchupIndexes()

		rr1 := &model.SingleRerun{
			RerunBuild: datastore.KeyForObj(c, rb1),
			Analysis:   datastore.KeyForObj(c, cfa),
			Status:     pb.RerunStatus_RERUN_STATUS_IN_PROGRESS,
		}
		rr2 := &model.SingleRerun{
			RerunBuild: datastore.KeyForObj(c, rb2),
			Analysis:   datastore.KeyForObj(c, cfa),
			Status:     pb.RerunStatus_RERUN_STATUS_FAILED,
		}
		rr3 := &model.SingleRerun{
			RerunBuild: datastore.KeyForObj(c, rb3),
			Analysis:   datastore.KeyForObj(c, cfa),
			Status:     pb.RerunStatus_RERUN_STATUS_IN_PROGRESS,
		}
		assert.Loosely(t, datastore.Put(c, rr1), should.BeNil)
		assert.Loosely(t, datastore.Put(c, rr2), should.BeNil)
		assert.Loosely(t, datastore.Put(c, rr3), should.BeNil)
		datastore.GetTestable(c).CatchupIndexes()

		mc.Client.EXPECT().CancelBuild(gomock.Any(), gomock.Any(), gomock.Any()).Return(&bbpb.Build{}, nil).Times(2)
		e := CancelAnalysis(c, 123)
		assert.Loosely(t, e, should.BeNil)

		datastore.GetTestable(c).CatchupIndexes()
		assert.Loosely(t, datastore.Get(c, cfa), should.BeNil)
		assert.Loosely(t, datastore.Get(c, nsa), should.BeNil)
		assert.Loosely(t, datastore.Get(c, rr1), should.BeNil)
		assert.Loosely(t, datastore.Get(c, rr3), should.BeNil)
		assert.Loosely(t, datastore.Get(c, rb1), should.BeNil)
		assert.Loosely(t, datastore.Get(c, rb3), should.BeNil)

		assert.Loosely(t, cfa.Status, should.Equal(pb.AnalysisStatus_NOTFOUND))
		assert.Loosely(t, cfa.RunStatus, should.Equal(pb.AnalysisRunStatus_CANCELED))
		assert.Loosely(t, nsa.Status, should.Equal(pb.AnalysisStatus_NOTFOUND))
		assert.Loosely(t, nsa.RunStatus, should.Equal(pb.AnalysisRunStatus_CANCELED))
		assert.Loosely(t, rr1.Status, should.Equal(pb.RerunStatus_RERUN_STATUS_CANCELED))
		assert.Loosely(t, rr3.Status, should.Equal(pb.RerunStatus_RERUN_STATUS_CANCELED))
		assert.Loosely(t, rb1.Status, should.Equal(bbpb.Status_CANCELED))
		assert.Loosely(t, rb3.Status, should.Equal(bbpb.Status_CANCELED))
	})
}
