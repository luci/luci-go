// Copyright 2022 The LUCI Authors.
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

package base

import (
	"testing"

	"github.com/bazelbuild/buildtools/build"
	. "github.com/smartystreets/goconvey/convey"
)

func TestConvertOrderingToTable(t *testing.T) {
	t.Parallel()

	Convey("Return names ordered correctly", t, func() {
		//Initialize random string array of potential name orders
		nameOrdering := []string{
			"argument_name_ordering",
			"name",
			"branch_selector",
			"builder_spec",
			"mirrors",
			"try_settings",
			"triggered_by",
			"builder_group",
			"builderless",
			"cores",
			"os",
			"cpu",
			"ssd",
			"sheriff_rotations",
			"tree_closing",
			"console_view_entry",
			"main_console_view",
			"cq_mirrors_console_view",
			"list_view",
		}

		//Correct output
		convertOrderingCorrectOutput := map[string]int{
			"argument_name_ordering":  -19,
			"name":                    -18,
			"branch_selector":         -17,
			"builder_spec":            -16,
			"mirrors":                 -15,
			"try_settings":            -14,
			"triggered_by":            -13,
			"builder_group":           -12,
			"builderless":             -11,
			"cores":                   -10,
			"os":                      -9,
			"cpu":                     -8,
			"ssd":                     -7,
			"sheriff_rotations":       -6,
			"tree_closing":            -5,
			"console_view_entry":      -4,
			"main_console_view":       -3,
			"cq_mirrors_console_view": -2,
			"list_view":               -1,
		}

		So(convertOrderingToTable(nameOrdering), ShouldResemble, convertOrderingCorrectOutput)
	})
}

func TestConvertOrderTableEmptyArray(t *testing.T) {
	t.Parallel()

	Convey("Empty table should be returned if operating on empty table", t, func() {
		So(convertOrderingToTable([]string{}), ShouldResemble, map[string]int{})
	})
}

func TestRewriterFromConfig(t *testing.T) {
	t.Parallel()

	Convey("Receive valid rewriter back from rewriterFromConfig given valid nameOrdering", t, func() {
		nameOrdering := map[string]int{
			"name":    -2,
			"builder": -1,
		}

		var rewriter = &build.Rewriter{
			RewriteSet: []string{
				"listsort",
				"loadsort",
				"formatdocstrings",
				"reorderarguments",
				"editoctal",
			},
		}

		rewriter.NamePriority = nameOrdering
		rewriter.RewriteSet = append(rewriter.RewriteSet, "callsort")
		So(rewriterFromConfig(nameOrdering), ShouldResemble, rewriter)
	})
}

func TestRewriterFromConfigEmptyMap(t *testing.T) {
	t.Parallel()

	Convey("Nil parameter for rewriterFromConfig should return default Rewriter", t, func() {
		var rewriter = &build.Rewriter{
			RewriteSet: []string{
				"listsort",
				"loadsort",
				"formatdocstrings",
				"reorderarguments",
				"editoctal",
			},
		}
		So(rewriterFromConfig(nil), ShouldResemble, rewriter)
	})
}
