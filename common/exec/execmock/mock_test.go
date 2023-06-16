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

package execmock

import (
	"context"
	"testing"

	"go.chromium.org/luci/common/exec"
	"go.chromium.org/luci/common/exec/internal/execmockctx"
	"go.chromium.org/luci/common/system/environ"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestFilter(t *testing.T) {
	t.Parallel()

	Convey(`filter`, t, func() {
		Convey(`Args`, func() {
			Convey(`valid`, func() {
				e := filter{}.withArgs([]string{"hello", "/(there|you)/"})
				So(e.matches(&execmockctx.MockCriteria{Args: []string{"hello", "there"}}), ShouldBeTrue)
				So(e.matches(&execmockctx.MockCriteria{Args: []string{"hello", "you"}}), ShouldBeTrue)
				So(e.matches(&execmockctx.MockCriteria{Args: []string{"hello", "sir"}}), ShouldBeFalse)
			})

			Convey(`invalid`, func() {
				So(func() {
					filter{}.withArgs([]string{"/*/"})
				}, ShouldPanicLike, "invalid regexp")
			})
		})

		Convey(`Env`, func() {
			Convey(`valid`, func() {
				Convey(`single`, func() {
					e := filter{}.withEnv("SOMETHING", "/cool.beans/")
					So(e.matches(&execmockctx.MockCriteria{Env: environ.New([]string{"SOMETHING=cool beans"})}), ShouldBeTrue)
					So(e.matches(&execmockctx.MockCriteria{Env: environ.New([]string{"SOMETHING=cool?beans"})}), ShouldBeTrue)
					So(e.matches(&execmockctx.MockCriteria{Env: environ.New([]string{"SOMETHING=cool pintos"})}), ShouldBeFalse)

					Convey(`double`, func() {
						e = e.withEnv("OTHER", "nerds")
						So(e.matches(&execmockctx.MockCriteria{Env: environ.New([]string{"SOMETHING=cool beans"})}), ShouldBeFalse)
						So(e.matches(&execmockctx.MockCriteria{Env: environ.New([]string{"SOMETHING=cool beans", "OTHER=nerds"})}), ShouldBeTrue)
					})

					Convey(`negative`, func() {
						e = e.withEnv("OTHER", "!")
						So(e.matches(&execmockctx.MockCriteria{Env: environ.New([]string{"SOMETHING=cool beans"})}), ShouldBeTrue)
						So(e.matches(&execmockctx.MockCriteria{Env: environ.New([]string{"SOMETHING=cool beans", "OTHER=nerds"})}), ShouldBeFalse)
					})
				})
			})

			Convey(`invalid`, func() {
				So(func() {
					filter{}.withEnv("SOMETHING", "/*/")
				}, ShouldPanicLike, "invalid regexp")
			})
		})

		Convey(`Less`, func() {
			// TODO: Test me
		})
	})
}

func TestContext(t *testing.T) {
	t.Parallel()

	Convey(`context`, t, func() {
		ctx := Init(context.Background())

		Convey(`uninitialized`, func() {
			err := exec.Command(context.Background(), "echo", "hello").Run()
			So(err, ShouldErrLike, execmockctx.ErrNoMatchingMock)
			So(err, ShouldErrLike, "execmock.Init not called on context")
		})

		Convey(`zero`, func() {
			err := exec.Command(ctx, "echo", "hello").Run()
			So(err, ShouldErrLike, execmockctx.ErrNoMatchingMock)

			misses := ResetState(ctx)
			So(misses, ShouldHaveLength, 1)
			So(misses[0].Args, ShouldResemble, []string{"echo", "hello"})
		})

		Convey(`single`, func() {
			uses := Simple.Mock(ctx)

			err := exec.Command(ctx, "echo", "hello").Run()
			So(err, ShouldBeNil)

			usages := uses.Snapshot()
			So(usages, ShouldHaveLength, 1)
			So(usages[0].Args, ShouldResemble, []string{"echo", "hello"})
			So(usages[0].GetPID(), ShouldNotEqual, 0)
		})

		Convey(`multi`, func() {
			generalUsage := Simple.Mock(ctx)
			specificUsage := Simple.WithArgs("echo").Mock(ctx, SimpleInput{Stdout: "mocky mock"})

			out, err := exec.Command(ctx, "echo", "hello").CombinedOutput()
			So(err, ShouldBeNil)
			So(out, ShouldResemble, []byte("mocky mock"))

			So(generalUsage.Snapshot(), ShouldBeEmpty)

			usages := specificUsage.Snapshot()
			So(usages, ShouldHaveLength, 1)
			So(usages[0].Args, ShouldResemble, []string{"echo", "hello"})
			So(usages[0].GetPID(), ShouldNotEqual, 0)
		})

		Convey(`multi (limit)`, func() {
			generalUsage := Simple.Mock(ctx)
			specificUsage := Simple.WithArgs("echo").WithLimit(1).Mock(ctx, SimpleInput{Stdout: "mocky mock"})

			// fully consumes specificUsage
			out, err := exec.Command(ctx, "echo", "hello").CombinedOutput()
			So(err, ShouldBeNil)
			So(out, ShouldResemble, []byte("mocky mock"))

			// falls into generalUsage mock
			out, err = exec.Command(ctx, "echo", "hello").CombinedOutput()
			So(err, ShouldBeNil)
			So(out, ShouldResemble, []byte(""))

			So(generalUsage.Snapshot(), ShouldHaveLength, 1)
			So(specificUsage.Snapshot(), ShouldHaveLength, 1)
		})
	})
}

func TestGobName(t *testing.T) {
	t.Parallel()

	Convey(`gobName`, t, func() {
		So(gobName(SimpleInput{}), ShouldResemble, "go.chromium.org/luci/common/exec/execmock.SimpleInput")
		So(gobName(&SimpleInput{}), ShouldResemble, "*go.chromium.org/luci/common/exec/execmock.SimpleInput")

		So(gobName(100), ShouldResemble, "int")
	})
}
