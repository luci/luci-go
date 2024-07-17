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

	. "github.com/smartystreets/goconvey/convey"
	resultpb "go.chromium.org/luci/resultdb/proto/v1"
)

func TestMatchWithContextRegexBuilder(t *testing.T) {
	t.Parallel()

	Convey(`MatchWithContextRegexBuilder`, t, func() {
		builder := newMatchWithContextRegexBuilder(&resultpb.ArtifactContentMatcher{
			Matcher: &resultpb.ArtifactContentMatcher_ExactContain{
				ExactContain: "exact.contain()",
			},
		})

		//  Matching part in the middle of a line.
		text1 := "line1\r\nline2\r\nline3exact.contain()line3after\r\nline4"
		// Matching part at the end of a line.
		text2 := "line1exact.contain()\r\nline2\r\nline3\r\nline4"
		// Matching part at the end of the text.
		text3 := "line1\r\nline2\r\nline3\r\nline4exact.contain()"
		verifyRegex := func(text string, regex string, expected string) {
			r, err := regexp.Compile(regex)
			So(err, ShouldBeNil)
			matches := r.FindStringSubmatch(text)
			So(matches, ShouldHaveLength, 2)
			So(matches[1], ShouldEqual, expected)
		}

		Convey(`capture match`, func() {
			resp := builder.withCaptureMatch(true).build()
			So(resp, ShouldEqual, "(?:\\r\\n|\\r|\\n)?[^\r\n]*(?:\\r\\n|\\r|\\n)?[^\r\n]*(exact\\.contain\\(\\))[^\r\n]*(?:\\r\\n|\\r|\\n)?[^\r\n]*(?:\\r\\n|\\r|\\n)?")
			verifyRegex(text1, resp, "exact.contain()")
			verifyRegex(text2, resp, "exact.contain()")
			verifyRegex(text3, resp, "exact.contain()")
		})

		Convey(`capture before`, func() {
			resp := builder.withCaptureContextBefore(true).build()
			So(resp, ShouldEqual, "(?:\\r\\n|\\r|\\n)?([^\r\n]*(?:\\r\\n|\\r|\\n)?[^\r\n]*)exact\\.contain\\(\\)[^\r\n]*(?:\\r\\n|\\r|\\n)?[^\r\n]*(?:\\r\\n|\\r|\\n)?")
			verifyRegex(text1, resp, "line2\r\nline3")
			verifyRegex(text2, resp, "line1")
			verifyRegex(text3, resp, "line3\r\nline4")
		})

		Convey(`capture after`, func() {
			resp := builder.withCaptureContextAfter(true).build()
			So(resp, ShouldEqual, "(?:\\r\\n|\\r|\\n)?[^\r\n]*(?:\\r\\n|\\r|\\n)?[^\r\n]*exact\\.contain\\(\\)([^\r\n]*(?:\\r\\n|\\r|\\n)?[^\r\n]*)(?:\\r\\n|\\r|\\n)?")
			verifyRegex(text1, resp, "line3after\r\nline4")
			verifyRegex(text2, resp, "\r\nline2")
			verifyRegex(text3, resp, "")
		})
	})
}
