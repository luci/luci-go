// Copyright 2015 The LUCI Authors.
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

package pubsub

import (
	"fmt"
	"strings"
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestResource(t *testing.T) {
	ftt.Run(`Resource validation`, t, func(t *ftt.Test) {
		for _, tc := range []struct {
			v     string
			c     string
			valid bool
		}{
			{"projects/foo/topics/googResource", "topics", false},
			{"projects/foo/topics/yo", "topics", false},
			{fmt.Sprintf("projects/foo/topics/%s", strings.Repeat("a", 256)), "topics", false},
			{"projects/foo/topics/1ring", "topics", false},
			{"projects/foo/topics/allAboutThe$", "topics", false},
			{"projects/foo/topics/アップ", "topics", false},

			{"projects/foo/topics/AAA", "topics", true},
			{"projects/foo/topics/testResource", "topics", true},
			{fmt.Sprintf("projects/foo/topics/%s", strings.Repeat("a", 255)), "topics", true},
			{"projects/foo/topics/myResource-a_b.c~d+e%%f", "topics", true},
		} {
			if tc.valid {
				t.Run(fmt.Sprintf(`A valid resource [%s] will validate.`, tc.v), func(t *ftt.Test) {
					assert.Loosely(t, validateResource(tc.v, tc.c), should.BeNil)
				})
			} else {
				t.Run(fmt.Sprintf(`An invalid resource [%s] will not validate.`, tc.v), func(t *ftt.Test) {
					assert.Loosely(t, validateResource(tc.v, tc.c), should.NotBeNil)
				})
			}
		}
	})

	ftt.Run(`Can create a new, valid Topic.`, t, func(t *ftt.Test) {
		top := NewTopic("foo", "testTopic")
		assert.Loosely(t, top, should.Equal(Topic("projects/foo/topics/testTopic")))
		assert.Loosely(t, top.Validate(), should.BeNil)
	})

	ftt.Run(`Can create a new, valid Subscription.`, t, func(t *ftt.Test) {
		sub := NewSubscription("foo", "testSubscription")
		assert.Loosely(t, sub, should.Equal(Subscription("projects/foo/subscriptions/testSubscription")))
		assert.Loosely(t, sub.Validate(), should.BeNil)
	})

	ftt.Run(`Can extract a resource project.`, t, func(t *ftt.Test) {
		p, n, err := resourceProjectName("projects/foo/topics/bar")
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, p, should.Equal("foo"))
		assert.Loosely(t, n, should.Equal("bar"))
	})
}
