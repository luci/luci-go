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

package common

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestState(t *testing.T) {
	t.Parallel()

	Convey("Name", t, func() {
		So(State(-1).Name(), ShouldEqual, "invalid state")
		So(State(0).Name(), ShouldEqual, "")
		So(State_STATE_UNSPECIFIED.Name(), ShouldEqual, "")
		So(State_FREE.Name(), ShouldEqual, "free")
		So(State_PRERELEASE.Name(), ShouldEqual, "prerelease")
		So(State_SERVING.Name(), ShouldEqual, "serving")
		So(State_TEST.Name(), ShouldEqual, "test")
		So(State_REPAIR.Name(), ShouldEqual, "repair")
		So(State_DECOMMISSIONED.Name(), ShouldEqual, "decommissioned")
	})
}

func TestGetStates(t *testing.T) {
	t.Parallel()

	Convey("no match", t, func() {
		_, err := GetState("invalid state")
		So(err, ShouldErrLike, "did not match any known state")
	})

	Convey("exact match", t, func() {
		s, err := GetState("")
		So(err, ShouldBeNil)
		So(s, ShouldEqual, State_STATE_UNSPECIFIED)
		s, err = GetState("free")
		So(err, ShouldBeNil)
		So(s, ShouldEqual, State_FREE)
		s, err = GetState("prerelease")
		So(err, ShouldBeNil)
		So(s, ShouldEqual, State_PRERELEASE)
		s, err = GetState("serving")
		So(err, ShouldBeNil)
		So(s, ShouldEqual, State_SERVING)
		s, err = GetState("test")
		So(err, ShouldBeNil)
		So(s, ShouldEqual, State_TEST)
		s, err = GetState("repair")
		So(err, ShouldBeNil)
		So(s, ShouldEqual, State_REPAIR)
		s, err = GetState("decommissioned")
		So(err, ShouldBeNil)
		So(s, ShouldEqual, State_DECOMMISSIONED)
	})

	Convey("case match", t, func() {
		s, err := GetState("Free")
		So(err, ShouldBeNil)
		So(s, ShouldEqual, State_FREE)
		s, err = GetState("FREE")
		So(err, ShouldBeNil)
		So(s, ShouldEqual, State_FREE)
		s, err = GetState("FrEe")
		So(err, ShouldBeNil)
		So(s, ShouldEqual, State_FREE)
	})

	Convey("prefix match", t, func() {
		s, err := GetState("f")
		So(err, ShouldBeNil)
		So(s, ShouldEqual, State_FREE)
		s, err = GetState("fr")
		So(err, ShouldBeNil)
		So(s, ShouldEqual, State_FREE)
		s, err = GetState("fre")
		So(err, ShouldBeNil)
		So(s, ShouldEqual, State_FREE)
		s, err = GetState("free")
		So(err, ShouldBeNil)
		So(s, ShouldEqual, State_FREE)
	})

	Convey("case prefix match", t, func() {
		s, err := GetState("F")
		So(err, ShouldBeNil)
		So(s, ShouldEqual, State_FREE)
		s, err = GetState("Fr")
		So(err, ShouldBeNil)
		So(s, ShouldEqual, State_FREE)
		s, err = GetState("FrE")
		So(err, ShouldBeNil)
		So(s, ShouldEqual, State_FREE)
		s, err = GetState("FrEe")
		So(err, ShouldBeNil)
		So(s, ShouldEqual, State_FREE)
	})
}
