// Copyright 2017 The LUCI Authors.
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

package fixflagpos

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestFixSubcommands(t *testing.T) {
	t.Parallel()

	s := func(args ...string) []string {
		return args
	}

	Convey(`Fix`, t, func() {
		tests := []struct {
			Name string
			In   []string
			Out  []string
		}{
			{Name: "empty"},

			{Name: "no func",
				In:  s("hello", "world", "-flag", "100"),
				Out: s("-flag", "100", "hello", "world")},

			{Name: "no flags",
				In:  s("hello", "world", "thing", "100"),
				Out: s("hello", "world", "thing", "100")},

			{Name: "positional after flags",
				In:  s("-flag", "100", "hello", "world"),
				Out: s("-flag", "100", "hello", "world")},

			{Name: "positional mixed with flags",
				In:  s("hello", "-flag", "100", "world"),
				Out: s("-flag", "100", "world", "hello")},

			{Name: "using --",
				In:  s("hello", "--", "-flag", "100", "world"),
				Out: s("--", "-flag", "100", "world", "hello")},
		}
		for _, tc := range tests {
			tc := tc
			Convey(tc.Name, func() {
				So(Fix(tc.In), ShouldResemble, tc.Out)
			})
		}
	})

	Convey(`FixSubcommands`, t, func() {
		tests := []struct {
			Name string
			In   []string
			Out  []string
		}{
			{Name: "empty"},

			{Name: "no flags",
				In:  s("hello", "world", "thing", "100"),
				Out: s("hello", "world", "thing", "100")},

			{Name: "single subcommand",
				In:  s("hello", "world", "-flag", "100"),
				Out: s("hello", "-flag", "100", "world")},
		}
		for _, tc := range tests {
			tc := tc
			Convey(tc.Name, func() {
				So(FixSubcommands(tc.In), ShouldResemble, tc.Out)
			})
		}
	})
}
