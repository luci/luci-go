// Copyright 2025 The LUCI Authors.
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

package config

import (
	"context"
	"fmt"
	"regexp"
	"testing"
	"time"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/config"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/server/caching"

	"go.chromium.org/luci/resultdb/pbutil"
	configpb "go.chromium.org/luci/resultdb/proto/config"
)

func TestCompiledConfig(t *testing.T) {
	ftt.Run(`With In-Process Cache`, t, func(t *ftt.Test) {
		ctx := context.Background()
		ctx = memory.Use(ctx)
		ctx, tc := testclock.UseTime(ctx, testclock.TestRecentTimeUTC)
		ctx = caching.WithEmptyProcessCache(ctx)

		cfg := generateServiceConfig(0)
		err := SetServiceConfigWithMetaForTesting(ctx, cfg, &config.Meta{Revision: "revision_0"})
		assert.Loosely(t, err, should.BeNil)

		// Check the first config.
		got, err := Service(ctx)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, got.Config, should.Match(generateServiceConfig(0)))
		assert.Loosely(t, got.Revision, should.Equal("revision_0"))
		assert.Loosely(t, len(got.Schemes), should.Equal(3))
		assert.Loosely(t, got.Schemes["junit0"], should.NotBeNil)
		assert.Loosely(t, got.Schemes["gtest"], should.NotBeNil)
		assert.Loosely(t, got.Schemes["legacy"], should.NotBeNil)
		gtestScheme := got.Schemes["gtest"]

		// Assert the GTest scheme is compiled as expected.
		assert.Loosely(t, gtestScheme.ID, should.Equal("gtest"))
		assert.Loosely(t, gtestScheme.HumanReadableName, should.Equal("GTest"))
		assert.Loosely(t, gtestScheme.Coarse, should.BeNil)
		assert.Loosely(t, gtestScheme.Fine.HumanReadableName, should.Equal("Suite"))
		assert.Loosely(t, gtestScheme.Fine.ValidationRegexp.String(), should.Equal(`^[^_]+$`))
		assert.Loosely(t, gtestScheme.Case.HumanReadableName, should.Equal("Method"))
		assert.Loosely(t, gtestScheme.Case.ValidationRegexp, should.BeNil)

		t.Run(`Repeated query works`, func(t *ftt.Test) {
			// Check we get exactly the same cached object as should have gotten a cache hit.
			gotAgain, err := Service(ctx)
			assert.Loosely(t, err, should.BeNil)
			assert.That(t, gotAgain, should.Equal(got))
		})
		t.Run(`If underlying config is refreshed, compiled config is updated`, func(t *ftt.Test) {
			cfg := generateServiceConfig(1)
			err := SetServiceConfigWithMetaForTesting(ctx, cfg, &config.Meta{Revision: "revision_1"})
			assert.Loosely(t, err, should.BeNil)

			// Initially, config should not be updated as it is still cached.
			got, err := Service(ctx)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, got.Revision, should.Equal("revision_0"))
			assert.Loosely(t, got.Config, should.Match(generateServiceConfig(0)))
			assert.Loosely(t, got.Schemes["junit0"], should.NotBeNil)

			tc.Add(2 * time.Minute)

			// After the cache expires, the config should be updated.
			got, err = Service(ctx)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, got.Revision, should.Equal("revision_1"))
			assert.Loosely(t, got.Config, should.Match(generateServiceConfig(1)))
			assert.Loosely(t, len(got.Schemes), should.Equal(3))
			assert.Loosely(t, got.Schemes["junit1"], should.NotBeNil)
		})
	})
}

