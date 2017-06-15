// Copyright 2017 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

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
			{[]string{"-log-level", "--", "-foo", "-bar"}, []string{"-log-level"}, []string{"--", "-foo", "-bar"}},

			{
				[]string{"-log-level", "debug", "--pants", "-spec=/foo", "-i", "-W"},
				[]string{"-log-level", "debug", "--pants", "-spec=/foo"},
				[]string{"-i", "-W"},
			},

			{
				[]string{"-i", "-log-level", "debug"},
				[]string{},
				[]string{"-i", "-log-level", "debug"},
			},

			{
				[]string{"-log-level", "--", "ohai"},
				[]string{"-log-level"},
				[]string{"--", "ohai"},
			},

			{
				[]string{"--log-level", "debug", "-d", "--", "script"},
				[]string{"--log-level", "debug"},
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
