// Copyright 2018 The LUCI Authors.
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
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestEmailTemplate(t *testing.T) {
	t.Parallel()

	Convey(`ParsedEmailTemplate.Parse`, t, func() {
		var t ParsedEmailTemplate

		Convey(`valid template`, func() {
			content := `subject

        body`
			So(t.Parse(content), ShouldBeNil)
		})

		Convey(`empty`, func() {
			So(t.Parse(""), ShouldErrLike, "empty")
		})

		Convey(`less than three lines`, func() {
			content := `subject
        body`
			So(t.Parse(content), ShouldErrLike, "less than three lines")
		})

		Convey(`no blank line`, func() {
			content := `subject
        body
        second line
        `
			So(t.Parse(content), ShouldErrLike, "second line is not blank")
		})

		Convey(`invalid subject template`, func() {
			content := `subject{{}}}

        body`
			So(t.Parse(content), ShouldErrLike, "missing value for command")
		})

		Convey(`invalid body template`, func() {
			content := `subject

        body {{!}}`
			So(t.Parse(content), ShouldErrLike, `unexpected "!" in command`)
		})
	})
}
