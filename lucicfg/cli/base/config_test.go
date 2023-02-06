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
	"path/filepath"
	"testing"

	"github.com/bazelbuild/buildtools/build"
	. "github.com/smartystreets/goconvey/convey"
	"go.chromium.org/luci/common/testing/testfs"
	"go.chromium.org/luci/lucicfg/buildifier"
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

func TestConfigParsing(t *testing.T) {
	root := t.TempDir()

	Convey("Config Parsing", t, func() {
		Convey("Duplicate paths in one rule in lucicfgfmtrc config should return error", func() {
			var configContent = `
				rules {
					path : "/"
					path : "/"
				}
				`

			layout := map[string]string{
				ConfigName: configContent,
			}

			if err := testfs.Build(root, layout); err != nil {
				t.Errorf(err.Error())
			}

			_, err := GetRewriterFactory(filepath.Join(root, ConfigName))
			So(err, ShouldBeError, "rule[0].path[1]: Found duplicate path '/'")
		})

		Convey("Multiple rules with same path in lucicfgfmtrc config should return error", func() {
			var configContent = `
				rules {
					path : "/"
				}
				rules {
					path : "/"
				}
				`

			layout := map[string]string{
				ConfigName: configContent,
			}

			if err := testfs.Build(root, layout); err != nil {
				t.Errorf(err.Error())
			}

			_, err := GetRewriterFactory(filepath.Join(root, ConfigName))
			So(err, ShouldBeError, "rule[1].path[0]: Found duplicate path '/'")
		})

		Convey("Backslash in lucicfgfmtrc config should return error", func() {
			var configContent = `
				rules {
					path : "\\"
				}
				`

			layout := map[string]string{
				ConfigName: configContent,
			}

			if err := testfs.Build(root, layout); err != nil {
				t.Errorf(err.Error())
			}
			_, err := GetRewriterFactory(filepath.Join(root, ConfigName))
			So(err, ShouldBeError, "rule[0].path[0]: Path should not contain backslash '\\'")
		})

		Convey("Make sure \"\" refers to root", func() {
			var configContent = `
				rules {
					path : ""
				}
				`

			layout := map[string]string{
				ConfigName:  configContent,
				"test.star": "",
			}

			if err := testfs.Build(root, layout); err != nil {
				t.Errorf(err.Error())
			}

			var sampleCallSortArgs = []string{
				"name1",
				"name2",
			}

			rewriterFactoryManualAlphanumeric, _ := getPostProcessedRewriterFactory(
				filepath.Join(root, ".lucicfgfmtrc"),
				&buildifier.LucicfgFmtConfig{
					Rules: []*buildifier.LucicfgFmtConfig_Rules{
						{ // Represents manual sorting and then alphanumeric sorting ruleset
							Path: []string{
								"",
							},
							FunctionArgsSort: &buildifier.LucicfgFmtConfig_Rules_FunctionArgsSort{
								Arg: sampleCallSortArgs,
							},
						},
					},
				},
			)

			rewriterManualAlphanumeric, _ := rewriterFactoryManualAlphanumeric.GetRewriter("test.star")
			var actualRewriterManualAlphanumeric = rewriterFromConfig(convertOrderingToTable(sampleCallSortArgs))
			So(rewriterManualAlphanumeric, ShouldResemble, actualRewriterManualAlphanumeric)
		})

		Convey("Differently normalized paths that are the same should result in an error", func() {
			var configContent = `
				rules {
					path : "something"
					path : "something/"
				}
				`
			layout := map[string]string{
				ConfigName: configContent,
			}

			if err := testfs.Build(root, layout); err != nil {
				t.Errorf(err.Error())
			}
			_, err := GetRewriterFactory(filepath.Join(root, ConfigName))
			So(err, ShouldBeError, "rule[0].path[1]: Found duplicate path 'something/'")
		})

		Convey("Rule doesn't contain any paths should throw an error", func() {
			var configContent = `
				rules {
				}
				`
			layout := map[string]string{
				ConfigName: configContent,
			}

			if err := testfs.Build(root, layout); err != nil {
				t.Errorf(err.Error())
			}
			_, err := GetRewriterFactory(filepath.Join(root, ConfigName))
			So(err, ShouldBeError, "rule[0]: Does not contain any paths")
		})
		Convey("Lucicfg file does not exit, should return default RewriterFactory", func() {
			defaultRewriter, _ := GetRewriterFactory("RANDOM_PATH/A/B/C/D")
			var rewriter = &RewriterFactory{
				rules:          []pathRules{},
				configFilePath: "",
			}
			So(defaultRewriter, ShouldResemble, rewriter)
		})
	})
}

