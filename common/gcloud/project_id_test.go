// Copyright 2021 The LUCI Authors.
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

package gcloud

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestValidateProjectID(t *testing.T) {
	t.Parallel()

	Convey(`Test ValidateProjectID`, t, func() {
		Convey(`With valid project IDs`, func() {
			So(ValidateProjectID(`project-id-foo`), ShouldBeNil)
		})

		Convey(`W/o a starting, lowercase ASCII letters`, func() {
			e := `must start with a lowercase ASCII letter`
			So(ValidateProjectID(`0123456`), ShouldErrLike, e)
			So(ValidateProjectID(`ProjectID`), ShouldErrLike, e)
			So(ValidateProjectID(`-project-id`), ShouldErrLike, e)
			So(ValidateProjectID(`ö-project-id`), ShouldErrLike, e)
		})

		Convey(`With a trailing hyphen`, func() {
			e := `must not have a trailing hyphen`
			So(ValidateProjectID(`project-id-`), ShouldErrLike, e)
			So(ValidateProjectID(`project--id--`), ShouldErrLike, e)
		})

		Convey(`With len() < 6 or > 30`, func() {
			e := `must contain 6 to 30 ASCII letters, digits, or hyphens`
			So(ValidateProjectID(``), ShouldErrLike, e)
			So(ValidateProjectID(`pro`), ShouldErrLike, e)
			So(ValidateProjectID(`project-id-1234567890-1234567890`), ShouldErrLike, e)
		})

		Convey(`With non-ascii letters`, func() {
			e := `invalid letter`
			So(ValidateProjectID(`project-✅-id`), ShouldErrLike, e)
			So(ValidateProjectID(`project-✅-👀`), ShouldErrLike, e)
			So(ValidateProjectID(`project-id-🍀🍀🍀🍀s`), ShouldErrLike, e)
		})
	})
}
