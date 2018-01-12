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

package application

import (
	"flag"
	"fmt"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestExtractFlagsForSet(t *testing.T) {
	t.Parallel()

	Convey(`With a testing FlagSet`, t, func() {
		var app application
		fs := flag.NewFlagSet("", flag.ContinueOnError)
		app.addToFlagSet(fs)
		_ = fs.Bool("pants", false, "Random boolean flag.")

		for _, tc := range []struct {
			args  []string
			self  []string
			extra []string
		}{
			{[]string{}, nil, nil},
			{[]string{"-i"}, []string{}, []string{"-i"}},
			{[]string{"script", "-log-level", "debug"}, nil, []string{"script", "-log-level", "debug"}},
			{[]string{"-vpython-log-level", "--", "-foo", "-bar"}, []string{"-vpython-log-level"}, []string{"--", "-foo", "-bar"}},

			{
				[]string{"-vpython-log-level", "debug", "--pants", "-vpython-spec=/foo", "-i", "-W"},
				[]string{"-vpython-log-level", "debug", "--pants", "-vpython-spec=/foo"},
				[]string{"-i", "-W"},
			},

			{
				[]string{"-i", "-log-level", "debug"},
				[]string{},
				[]string{"-i", "-log-level", "debug"},
			},

			{
				[]string{"-vpython-log-level", "--", "ohai"},
				[]string{"-vpython-log-level"},
				[]string{"--", "ohai"},
			},

			{
				[]string{"--vpython-log-level", "debug", "-d", "--", "script"},
				[]string{"--vpython-log-level", "debug"},
				[]string{"-d", "--", "script"},
			},
		} {
			Convey(fmt.Sprintf(`Flags %v are split into %v and %v`, tc.args, tc.self, tc.extra), func() {
				self, extra := extractFlagsForSet(tc.args, fs)
				So(self, ShouldResemble, tc.self)
				So(extra, ShouldResemble, tc.extra)
			})
		}
	})
}
