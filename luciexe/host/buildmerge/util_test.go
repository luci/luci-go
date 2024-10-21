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

package buildmerge

import (
	"fmt"
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/logdog/common/types"
)

func TestAbsolutize(t *testing.T) {
	t.Parallel()
	ftt.Run(`absolutize`, t, func(t *ftt.Test) {
		absolutize := func(logURL, viewURL string) (absLogURL, absViewURL string, err error) {
			return absolutizeURLs(logURL, viewURL, "ns/", func(ns, streamName types.StreamName) (url string, viewUrl string) {
				return fmt.Sprintf("url://%s%s", ns, streamName), fmt.Sprintf("viewURL://%s%s", ns, streamName)
			})

		}
		t.Run(`no-op if both urls are absolute`, func(t *ftt.Test) {
			absLogURL, absViewURL, err := absolutize("url://ns/log/foo", "viewURL://ns/log/foo")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, absLogURL, should.Equal("url://ns/log/foo"))
			assert.Loosely(t, absViewURL, should.Equal("viewURL://ns/log/foo"))
		})

		t.Run(`calc urls if log url is relative`, func(t *ftt.Test) {
			absLogURL, absViewURL, err := absolutize("log/foo", "")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, absLogURL, should.Equal("url://ns/log/foo"))
			assert.Loosely(t, absViewURL, should.Equal("viewURL://ns/log/foo"))
			t.Run(`even when log url has non-alnum char but valid stream char`, func(t *ftt.Test) {
				absLogURL, absViewURL, err := absolutize("log:hi.hello_hey-aloha/foo", "")
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, absLogURL, should.Equal("url://ns/log:hi.hello_hey-aloha/foo"))
				assert.Loosely(t, absViewURL, should.Equal("viewURL://ns/log:hi.hello_hey-aloha/foo"))
			})
			t.Run(`omits provided view url`, func(t *ftt.Test) {
				absLogURL, absViewURL, err := absolutize("log/foo", "Hi there!")
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, absLogURL, should.Equal("url://ns/log/foo"))
				assert.Loosely(t, absViewURL, should.Equal("viewURL://ns/log/foo"))
			})
		})

		t.Run(`error`, func(t *ftt.Test) {
			t.Run(`if log url is absolute`, func(t *ftt.Test) {
				t.Run(`but view url is empty`, func(t *ftt.Test) {
					absLogURL, absViewURL, err := absolutize("url://ns/log/foo", "")
					assert.Loosely(t, err, should.ErrLike("absolute log url is provided"))
					assert.Loosely(t, err, should.ErrLike("view url is empty"))
					assert.Loosely(t, absLogURL, should.Equal("url://ns/log/foo"))
					assert.Loosely(t, absViewURL, should.BeEmpty)
				})
				t.Run(`but view url is not absolute`, func(t *ftt.Test) {
					absLogURL, absViewURL, err := absolutize("url://ns/log/foo", "log/foo")
					assert.Loosely(t, err, should.ErrLike("expected absolute view url, got"))
					assert.Loosely(t, absLogURL, should.Equal("url://ns/log/foo"))
					assert.Loosely(t, absViewURL, should.Equal("log/foo"))
				})
			})

			t.Run(`if log url is relative but not a valid stream`, func(t *ftt.Test) {
				absLogURL, absViewURL, err := absolutize("log/foo#key=value", "")
				assert.Loosely(t, err, should.ErrLike("bad log url"))
				assert.Loosely(t, err, should.ErrLike("illegal character"))
				assert.Loosely(t, absLogURL, should.Equal("log/foo#key=value"))
				assert.Loosely(t, absViewURL, should.BeEmpty)
			})
		})
	})
}
