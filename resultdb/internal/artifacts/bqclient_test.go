// Copyright 2024 The LUCI Authors.
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

package artifacts

import (
	"regexp"
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	resultpb "go.chromium.org/luci/resultdb/proto/v1"
)

func TestMatchWithContextRegexBuilder(t *testing.T) {
	t.Parallel()

	ftt.Run(`MatchWithContextRegexBuilder`, t, func(t *ftt.Test) {
		// Matching part in the middle of a line.
		text1 := "line1\r\nline2\r\nline3exact.contain()line3after\r\nline4"
		// Matching part at the end of a line.
		text2 := "line1exact.contain()\r\nline2\r\nline3\r\nline4"
		// Matching part at the end of the text.
		text3 := "line1\r\nline2\r\nline3\r\nline4exact.contain()"
		verifyRegex := func(text string, regex string, expected string) {
			r, err := regexp.Compile(regex)
			assert.Loosely(t, err, should.BeNil)
			matches := r.FindStringSubmatch(text)
			assert.Loosely(t, matches, should.HaveLength(2))
			assert.Loosely(t, matches[1], should.Equal(expected))
		}
		t.Run(`exact contain mode`, func(t *ftt.Test) {
			builder := newMatchWithContextRegexBuilder(&resultpb.ArtifactContentMatcher{
				Matcher: &resultpb.ArtifactContentMatcher_Contain{
					Contain: "exact.contain()",
				},
			})

			t.Run(`capture match`, func(t *ftt.Test) {
				resp := builder.withCaptureMatch(true).build()
				assert.Loosely(t, resp, should.Equal("(?:\\r\\n|\\r|\\n)?[^\\r\\n]*?(?:\\r\\n|\\r|\\n)?[^\\r\\n]*?((?i)exact\\.contain\\(\\))[^\\r\\n]*(?:\\r\\n|\\r|\\n)?[^\\r\\n]*(?:\\r\\n|\\r|\\n)?"))
				verifyRegex(text1, resp, "exact.contain()")
				verifyRegex(text2, resp, "exact.contain()")
				verifyRegex(text3, resp, "exact.contain()")
			})

			t.Run(`capture before`, func(t *ftt.Test) {
				resp := builder.withCaptureContextBefore(true).build()
				assert.Loosely(t, resp, should.Equal("(?:\\r\\n|\\r|\\n)?([^\\r\\n]*?(?:\\r\\n|\\r|\\n)?[^\\r\\n]*?)(?i)exact\\.contain\\(\\)[^\\r\\n]*(?:\\r\\n|\\r|\\n)?[^\\r\\n]*(?:\\r\\n|\\r|\\n)?"))
				verifyRegex(text1, resp, "line2\r\nline3")
				verifyRegex(text2, resp, "line1")
				verifyRegex(text3, resp, "line3\r\nline4")
			})

			t.Run(`capture after`, func(t *ftt.Test) {
				resp := builder.withCaptureContextAfter(true).build()
				assert.Loosely(t, resp, should.Equal("(?:\\r\\n|\\r|\\n)?[^\\r\\n]*?(?:\\r\\n|\\r|\\n)?[^\\r\\n]*?(?i)exact\\.contain\\(\\)([^\\r\\n]*(?:\\r\\n|\\r|\\n)?[^\\r\\n]*)(?:\\r\\n|\\r|\\n)?"))
				verifyRegex(text1, resp, "line3after\r\nline4")
				verifyRegex(text2, resp, "\r\nline2")
				verifyRegex(text3, resp, "")
			})

			t.Run(`should be case insensitive`, func(t *ftt.Test) {
				builder := newMatchWithContextRegexBuilder(&resultpb.ArtifactContentMatcher{
					Matcher: &resultpb.ArtifactContentMatcher_Contain{
						Contain: "EXACT.contain()",
					},
				})

				resp := builder.withCaptureMatch(true).build()
				assert.Loosely(t, resp, should.Equal("(?:\\r\\n|\\r|\\n)?[^\\r\\n]*?(?:\\r\\n|\\r|\\n)?[^\\r\\n]*?((?i)EXACT\\.contain\\(\\))[^\\r\\n]*(?:\\r\\n|\\r|\\n)?[^\\r\\n]*(?:\\r\\n|\\r|\\n)?"))
				verifyRegex(text1, resp, "exact.contain()")
				verifyRegex(text2, resp, "exact.contain()")
				verifyRegex(text3, resp, "exact.contain()")
			})
		})

		t.Run(`regex contain mode`, func(t *ftt.Test) {
			builder := newMatchWithContextRegexBuilder(&resultpb.ArtifactContentMatcher{
				Matcher: &resultpb.ArtifactContentMatcher_RegexContain{
					RegexContain: `\..*\(\)`,
				},
			})

			t.Run(`capture match`, func(t *ftt.Test) {
				resp := builder.withCaptureMatch(true).build()
				assert.Loosely(t, resp, should.Equal("(?:\\r\\n|\\r|\\n)?[^\\r\\n]*?(?:\\r\\n|\\r|\\n)?[^\\r\\n]*?(\\..*\\(\\))[^\\r\\n]*(?:\\r\\n|\\r|\\n)?[^\\r\\n]*(?:\\r\\n|\\r|\\n)?"))
				verifyRegex(text1, resp, ".contain()")
				verifyRegex(text2, resp, ".contain()")
				verifyRegex(text3, resp, ".contain()")
			})

			t.Run(`capture before`, func(t *ftt.Test) {
				resp := builder.withCaptureContextBefore(true).build()
				assert.Loosely(t, resp, should.Equal("(?:\\r\\n|\\r|\\n)?([^\\r\\n]*?(?:\\r\\n|\\r|\\n)?[^\\r\\n]*?)\\..*\\(\\)[^\\r\\n]*(?:\\r\\n|\\r|\\n)?[^\\r\\n]*(?:\\r\\n|\\r|\\n)?"))
				verifyRegex(text1, resp, "line2\r\nline3exact")
				verifyRegex(text2, resp, "line1exact")
				verifyRegex(text3, resp, "line3\r\nline4exact")
			})

			t.Run(`capture after`, func(t *ftt.Test) {
				resp := builder.withCaptureContextAfter(true).build()
				assert.Loosely(t, resp, should.Equal("(?:\\r\\n|\\r|\\n)?[^\\r\\n]*?(?:\\r\\n|\\r|\\n)?[^\\r\\n]*?\\..*\\(\\)([^\\r\\n]*(?:\\r\\n|\\r|\\n)?[^\\r\\n]*)(?:\\r\\n|\\r|\\n)?"))
				verifyRegex(text1, resp, "line3after\r\nline4")
				verifyRegex(text2, resp, "\r\nline2")
				verifyRegex(text3, resp, "")
			})
		})

		t.Run(`should match first occurrence`, func(t *ftt.Test) {
			builder := newMatchWithContextRegexBuilder(&resultpb.ArtifactContentMatcher{
				Matcher: &resultpb.ArtifactContentMatcher_Contain{
					Contain: "l",
				},
			})

			resp := builder.withCaptureMatch(true).withCaptureContextBefore(true).withCaptureContextAfter(true).build()
			r, err := regexp.Compile(resp)
			assert.Loosely(t, err, should.BeNil)
			matches := r.FindStringSubmatch(text1)
			assert.Loosely(t, matches, should.HaveLength(4))
			assert.Loosely(t, matches[1], should.BeEmpty)
			assert.Loosely(t, matches[2], should.Equal("l"))
			assert.Loosely(t, matches[3], should.Equal("ine1\r\nline2"))
		})
	})
}
