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

package model

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestChangeLogs(t *testing.T) {
	Convey("GetReviewUrl", t, func() {
		cl := &ChangeLog{
			Message: "",
		}
		_, err := cl.GetReviewUrl()
		So(err, ShouldNotBeNil)
		cl = &ChangeLog{
			Message: "Use TestActivationManager for all page activations\n\nblah blah\n\nChange-Id: blah\nBug: blah\nReviewed-on: https://chromium-review.googlesource.com/c/chromium/src/+/3472129\nReviewed-by: blah blah\n",
		}
		reviewUrl, err := cl.GetReviewUrl()
		So(err, ShouldBeNil)
		So(reviewUrl, ShouldEqual, "https://chromium-review.googlesource.com/c/chromium/src/+/3472129")
	})

	Convey("GetReviewTitle", t, func() {
		cl := &ChangeLog{
			Message: "",
		}
		reviewTitle, err := cl.GetReviewTitle()
		So(err, ShouldNotBeNil)
		So(reviewTitle, ShouldEqual, "")

		cl = &ChangeLog{
			Message: "Use TestActivationManager for all page activations\n\nblah blah\n\nChange-Id: blah\nBug: blah\nReviewed-on: https://chromium-review.googlesource.com/c/chromium/src/+/3472129\nReviewed-by: blah blah\n",
		}
		reviewTitle, err = cl.GetReviewTitle()
		So(err, ShouldBeNil)
		So(reviewTitle, ShouldEqual, "Use TestActivationManager for all page activations")
	})
}
