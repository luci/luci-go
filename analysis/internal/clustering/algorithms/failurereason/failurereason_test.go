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

package failurereason

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
	cfgpb := &configpb.ProjectConfig{
		Clustering: &configpb.Clustering{
			ReasonMaskPatterns: []string{
				`(?:^\[Fixture failure\] )([a-zA-Z0-9_]+)[:]`,
			},
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

		t.Run(`Does not cluster test result without failure reason`, func(t *ftt.Test) {
			id := a.Cluster(cfg, &clustering.Failure{})
			assert.Loosely(t, id, should.BeNil)
		})
		t.Run(`ID of appropriate length`, func(t *ftt.Test) {
			id := a.Cluster(cfg, &clustering.Failure{
				Reason: &pb.FailureReason{PrimaryErrorMessage: "abcd this is a test failure message"},
			})
			// IDs may be 16 bytes at most.
			assert.Loosely(t, len(id), should.BeGreaterThan(0))
			assert.Loosely(t, len(id), should.BeLessThanOrEqual(clustering.MaxClusterIDBytes))
		})
		t.Run(`Same ID for same cluster with different numbers`, func(t *ftt.Test) {
			id1 := a.Cluster(cfg, &clustering.Failure{
				Reason: &pb.FailureReason{PrimaryErrorMessage: "Null pointer exception at ip 0x45637271"},
			})
			id2 := a.Cluster(cfg, &clustering.Failure{
				Reason: &pb.FailureReason{PrimaryErrorMessage: "Null pointer exception at ip 0x12345678"},
			})
			assert.Loosely(t, id2, should.Match(id1))
		})
		t.Run(`Different ID for different clusters`, func(t *ftt.Test) {
			id1 := a.Cluster(cfg, &clustering.Failure{
				Reason: &pb.FailureReason{PrimaryErrorMessage: "Exception in TestMethod"},
			})
			id2 := a.Cluster(cfg, &clustering.Failure{
				Reason: &pb.FailureReason{PrimaryErrorMessage: "Exception in MethodUnderTest"},
			})
			assert.Loosely(t, id2, should.NotResemble(id1))
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
			assert.Loosely(t, expr.Evaluate(failure), should.BeTrue)
		}
		t.Run(`Hexadecimal`, func(t *ftt.Test) {
			failure := &clustering.Failure{
				Reason: &pb.FailureReason{PrimaryErrorMessage: "Null pointer exception at ip 0x45637271"},
			}
			test(failure, `reason LIKE "Null pointer exception at ip %"`)
		})
		t.Run(`Numeric`, func(t *ftt.Test) {
			failure := &clustering.Failure{
				Reason: &pb.FailureReason{PrimaryErrorMessage: "Could not connect to 127.1.2.1: connection refused"},
			}
			test(failure, `reason LIKE "Could not connect to %.%.%.%: connection refused"`)
		})
		t.Run(`Base64`, func(t *ftt.Test) {
			failure := &clustering.Failure{
				Reason: &pb.FailureReason{PrimaryErrorMessage: "Received unexpected response: AdafdxAAD17917+/="},
			}
			test(failure, `reason LIKE "Received unexpected response: %"`)
		})
		t.Run(`Escaping`, func(t *ftt.Test) {
			failure := &clustering.Failure{
				Reason: &pb.FailureReason{PrimaryErrorMessage: `_%"'+[]|` + "\u0000\r\n\v\u202E\u2066 AdafdxAAD17917+/="},
			}
			test(failure, `reason LIKE "\\_\\%\"'+[]|\x00\r\n\v\u202e\u2066 %"`)
		})
		t.Run(`Escaping of unicode replacement character / error rune (U+FFFD)`, func(t *ftt.Test) {
			reason := "a\ufffdb\ufffd"

			failure := &clustering.Failure{
				Reason: &pb.FailureReason{PrimaryErrorMessage: string(reason)},
			}
			test(failure, `reason LIKE "a\ufffdb\ufffd"`)
		})
		t.Run(`Multiline`, func(t *ftt.Test) {
			failure := &clustering.Failure{
				Reason: &pb.FailureReason{
					// Previously "ce\n ... Ac" matched the hexadecimal format
					// for hexadecimal strings of 16 characters or more.
					PrimaryErrorMessage: "Expected: to be called once\n          Actual: never called",
				},
			}
			test(failure, `reason LIKE "Expected: to be called once\n          Actual: never called"`)
		})
		t.Run(`User-defined masking`, func(t *ftt.Test) {
			failure := &clustering.Failure{
				Reason: &pb.FailureReason{PrimaryErrorMessage: `[Fixture failure] crostiniBuster: Failed to install Crostini: 3 is not 5`},
			}
			test(failure, `reason LIKE "[Fixture failure] %: Failed to install Crostini: % is not %"`)
		})
	})
	ftt.Run(`Cluster Title`, t, func(t *ftt.Test) {
		a := &Algorithm{}
		cfg, err := compiledcfg.NewConfig(cfgpb)
		assert.Loosely(t, err, should.BeNil)

		t.Run(`Baseline`, func(t *ftt.Test) {
			failure := &clustering.Failure{
				Reason: &pb.FailureReason{PrimaryErrorMessage: "Null pointer exception at ip 0x45637271"},
			}
			title := a.ClusterTitle(cfg, failure)
			assert.Loosely(t, title, should.Equal(`Null pointer exception at ip %`))
		})
		t.Run(`Escaping`, func(t *ftt.Test) {
			failure := &clustering.Failure{
				Reason: &pb.FailureReason{PrimaryErrorMessage: `_%"'+[]|` + "\u0000\r\n\v\u202E\u2066 AdafdxAAD17917+/="},
			}
			title := a.ClusterTitle(cfg, failure)
			assert.Loosely(t, title, should.Equal(`_%\"'+[]|\x00\r\n\v\u202e\u2066 %`))
		})
		t.Run(`User-defined masking`, func(t *ftt.Test) {
			failure := &clustering.Failure{
				Reason: &pb.FailureReason{PrimaryErrorMessage: `[Fixture failure] crostiniBuster: Failed to install Crostini: 3 is not 5`},
			}
			title := a.ClusterTitle(cfg, failure)
			assert.Loosely(t, title, should.Equal(`[Fixture failure] %: Failed to install Crostini: % is not %`))
		})
	})
	ftt.Run(`Cluster Description`, t, func(t *ftt.Test) {
		a := &Algorithm{}
		cfg, err := compiledcfg.NewConfig(cfgpb)
		assert.Loosely(t, err, should.BeNil)

		t.Run(`Baseline`, func(t *ftt.Test) {
			failure := &clustering.ClusterSummary{
				Example: clustering.Failure{
					Reason: &pb.FailureReason{PrimaryErrorMessage: "Null pointer exception at ip 0x45637271"},
				},
				TopTests: []string{
					"ninja://test_one",
					"ninja://test_two",
					"ninja://test_three",
				},
			}
			description, err := a.ClusterDescription(cfg, failure)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, description.Title, should.Equal(`Null pointer exception at ip 0x45637271`))
			assert.Loosely(t, description.Description, should.ContainSubstring(`Null pointer exception at ip 0x45637271`))
			assert.Loosely(t, description.Description, should.ContainSubstring(`- ninja://test_one`))
			assert.Loosely(t, description.Description, should.ContainSubstring(`- ninja://test_three`))
			assert.Loosely(t, description.Description, should.ContainSubstring(`- ninja://test_three`))
		})
		t.Run(`Escaping`, func(t *ftt.Test) {
			summary := &clustering.ClusterSummary{
				Example: clustering.Failure{
					Reason: &pb.FailureReason{PrimaryErrorMessage: `_%"'+[]|` + "\u0000\r\n\v\u202E\u2066 AdafdxAAD17917+/="},
				},
				TopTests: []string{
					"\u2066\u202E\v\n\r\u0000",
				},
			}
			description, err := a.ClusterDescription(cfg, summary)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, description.Title, should.Equal(`_%\"'+[]|\x00\r\n\v\u202e\u2066 AdafdxAAD17917+/=`))
			assert.Loosely(t, description.Description, should.ContainSubstring(`_%\"'+[]|\x00\r\n\v\u202e\u2066 AdafdxAAD17917+/=`))
			assert.Loosely(t, description.Description, should.ContainSubstring(`- \u2066\u202e\v\n\r\x00`))
		})
	})
}
