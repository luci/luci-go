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

package testname

import (
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	"go.chromium.org/luci/analysis/internal/clustering"
	"go.chromium.org/luci/analysis/internal/clustering/rules/lang"
	"go.chromium.org/luci/analysis/internal/config/compiledcfg"
	configpb "go.chromium.org/luci/analysis/proto/config"
	pb "go.chromium.org/luci/analysis/proto/v1"
)

func TestAlgorithm(t *testing.T) {
	rules := []*configpb.TestNameClusteringRule{
		{
			Name:         "Blink Web Tests",
			Pattern:      `^ninja://:blink_web_tests/(virtual/[^/]+/)?(?P<testname>([^/]+/)+[^/]+\.[a-zA-Z]+).*$`,
			LikeTemplate: "ninja://:blink\\_web\\_tests/%${testname}%",
		},
	}
	cfgpb := &configpb.ProjectConfig{
		Clustering: &configpb.Clustering{
			TestNameRules: rules,
		},
	}

	ftt.Run(`Name`, t, func(t *ftt.Test) {
		// Algorithm name should be valid.
		a := &Algorithm{}
		assert.Loosely(t, clustering.AlgorithmRe.MatchString(a.Name()), should.BeTrue)
	})
	ftt.Run(`Cluster`, t, func(t *ftt.Test) {
		a := &Algorithm{}
		cfg, err := compiledcfg.NewConfig(cfgpb)
		assert.Loosely(t, err, should.BeNil)

		t.Run(`ID of appropriate length`, func(t *ftt.Test) {
			id := a.Cluster(cfg, &clustering.Failure{
				TestID: "ninja://test_name",
			})
			// IDs may be 16 bytes at most.
			assert.Loosely(t, len(id), should.BeGreaterThan(0))
			assert.Loosely(t, len(id), should.BeLessThanOrEqual(clustering.MaxClusterIDBytes))
		})
		t.Run(`Same ID for same test name`, func(t *ftt.Test) {
			t.Run(`No matching rules`, func(t *ftt.Test) {
				id1 := a.Cluster(cfg, &clustering.Failure{
					TestID: "ninja://test_name_one/",
					Reason: &pb.FailureReason{PrimaryErrorMessage: "A"},
				})
				id2 := a.Cluster(cfg, &clustering.Failure{
					TestID: "ninja://test_name_one/",
					Reason: &pb.FailureReason{PrimaryErrorMessage: "B"},
				})
				assert.Loosely(t, id2, should.Match(id1))
			})
			t.Run(`Matching rules`, func(t *ftt.Test) {
				id1 := a.Cluster(cfg, &clustering.Failure{
					TestID: "ninja://:blink_web_tests/virtual/abc/folder/test-name.html",
					Reason: &pb.FailureReason{PrimaryErrorMessage: "A"},
				})
				id2 := a.Cluster(cfg, &clustering.Failure{
					TestID: "ninja://:blink_web_tests/folder/test-name.html?param=2",
					Reason: &pb.FailureReason{PrimaryErrorMessage: "B"},
				})
				assert.Loosely(t, id2, should.Match(id1))
			})
		})
		t.Run(`Different ID for different clusters`, func(t *ftt.Test) {
			t.Run(`No matching rules`, func(t *ftt.Test) {
				id1 := a.Cluster(cfg, &clustering.Failure{
					TestID: "ninja://test_name_one/",
				})
				id2 := a.Cluster(cfg, &clustering.Failure{
					TestID: "ninja://test_name_two/",
				})
				assert.Loosely(t, id2, should.NotResemble(id1))
			})
			t.Run(`Matching rules`, func(t *ftt.Test) {
				id1 := a.Cluster(cfg, &clustering.Failure{
					TestID: "ninja://:blink_web_tests/virtual/abc/folder/test-name-a.html",
					Reason: &pb.FailureReason{PrimaryErrorMessage: "A"},
				})
				id2 := a.Cluster(cfg, &clustering.Failure{
					TestID: "ninja://:blink_web_tests/folder/test-name-b.html?param=2",
					Reason: &pb.FailureReason{PrimaryErrorMessage: "B"},
				})
				assert.Loosely(t, id2, should.NotResemble(id1))
			})
		})
	})
	ftt.Run(`Failure Association Rule`, t, func(t *ftt.Test) {
		a := &Algorithm{}
		cfg, err := compiledcfg.NewConfig(cfgpb)
		assert.Loosely(t, err, should.BeNil)

		test := func(failure *clustering.Failure, expectedRule string) {
			rule := a.FailureAssociationRule(cfg, failure)
			assert.Loosely(t, rule, should.Equal(expectedRule))

			// Test the rule is valid syntax and matches at least the example failure.
			expr, err := lang.Parse(rule)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, expr.Evaluate(lang.Failure{Test: failure.TestID}), should.BeTrue)
		}
		t.Run(`No matching rules`, func(t *ftt.Test) {
			failure := &clustering.Failure{
				TestID: "ninja://test_name_one/",
			}
			test(failure, `test = "ninja://test_name_one/"`)
		})
		t.Run(`Matching rule`, func(t *ftt.Test) {
			failure := &clustering.Failure{
				TestID: "ninja://:blink_web_tests/virtual/dark-color-scheme/fast/forms/color-scheme/select/select-multiple-hover-unselected.html",
			}
			test(failure, `test LIKE "ninja://:blink\\_web\\_tests/%fast/forms/color-scheme/select/select-multiple-hover-unselected.html%"`)
		})
		t.Run(`Escapes LIKE syntax`, func(t *ftt.Test) {
			failure := &clustering.Failure{
				TestID: `ninja://:blink_web_tests/a/b_\%c.html`,
			}
			test(failure, `test LIKE "ninja://:blink\\_web\\_tests/%a/b\\_\\\\\\%c.html%"`)
		})
		t.Run(`Escapes non-graphic Unicode characters`, func(t *ftt.Test) {
			failure := &clustering.Failure{
				TestID: "\u0000\r\n\v\u202E\u2066",
			}
			test(failure, `test = "\x00\r\n\v\u202e\u2066"`)
		})
	})
	ftt.Run(`Cluster Title`, t, func(t *ftt.Test) {
		a := &Algorithm{}
		cfg, err := compiledcfg.NewConfig(cfgpb)
		assert.Loosely(t, err, should.BeNil)

		t.Run(`No matching rules`, func(t *ftt.Test) {
			failure := &clustering.Failure{
				TestID: "ninja://test_name_one",
			}
			title := a.ClusterTitle(cfg, failure)
			assert.Loosely(t, title, should.Equal("ninja://test_name_one"))
		})
		t.Run(`Matching rule`, func(t *ftt.Test) {
			failure := &clustering.Failure{
				TestID: "ninja://:blink_web_tests/virtual/dark-color-scheme/fast/forms/color-scheme/select/select-multiple-hover-unselected.html",
			}
			title := a.ClusterTitle(cfg, failure)
			assert.Loosely(t, title, should.Equal(`ninja://:blink\\_web\\_tests/%fast/forms/color-scheme/select/select-multiple-hover-unselected.html%`))
		})
	})
	ftt.Run(`Cluster Description`, t, func(t *ftt.Test) {
		a := &Algorithm{}
		cfg, err := compiledcfg.NewConfig(cfgpb)
		assert.Loosely(t, err, should.BeNil)

		t.Run(`No matching rules`, func(t *ftt.Test) {
			summary := &clustering.ClusterSummary{
				Example: clustering.Failure{
					TestID: "ninja://test_name_one",
				},
			}
			description, err := a.ClusterDescription(cfg, summary)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, description.Title, should.Equal("ninja://test_name_one"))
			assert.Loosely(t, description.Description, should.ContainSubstring("ninja://test_name_one"))
		})
		t.Run(`Matching rule`, func(t *ftt.Test) {
			summary := &clustering.ClusterSummary{
				Example: clustering.Failure{
					TestID: "ninja://:blink_web_tests/virtual/dark-color-scheme/fast/forms/color-scheme/select/select-multiple-hover-unselected.html",
				},
			}
			description, err := a.ClusterDescription(cfg, summary)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, description.Title, should.Equal(`ninja://:blink\\_web\\_tests/%fast/forms/color-scheme/select/select-multiple-hover-unselected.html%`))
			assert.Loosely(t, description.Description, should.ContainSubstring(`ninja://:blink\\_web\\_tests/%fast/forms/color-scheme/select/select-multiple-hover-unselected.html%`))
		})
	})
}
