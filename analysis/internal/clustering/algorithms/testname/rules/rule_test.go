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

package rules

import (
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	configpb "go.chromium.org/luci/analysis/proto/config"
)

func TestRule(t *testing.T) {
	ftt.Run(`Evaluate`, t, func(t *ftt.Test) {
		t.Run(`Valid Examples`, func(t *ftt.Test) {
			t.Run(`Blink Web Tests`, func(t *ftt.Test) {
				rule := &configpb.TestNameClusteringRule{
					Name:         "Blink Web Tests",
					Pattern:      `^ninja://:blink_web_tests/(virtual/[^/]+/)?(?P<testname>([^/]+/)+[^/]+\.[a-zA-Z]+).*$`,
					LikeTemplate: `ninja://:blink\_web\_tests/%${testname}%`,
				}
				eval, err := Compile(rule)
				assert.Loosely(t, err, should.BeNil)

				inputs := []string{
					"ninja://:blink_web_tests/virtual/oopr-canvas2d/fast/canvas/canvas-getImageData.html",
					"ninja://:blink_web_tests/virtual/oopr-canvas2d/fast/canvas/canvas-getImageData.html?param=a",
					"ninja://:blink_web_tests/virtual/oopr-canvas3d/fast/canvas/canvas-getImageData.html?param=b",
					"ninja://:blink_web_tests/fast/canvas/canvas-getImageData.html",
				}
				for _, testname := range inputs {
					like, ok := eval(testname)
					assert.Loosely(t, ok, should.BeTrue)
					assert.Loosely(t, like, should.Equal(`ninja://:blink\_web\_tests/%fast/canvas/canvas-getImageData.html%`))
				}

				_, ok := eval("ninja://:not_blink_web_tests/fast/canvas/canvas-getImageData.html")
				assert.Loosely(t, ok, should.BeFalse)
			})
			t.Run(`Google Tests`, func(t *ftt.Test) {
				rule := &configpb.TestNameClusteringRule{
					Name: "Google Test (Value-parameterized)",
					// E.g. ninja:{target}/Prefix/ColorSpaceTest.testNullTransform/11
					// Note that "Prefix/" portion may be blank/omitted.
					Pattern:      `^ninja:(?P<target>[\w/]+:\w+)/(\w+/)?(?P<suite>\w+)\.(?P<case>\w+)/\w+$`,
					LikeTemplate: `ninja:${target}/%${suite}.${case}%`,
				}
				eval, err := Compile(rule)
				assert.Loosely(t, err, should.BeNil)

				inputs := []string{
					"ninja://chrome/test:interactive_ui_tests/Name/ColorSpaceTest.testNullTransform/0",
					"ninja://chrome/test:interactive_ui_tests/Name/ColorSpaceTest.testNullTransform/0",
					"ninja://chrome/test:interactive_ui_tests/Name/ColorSpaceTest.testNullTransform/11",
				}
				for _, testname := range inputs {
					like, ok := eval(testname)
					assert.Loosely(t, ok, should.BeTrue)
					assert.Loosely(t, like, should.Equal("ninja://chrome/test:interactive\\_ui\\_tests/%ColorSpaceTest.testNullTransform%"))
				}

				_, ok := eval("ninja://:blink_web_tests/virtual/oopr-canvas2d/fast/canvas/canvas-getImageData.html")
				assert.Loosely(t, ok, should.BeFalse)
			})
		})
		t.Run(`Test name escaping in LIKE output`, func(t *ftt.Test) {
			t.Run(`Test name is escaped when substituted`, func(t *ftt.Test) {
				rule := &configpb.TestNameClusteringRule{
					Name:         "Escape test",
					Pattern:      `^(?P<testname>.*)$`,
					LikeTemplate: `${testname}_%`,
				}
				eval, err := Compile(rule)
				assert.Loosely(t, err, should.BeNil)

				// Verify that the test name is not injected varbatim in the generated
				// like expression, but is escaped to avoid it being interpreted.
				like, ok := eval(`_\%`)
				assert.Loosely(t, ok, should.BeTrue)
				assert.Loosely(t, like, should.Equal(`\_\\\%_%`))
			})
			t.Run(`Unsafe LIKE templates are rejected`, func(t *ftt.Test) {
				rule := &configpb.TestNameClusteringRule{
					Name:    "Escape test",
					Pattern: `^path\\(?P<testname>.*)$`,
					// The user as incorrectly used an unfinished LIKE escape sequence
					// (with trailing '\') before the testname substitution.
					// If substitution were allowed, this may allow the test name to be
					// interpreted as a LIKE expression instead as literal text.
					// E.g. a test name of `path\%` may yield `path\\%` after template
					// evaluation which invokes the LIKE '%' operator.
					LikeTemplate: `path\${testname}`,
				}
				_, err := Compile(rule)
				assert.Loosely(t, err, should.ErrLike(`"path\\" is not a valid standalone LIKE expression: unfinished escape sequence "\" at end of LIKE pattern`))
			})
		})
		t.Run(`Substitution operator`, func(t *ftt.Test) {
			t.Run(`Dollar sign can be inserted into output`, func(t *ftt.Test) {
				rule := &configpb.TestNameClusteringRule{
					Name:         "Insert $",
					Pattern:      `^(?P<testname>.*)$`,
					LikeTemplate: `${testname}$$blah$$$$`,
				}
				eval, err := Compile(rule)
				assert.Loosely(t, err, should.BeNil)

				like, ok := eval(`test`)
				assert.Loosely(t, ok, should.BeTrue)
				assert.Loosely(t, like, should.Equal(`test$blah$$`))
			})
			t.Run(`Invalid uses of substitution operator are rejected`, func(t *ftt.Test) {
				rule := &configpb.TestNameClusteringRule{
					Name:         "Invalid use of $ (neither $$ or ${name})",
					Pattern:      `^(?P<testname>.*)$`,
					LikeTemplate: `${testname}blah$$$`,
				}
				_, err := Compile(rule)
				assert.Loosely(t, err, should.ErrLike(`invalid use of the $ operator at position 17 in "${testname}blah$$$"`))

				rule = &configpb.TestNameClusteringRule{
					Name:         "Invalid use of $ (invalid capture group name)",
					Pattern:      `^(?P<testname>.*)$`,
					LikeTemplate: `${template@}blah`,
				}
				_, err = Compile(rule)
				assert.Loosely(t, err, should.ErrLike(`invalid use of the $ operator at position 0 in "${template@}blah"`))

				rule = &configpb.TestNameClusteringRule{
					Name:         "Capture group name not defined",
					Pattern:      `^(?P<testname>.*)$`,
					LikeTemplate: `${myname}blah`,
				}
				_, err = Compile(rule)
				assert.Loosely(t, err, should.ErrLike(`like_template: contains reference to non-existant capturing group with name "myname"`))
			})
		})
		t.Run(`Invalid Pattern`, func(t *ftt.Test) {
			rule := &configpb.TestNameClusteringRule{
				Name:         "Invalid Pattern",
				Pattern:      `[`,
				LikeTemplate: ``,
			}
			_, err := Compile(rule)
			assert.Loosely(t, err, should.ErrLike(`pattern: error parsing regexp`))
		})
	})
}