func generateServiceConfig(uniqifier int) *configpb.Config {
	cfg := &configpb.Config{
		Schemes: []*configpb.Scheme{
			{
				Id:                "gtest",
				HumanReadableName: "GTest",
				Fine: &configpb.Scheme_Level{
					HumanReadableName: "Suite",
					// Do not allow underscores as per https://google.github.io/googletest/reference/testing.html.
					ValidationRegexp: `^[^_]+$`,
				},
				Case: &configpb.Scheme_Level{
					HumanReadableName: "Method",
				},
			},
			{
				Id:                fmt.Sprintf("junit%v", uniqifier),
				HumanReadableName: "JUnit",
				Coarse: &configpb.Scheme_Level{
					HumanReadableName: "Package",
				},
				Fine: &configpb.Scheme_Level{
					HumanReadableName: "Class",
				},
				Case: &configpb.Scheme_Level{
					HumanReadableName: "Method",
				},
			},
		},
	}
	return cfg
}

func TestTestIDLimits(t *testing.T) {
	ftt.Run(`TestIDLimits`, t, func(t *ftt.Test) {
		c := &CompiledServiceConfig{
			TestIDsWithHigherLimit: []CompiledTestIdEntry{
				{
					ModuleName: "exact_module",
					CoarseName: "exact_coarse",
					FineName:   "exact_fine",
				},
				{
					ModuleNamePattern: regexp.MustCompile(`^pattern_.*$`),
					CoarseName:        "pattern_coarse",
				},
				{
					ModuleName: "any_coarse_fine",
				},
				{
					CoarseName: "any_module_fine",
				},
				{
					FineName: "any_module_coarse",
				},
			},
		}

		callback := c.TestIDLimits

		t.Run(`Exact match`, func(t *ftt.Test) {
			id := pbutil.BaseTestIdentifier{
				ModuleName: "exact_module",
				CoarseName: "exact_coarse",
				FineName:   "exact_fine",
			}
			assert.Loosely(t, callback(id), should.Match(pbutil.HigherTestIDValidationLimits))
		})

		t.Run(`Pattern match`, func(t *ftt.Test) {
			id := pbutil.BaseTestIdentifier{
				ModuleName: "pattern_module",
				CoarseName: "pattern_coarse",
			}
			assert.Loosely(t, callback(id), should.Match(pbutil.HigherTestIDValidationLimits))
		})

		t.Run(`Any coarse and fine match`, func(t *ftt.Test) {
			id := pbutil.BaseTestIdentifier{
				ModuleName: "any_coarse_fine",
				CoarseName: "something",
				FineName:   "else",
			}
			assert.Loosely(t, callback(id), should.Match(pbutil.HigherTestIDValidationLimits))
		})

		t.Run(`Any module and fine match`, func(t *ftt.Test) {
			id := pbutil.BaseTestIdentifier{
				ModuleName: "something",
				CoarseName: "any_module_fine",
				FineName:   "else",
			}
			assert.Loosely(t, callback(id), should.Match(pbutil.HigherTestIDValidationLimits))
		})

		t.Run(`Any module and coarse match`, func(t *ftt.Test) {
			id := pbutil.BaseTestIdentifier{
				ModuleName: "something",
				CoarseName: "else",
				FineName:   "any_module_coarse",
			}
			assert.Loosely(t, callback(id), should.Match(pbutil.HigherTestIDValidationLimits))
		})

		t.Run(`Default limits when no match`, func(t *ftt.Test) {
			id := pbutil.BaseTestIdentifier{
				ModuleName: "nomatch",
				CoarseName: "nomatch",
				FineName:   "nomatch",
			}
			assert.Loosely(t, callback(id), should.Match(pbutil.DefaultTestIDValidationLimits))
		})

		t.Run(`Empty string in ID does not match non-empty in config`, func(t *ftt.Test) {
			id := pbutil.BaseTestIdentifier{
				ModuleName: "",
				CoarseName: "exact_coarse",
				FineName:   "exact_fine",
			}
			// Should not match the first entry because ModuleName is empty in ID but "exact_module" in config.
			assert.Loosely(t, callback(id), should.Match(pbutil.DefaultTestIDValidationLimits))
		})
	})
}
