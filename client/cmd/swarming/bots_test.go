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

package main

import (
	"testing"

	"go.chromium.org/luci/auth"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestBotsParse(t *testing.T) {
	Convey(`Make sure that Parse fails with -mp and -nomp.`, t, func() {
		b := botsRun{}
		b.Init(auth.Options{})
		err := b.GetFlags().Parse([]string{"-server", "http://localhost:9050", "-mp", "-nomp"})
		So(err, ShouldBeNil)
		err = b.Parse()
		So(err, ShouldErrLike, "at most one of")
	})
	Convey(`Make sure that Parse fails with -quiet without -json.`, t, func() {
		b := botsRun{}
		b.Init(auth.Options{})
		err := b.GetFlags().Parse([]string{"-server", "http://localhost:9050", "-quiet"})
		So(err, ShouldBeNil)
		err = b.Parse()
		So(err, ShouldErrLike, "specify -json")
	})
}
