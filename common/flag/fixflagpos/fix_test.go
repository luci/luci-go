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

	"github.com/maruel/subcommands"
	. "github.com/smartystreets/goconvey/convey"
	"go.chromium.org/luci/common/cli"
)

func TestFixSubcommands(t *testing.T) {
	t.Parallel()

	s := func(args ...string) []string {
		return args
	}

	tests := []struct {
		Name string
		In   []string
		Fn   IsCommandFn
		Out  []string
	}{
		{Name: "empty"},

		{Name: "no func",
			In:  s("hello", "world", "-flag", "100"),
			Out: s("-flag", "100", "hello", "world")},

		{Name: "no flags",
			In:  s("hello", "world", "thing", "100"),
			Out: s("hello", "world", "thing", "100")},

		{Name: "single subcommand",
			In: s("hello", "world", "-flag", "100"),
			Fn: func(toks []string) bool {
				return len(toks) == 1 && toks[0] == "hello"
			},
			Out: s("hello", "-flag", "100", "world")},

		{Name: "MaruelSubcommandsFn",
			In: s("hello", "world", "-flag", "100"),
			Fn: MaruelSubcommandsFn(&cli.Application{
				Commands: []*subcommands.Command{
					{UsageLine: "hello world"},
				},
			}),
			Out: s("hello", "-flag", "100", "world")},
	}

	Convey(`FixSubcommands`, t, func() {
		for _, tc := range tests {
			tc := tc
			Convey(tc.Name, func() {
				So(FixSubcommands(tc.In, tc.Fn), ShouldResemble, tc.Out)

				if tc.Fn == nil {
					So(Fix(tc.In), ShouldResemble, tc.Out)
				}
			})
		}
	})
}
