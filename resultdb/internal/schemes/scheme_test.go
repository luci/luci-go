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

package schemes

import (
	"regexp"
	"testing"

	"github.com/google/go-cmp/cmp"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	"go.chromium.org/luci/resultdb/pbutil"
)

func TestSchemes(t *testing.T) {
	t.Parallel()
	ftt.Run(`Schemes`, t, func(t *ftt.Test) {
		longScheme := &Scheme{
			ID:                "junit",
			HumanReadableName: "JUnit",
			Coarse: &SchemeLevel{
				ValidationRegexp:  regexp.MustCompile("^[a-z][a-z_0-9.]+$"),
				HumanReadableName: "Package",
			},
			Fine: &SchemeLevel{
				ValidationRegexp:  regexp.MustCompile("^[a-zA-Z_][a-zA-Z_0-9]+$"),
				HumanReadableName: "Class",
			},
			Case: &SchemeLevel{
				ValidationRegexp:  regexp.MustCompile("^[a-zA-Z_][a-zA-Z_0-9]+$"),
				HumanReadableName: "Method",
			},
		}

		basicScheme := &Scheme{
			ID:                "basic",
			HumanReadableName: "Basic",
			Case: &SchemeLevel{
				HumanReadableName: "Method",
			},
		}

		t.Run("Serialisation roundtrips", func(t *ftt.Test) {
			// Roundtripppability is important so that ResultSink can apply
			// scheme validation exactly like the backend does.

			// We need a custom regexp comparer as it has unexported fields.
			regexpComparer := func(x, y *regexp.Regexp) bool {
				if (x == nil) || (y == nil) {
					return x == y
				}
				return x.String() == y.String()
			}

			roundtrip, err := FromProto(longScheme.ToProto())
			assert.NoErr(t, err)
			assert.That(t, roundtrip, should.Match(longScheme, cmp.Comparer(regexpComparer)))

			roundtrip, err = FromProto(basicScheme.ToProto())
			assert.NoErr(t, err)
			assert.That(t, roundtrip, should.Match(basicScheme, cmp.Comparer(regexpComparer)))
		})

		testID := pbutil.BaseTestIdentifier{
			ModuleName:   "myModule",
			ModuleScheme: "junit",
			CoarseName:   "com.example.package",
			FineName:     "ExampleClass",
			CaseName:     "testMethod",
		}

		t.Run("valid", func(t *ftt.Test) {
			assert.Loosely(t, longScheme.Validate(testID), should.BeNil)
		})
		t.Run("Wrong scheme", func(t *ftt.Test) {
			testID.ModuleScheme = "undefined"
			assert.That(t, longScheme.Validate(testID), should.ErrLike("module_scheme: expected test scheme \"junit\" but got scheme \"undefined\""))
		})
		t.Run("Coarse Name", func(t *ftt.Test) {
			t.Run("missing", func(t *ftt.Test) {
				testID.CoarseName = ""
				assert.That(t, longScheme.Validate(testID), should.ErrLike("coarse_name: required, please set a Package (scheme \"junit\")"))
			})
			t.Run("invalid", func(t *ftt.Test) {
				testID.CoarseName = "1com.example.package"
				assert.That(t, longScheme.Validate(testID), should.ErrLike("coarse_name: does not match validation regexp \"^[a-z][a-z_0-9.]+$\", please set a valid Package (scheme \"junit\")"))
			})
			t.Run("set when not expected", func(t *ftt.Test) {
				testID.ModuleScheme = "basic"
				testID.CoarseName = "value"
				assert.That(t, basicScheme.Validate(testID), should.ErrLike("coarse_name: expected empty value (level is not defined by scheme \"basic\")"))
			})
		})
		t.Run("Fine Name", func(t *ftt.Test) {
			t.Run("missing", func(t *ftt.Test) {
				testID.FineName = ""
				assert.That(t, longScheme.Validate(testID), should.ErrLike("fine_name: required, please set a Class (scheme \"junit\")"))
			})
			t.Run("invalid", func(t *ftt.Test) {
				testID.FineName = "1com.example.package"
				assert.That(t, longScheme.Validate(testID), should.ErrLike("fine_name: does not match validation regexp \"^[a-zA-Z_][a-zA-Z_0-9]+$\", please set a valid Class (scheme \"junit\")"))
			})
			t.Run("set when not expected", func(t *ftt.Test) {
				testID.ModuleScheme = "basic"
				testID.CoarseName = ""
				testID.FineName = "value"
				assert.That(t, basicScheme.Validate(testID), should.ErrLike("fine_name: expected empty value (level is not defined by scheme \"basic\")"))
			})
		})
		t.Run("Case Name", func(t *ftt.Test) {
			t.Run("missing", func(t *ftt.Test) {
				testID.CaseName = ""
				assert.That(t, longScheme.Validate(testID), should.ErrLike("case_name: required, please set a Method (scheme \"junit\")"))
			})
			t.Run("invalid", func(t *ftt.Test) {
				testID.CaseName = "1method"
				assert.That(t, longScheme.Validate(testID), should.ErrLike("case_name: does not match validation regexp \"^[a-zA-Z_][a-zA-Z_0-9]+$\", please set a valid Method (scheme \"junit\")"))
			})
		})
	})
}
