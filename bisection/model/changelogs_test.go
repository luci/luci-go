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
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestChangeLogs(t *testing.T) {
	ftt.Run("GetReviewUrl", t, func(t *ftt.Test) {
		cl := &ChangeLog{
			Message: "",
		}
		_, err := cl.GetReviewUrl()
		assert.Loosely(t, err, should.NotBeNil)
		cl = &ChangeLog{
			Message: "Use TestActivationManager for all page activations\n\nblah blah\n\nChange-Id: blah\nBug: blah\nReviewed-on: https://chromium-review.googlesource.com/c/chromium/src/+/3472129\nReviewed-by: blah blah\n",
		}
		reviewUrl, err := cl.GetReviewUrl()
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, reviewUrl, should.Equal("https://chromium-review.googlesource.com/c/chromium/src/+/3472129"))
	})

	ftt.Run("GetReviewTitle", t, func(t *ftt.Test) {
		cl := &ChangeLog{
			Message: "",
		}
		reviewTitle, err := cl.GetReviewTitle()
		assert.Loosely(t, err, should.NotBeNil)
		assert.Loosely(t, reviewTitle, should.BeEmpty)

		cl = &ChangeLog{
			Message: "Use TestActivationManager for all page activations\n\nblah blah\n\nChange-Id: blah\nBug: blah\nReviewed-on: https://chromium-review.googlesource.com/c/chromium/src/+/3472129\nReviewed-by: blah blah\n",
		}
		reviewTitle, err = cl.GetReviewTitle()
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, reviewTitle, should.Equal("Use TestActivationManager for all page activations"))
	})

	ftt.Run("Get commit time", t, func(t *ftt.Test) {
		cl := &ChangeLog{
			Message: "",
		}
		_, err := cl.GetCommitTime()
		assert.Loosely(t, err, should.NotBeNil)

		cl = &ChangeLog{
			Committer: ChangeLogActor{
				Time: "Tue Oct 17 07:06:57 2023",
			},
		}
		commitTime, err := cl.GetCommitTime()
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, commitTime, should.Match(timestamppb.New(time.Date(2023, time.October, 17, 7, 6, 57, 0, time.UTC))))
	})
}