func TestGetRewriter(t *testing.T) {
	root := t.TempDir()

	Convey("Testing GetRewriter", t, func() {
		// Initialize test Folders/Files
		layout := map[string]string{
			"test.star":               "",
			"a/test.star":             "",
			"a/b/test.star":           "",
			"a/b/c/test.star":         "",
			"no_rule_match/test.star": "",
		}

		if err := testfs.Build(root, layout); err != nil {
			t.Errorf(err.Error())
		}

		Convey("No matching path returns a rewriter that doesn't apply the callsort rewrite", func() {
			rewriterFactoryNoCallSort, _ := getPostProcessedRewriterFactory(
				filepath.Join(root, ".lucicfgfmtrc"),
				&buildifier.LucicfgFmtConfig{
					Rules: []*buildifier.LucicfgFmtConfig_Rules{},
				},
			)
			rewriterNoCallsort, _ := rewriterFactoryNoCallSort.GetRewriter("no_rule_match/test.star")
			var actualRewriterNoCallSort = rewriterFromConfig(nil)
			So(rewriterNoCallsort, ShouldResemble, actualRewriterNoCallSort)
		})

		Convey("Matching rule to nil FunctionArgSort, return rewriter without callsort rewrite", func() {
			rewriterFactoryNoCallSort, _ := getPostProcessedRewriterFactory(
				filepath.Join(root, ".lucicfgfmtrc"),
				&buildifier.LucicfgFmtConfig{
					Rules: []*buildifier.LucicfgFmtConfig_Rules{
						{ // Represents no callsort
							Path: []string{
								"a/b/c",
							},
						},
					},
				},
			)
			rewriterNoCallSort, _ := rewriterFactoryNoCallSort.GetRewriter("a/b/c/test.star")
			var actualRewriterNoCallSort = rewriterFromConfig(nil)
			So(rewriterNoCallSort, ShouldResemble, actualRewriterNoCallSort)
		})

		Convey("Matching rule to non-nil FunctionArgSort, has callsort + ordering", func() {
			var sampleCallSortArgs = []string{
				"name1",
				"name2",
			}
			rewriterFactoryManualAlphanumeric, _ := getPostProcessedRewriterFactory(
				filepath.Join(root, ".lucicfgfmtrc"),
				&buildifier.LucicfgFmtConfig{
					Rules: []*buildifier.LucicfgFmtConfig_Rules{
						{ // Represents manual sorting and then alphanumeric sorting ruleset
							Path: []string{
								"a",
							},
							FunctionArgsSort: &buildifier.LucicfgFmtConfig_Rules_FunctionArgsSort{
								Arg: sampleCallSortArgs,
							},
						},
					},
				},
			)
			rewriterManualAlphanumeric, _ := rewriterFactoryManualAlphanumeric.GetRewriter("a/test.star")
			var actualRewriterManualAlphanumeric = rewriterFromConfig(convertOrderingToTable(sampleCallSortArgs))
			So(rewriterManualAlphanumeric, ShouldResemble, actualRewriterManualAlphanumeric)
		})

		Convey("Matching to multiple rules, accepts the longest match", func() {
			var sampleCallSortArgs = []string{
				"name1",
				"name2",
			}
			rewriterFactoryAlphanumeric, _ := getPostProcessedRewriterFactory(
				filepath.Join(root, ".lucicfgfmtrc"),
				&buildifier.LucicfgFmtConfig{
					Rules: []*buildifier.LucicfgFmtConfig_Rules{
						{ // Represents manual sorting and then alphanumeric sorting ruleset
							Path: []string{
								"a",
							},
							FunctionArgsSort: &buildifier.LucicfgFmtConfig_Rules_FunctionArgsSort{
								Arg: sampleCallSortArgs,
							},
						},
						{ // Represents alphanumeric sorting
							Path: []string{
								"a/b",
							},
							FunctionArgsSort: &buildifier.LucicfgFmtConfig_Rules_FunctionArgsSort{},
						},
					},
				},
			)
			rewriterAlphanumeric, _ := rewriterFactoryAlphanumeric.GetRewriter("a/b/test.star")
			var actualRewriterAlphanumeric = rewriterFromConfig(convertOrderingToTable([]string{}))
			So(rewriterAlphanumeric, ShouldResemble, actualRewriterAlphanumeric)
		})
	})
}
