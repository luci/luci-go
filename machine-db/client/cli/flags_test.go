// Copyright 2018 The LUCI Authors.
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

package cli

import (
	"testing"

	"go.chromium.org/luci/machine-db/api/common/v1"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestStateFlag(t *testing.T) {
	t.Parallel()

	Convey("stateFlag", t, func() {
		var flag stateFlag
		So(flag.Set("invalid state"), ShouldErrLike, "value must be a valid state")
		So(flag.String(), ShouldEqual, "")
		So(flag.Set(""), ShouldBeNil)
		So(flag.String(), ShouldEqual, "")
		So(flag.Set("free"), ShouldBeNil)
		So(flag.String(), ShouldEqual, "free")
		So(flag.Set("prerelease"), ShouldBeNil)
		So(flag.String(), ShouldEqual, "prerelease")
		So(flag.Set("serving"), ShouldBeNil)
		So(flag.String(), ShouldEqual, "serving")
		So(flag.Set("test"), ShouldBeNil)
		So(flag.String(), ShouldEqual, "test")
		So(flag.Set("repair"), ShouldBeNil)
		So(flag.String(), ShouldEqual, "repair")
		So(flag.Set("decommissioned"), ShouldBeNil)
		So(flag.String(), ShouldEqual, "decommissioned")
	})

	Convey("StateFlag", t, func() {
		var state common.State
		So(StateFlag(&state).Set("invalid state"), ShouldErrLike, "value must be a valid state")
		So(state, ShouldEqual, common.State_STATE_UNSPECIFIED)
		So(StateFlag(&state).Set(""), ShouldBeNil)
		So(state, ShouldEqual, common.State_STATE_UNSPECIFIED)
		So(StateFlag(&state).Set("free"), ShouldBeNil)
		So(state, ShouldEqual, common.State_FREE)
		So(StateFlag(&state).Set("prerelease"), ShouldBeNil)
		So(state, ShouldEqual, common.State_PRERELEASE)
		So(StateFlag(&state).Set("serving"), ShouldBeNil)
		So(state, ShouldEqual, common.State_SERVING)
		So(StateFlag(&state).Set("test"), ShouldBeNil)
		So(state, ShouldEqual, common.State_TEST)
		So(StateFlag(&state).Set("repair"), ShouldBeNil)
		So(state, ShouldEqual, common.State_REPAIR)
		So(StateFlag(&state).Set("decommissioned"), ShouldBeNil)
		So(state, ShouldEqual, common.State_DECOMMISSIONED)
	})
}
