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

package streamclient

import (
	"context"
	"testing"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	"go.chromium.org/luci/logdog/client/butlerlib/streamproto"
)

func TestOptions(t *testing.T) {
	t.Parallel()

	ftt.Run(`options`, t, func(t *ftt.Test) {
		scFake, client := NewUnregisteredFake("")

		ctx, _ := testclock.UseTime(context.Background(), testclock.TestTimeUTC)

		t.Run(`defaults`, func(t *ftt.Test) {
			_, err := client.NewStream(ctx, "test")
			assert.Loosely(t, err, should.BeNil)
			defaultFlags := scFake.Data()["test"].GetFlags()
			assert.Loosely(t, defaultFlags.ContentType, should.Equal("text/plain; charset=utf-8"))
			assert.Loosely(t, defaultFlags.Timestamp.Time(), should.Match(testclock.TestTimeUTC))
			assert.Loosely(t, defaultFlags.Tags, should.BeEmpty)
		})

		t.Run(`can change content type`, func(t *ftt.Test) {
			_, err := client.NewStream(ctx, "test", WithContentType("narple"))
			assert.Loosely(t, err, should.BeNil)
			testFlags := scFake.Data()["test"].GetFlags()
			assert.Loosely(t, testFlags.ContentType, should.Equal("narple"))
		})

		t.Run(`can set initial timestamp`, func(t *ftt.Test) {
			_, err := client.NewStream(ctx, "test", WithTimestamp(testclock.TestRecentTimeUTC))
			assert.Loosely(t, err, should.BeNil)
			testFlags := scFake.Data()["test"].GetFlags()
			assert.Loosely(t, testFlags.Timestamp.Time(), should.Match(testclock.TestRecentTimeUTC))
		})

		t.Run(`can set tags nicely`, func(t *ftt.Test) {
			_, err := client.NewStream(ctx, "test", WithTags(
				"key1", "value",
				"key2", "value",
			))
			assert.Loosely(t, err, should.BeNil)
			testFlags := scFake.Data()["test"].GetFlags()
			assert.Loosely(t, testFlags.Tags, should.Match(streamproto.TagMap{
				"key1": "value",
				"key2": "value",
			}))
		})

		t.Run(`WithTags expects an even number of args`, func(t *ftt.Test) {
			assert.Loosely(t, func() {
				WithTags("hi")
			}, should.PanicLike("even number of arguments"))
		})

		t.Run(`can set tags practically`, func(t *ftt.Test) {
			_, err := client.NewStream(ctx, "test", WithTagMap(map[string]string{
				"key1": "value",
				"key2": "value",
			}))
			assert.Loosely(t, err, should.BeNil)
			testFlags := scFake.Data()["test"].GetFlags()
			assert.Loosely(t, testFlags.Tags, should.Match(streamproto.TagMap{
				"key1": "value",
				"key2": "value",
			}))
		})
	})
}
