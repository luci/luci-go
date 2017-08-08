// Copyright 2015 The LUCI Authors.
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

package deps

import (
	"testing"

	ds "go.chromium.org/gae/service/datastore"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/dm/api/service/v1"
	"go.chromium.org/luci/dm/appengine/model"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestFinishAttempt(t *testing.T) {
	t.Parallel()

	Convey("FinishAttempt", t, func() {
		_, c, _, s := testSetup()

		So(ds.Put(c, &model.Quest{ID: "quest"}), ShouldBeNil)
		a := &model.Attempt{
			ID:           *dm.NewAttemptID("quest", 1),
			State:        dm.Attempt_EXECUTING,
			CurExecution: 1,
		}
		e := model.ExecutionFromID(c, dm.NewExecutionID("quest", 1, 1))
		ar := &model.AttemptResult{Attempt: ds.KeyForObj(c, a)}
		So(ds.Put(c, a), ShouldBeNil)
		So(ds.Put(c, &model.Execution{
			ID: 1, Attempt: ds.KeyForObj(c, a), Token: []byte("exKey"),
			State: dm.Execution_RUNNING}), ShouldBeNil)

		req := &dm.FinishAttemptReq{
			Auth: &dm.Execution_Auth{
				Id:    dm.NewExecutionID(a.ID.Quest, a.ID.Id, 1),
				Token: []byte("exKey"),
			},
			Data: dm.NewJsonResult(`{"something": "valid"}`, testclock.TestTimeUTC),
		}

		Convey("bad", func() {
			Convey("bad Token", func() {
				req.Auth.Token = []byte("fake")
				_, err := s.FinishAttempt(c, req)
				So(err, ShouldBeRPCPermissionDenied, "execution Auth")
			})

			Convey("not real json", func() {
				req.Data = dm.NewJsonResult(`i am not valid json`)
				_, err := s.FinishAttempt(c, req)
				So(err, ShouldErrLike, "invalid character 'i'")
			})
		})

		Convey("good", func() {
			_, err := s.FinishAttempt(c, req)
			So(err, ShouldBeNil)

			So(ds.Get(c, a, ar, e), ShouldBeNil)
			So(a.State, ShouldEqual, dm.Attempt_EXECUTING)
			So(e.State, ShouldEqual, dm.Execution_STOPPING)

			So(a.Result.Data.Size, ShouldEqual, 21)
			So(ar.Data.Size, ShouldEqual, 21)
		})

	})
}
