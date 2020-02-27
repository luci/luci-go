// Copyright 2020 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package job

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestEditCIPDPkgs(t *testing.T) {
	t.Parallel()

	runCases(t, "EditCIPDPkgs", []testCase{
		{
			name: "nil",
			fn: func(jd *Definition) {
				SoEdit(jd, func(je Editor) {
					je.CIPDPkgs(nil)
				})
				So(must(jd.Info().CIPDPkgs()), ShouldBeEmpty)
			},
		},

		{
			name: "add",
			fn: func(jd *Definition) {
				SoEdit(jd, func(je Editor) {
					je.CIPDPkgs(CIPDPkgs{
						"subdir:some/pkg":       "version",
						"other_subdir:some/pkg": "different_version",
						"some/other/pkg":        "latest",
						":more/other/pkg":       "whatever",
					})
				})
				if sw := jd.GetSwarming(); sw != nil && len(sw.GetTask().GetTaskSlices()) == 0 {
					So(must(jd.Info().CIPDPkgs()), ShouldBeEmpty)
				} else {
					So(must(jd.Info().CIPDPkgs()), ShouldResemble, CIPDPkgs{
						"subdir:some/pkg":       "version",
						"other_subdir:some/pkg": "different_version",
						"some/other/pkg":        "latest",
						"more/other/pkg":        "whatever",
					})
				}
			},
		},

		{
			name: "delete",
			fn: func(jd *Definition) {
				SoEdit(jd, func(je Editor) {
					je.CIPDPkgs(CIPDPkgs{
						"subdir:some/pkg":       "version",
						"other_subdir:some/pkg": "different_version",
						"some/other/pkg":        "latest",
					})
				})
				SoEdit(jd, func(je Editor) {
					je.CIPDPkgs(CIPDPkgs{
						"subdir:some/pkg": "",
						"some/other/pkg":  "",
					})
				})

				if sw := jd.GetSwarming(); sw != nil && len(sw.GetTask().GetTaskSlices()) == 0 {
					So(must(jd.Info().CIPDPkgs()), ShouldBeEmpty)
				} else {
					So(must(jd.Info().CIPDPkgs()), ShouldResemble, CIPDPkgs{
						"other_subdir:some/pkg": "different_version",
					})
				}
			},
		},
	})
}
