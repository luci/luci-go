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

	"golang.org/x/net/context"

	"go.chromium.org/luci/common/config/validation"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestState(t *testing.T) {
	t.Parallel()

	Convey("States", t, func() {
		So(Free, ShouldNotEqual, 0)
		So(Free, ShouldNotEqual, Prerelease)
		So(Free, ShouldNotEqual, Serving)
		So(Free, ShouldNotEqual, Test)
		So(Free, ShouldNotEqual, Repair)
		So(Free, ShouldNotEqual, Decommissioned)
		So(Prerelease, ShouldNotEqual, 0)
		So(Prerelease, ShouldNotEqual, Serving)
		So(Prerelease, ShouldNotEqual, Test)
		So(Prerelease, ShouldNotEqual, Repair)
		So(Prerelease, ShouldNotEqual, Decommissioned)
		So(Serving, ShouldNotEqual, 0)
		So(Serving, ShouldNotEqual, Test)
		So(Serving, ShouldNotEqual, Repair)
		So(Serving, ShouldNotEqual, Decommissioned)
		So(Test, ShouldNotEqual, 0)
		So(Test, ShouldNotEqual, Repair)
		So(Test, ShouldNotEqual, Decommissioned)
		So(Repair, ShouldNotEqual, 0)
		So(Repair, ShouldNotEqual, Decommissioned)
		So(Decommissioned, ShouldNotEqual, 0)
	})

	Convey("String", t, func() {
		So(Free.String(), ShouldEqual, "free")
		So(Prerelease.String(), ShouldEqual, "prerelease")
		So(Serving.String(), ShouldEqual, "serving")
		So(Test.String(), ShouldEqual, "test")
		So(Repair.String(), ShouldEqual, "repair")
		So(Decommissioned.String(), ShouldEqual, "decommissioned")
	})
}

func TestGetStates(t *testing.T) {
	t.Parallel()

	Convey("no match", t, func() {
		_, err := GetState("invalid state")
		So(err, ShouldErrLike, "did not match any known state")
		_, err = GetState("")
		So(err, ShouldErrLike, "did not match any known state")
	})

	Convey("exact match", t, func() {
		s, err := GetState("free")
		So(err, ShouldBeNil)
		So(s, ShouldEqual, Free)
		s, err = GetState("prerelease")
		So(err, ShouldBeNil)
		So(s, ShouldEqual, Prerelease)
		s, err = GetState("serving")
		So(err, ShouldBeNil)
		So(s, ShouldEqual, Serving)
		s, err = GetState("test")
		So(err, ShouldBeNil)
		So(s, ShouldEqual, Test)
		s, err = GetState("repair")
		So(err, ShouldBeNil)
		So(s, ShouldEqual, Repair)
		s, err = GetState("decommissioned")
		So(err, ShouldBeNil)
		So(s, ShouldEqual, Decommissioned)
	})

	Convey("prefix match", t, func() {
		s, err := GetState("f")
		So(err, ShouldBeNil)
		So(s, ShouldEqual, Free)
		s, err = GetState("fr")
		So(err, ShouldBeNil)
		s, err = GetState("fre")
		So(err, ShouldBeNil)
		So(s, ShouldEqual, Free)
		s, err = GetState("free")
		So(s, ShouldEqual, Free)
	})
}

func TestValidateState(t *testing.T) {
	t.Parallel()

	Convey("ValidateState", t, func() {
		context := &validation.Context{Context: context.Background()}

		Convey("empty", func() {
			ValidateState(context, "")
			So(context.Finalize(), ShouldBeNil)
		})

		Convey("invalid", func() {
			ValidateState(context, "invalid state")
			So(context.Finalize(), ShouldErrLike, "invalid state")
		})

		Convey("ok", func() {
			ValidateState(context, "free")
			So(context.Finalize(), ShouldBeNil)
		})
	})
}
