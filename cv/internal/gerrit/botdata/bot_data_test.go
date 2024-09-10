// Copyright 2020 The LUCI Authors.
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

package botdata

import (
	"testing"
	"time"

	gerritpb "go.chromium.org/luci/common/proto/gerrit"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestParse(t *testing.T) {
	t.Parallel()

	ftt.Run("Parse", t, func(t *ftt.Test) {

		t.Run("Nil ChangeMessageInfo", func(t *ftt.Test) {
			_, ok := Parse(nil)
			assert.Loosely(t, ok, should.BeFalse)
		})

		t.Run("Success", func(t *ftt.Test) {
			cmi := &gerritpb.ChangeMessageInfo{
				Message: `human message

				Bot data: {"action":"start","triggered_at":"2013-03-23T21:36:52.332Z","revision":"abcd","cls":["chromium-review.googlesource.com:1111","chromium-review.googlesource.com:2222"]}`,
			}
			ret, ok := Parse(cmi)
			assert.Loosely(t, ok, should.BeTrue)
			assert.Loosely(t, ret, should.Resemble(BotData{
				Action:      Start,
				TriggeredAt: time.Date(2013, 03, 23, 21, 36, 52, 332000000, time.UTC),
				Revision:    "abcd",
			}))
		})

		t.Run("BotDataPrefix Missing", func(t *ftt.Test) {
			cmi := &gerritpb.ChangeMessageInfo{
				Message: `{"action": "start", "triggered_at": "2013-03-23T21:36:52.332Z", "revision": "abcdef"}`,
			}
			_, ok := Parse(cmi)
			assert.Loosely(t, ok, should.BeFalse)
		})

		t.Run("Invalid BotData JSON string", func(t *ftt.Test) {
			cmi := &gerritpb.ChangeMessageInfo{
				Message: `Bot data: I'm a plain string`,
			}
			_, ok := Parse(cmi)
			assert.Loosely(t, ok, should.BeFalse)
		})
	})
}

func TestAppend(t *testing.T) {
	t.Parallel()

	ftt.Run("Append", t, func(t *ftt.Test) {
		t.Run("Bot Message too long", func(t *ftt.Test) {
			bd := BotData{
				Action:      Cancel,
				TriggeredAt: time.Date(2013, 03, 23, 21, 36, 52, 332000000, time.UTC),
				Revision:    "abcd",
			}
			_, err := append("", bd, 10)
			assert.Loosely(t, err, should.ErrLike("bot data too long; max length: 10"))
		})

		t.Run("Empty human message", func(t *ftt.Test) {
			bd := BotData{
				Action:      Cancel,
				TriggeredAt: time.Date(2013, 03, 23, 21, 36, 52, 332000000, time.UTC),
				Revision:    "abcd",
			}
			ret, err := append("", bd, 1000)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, ret, should.Equal(`Bot data: {"action":"cancel","triggered_at":"2013-03-23T21:36:52.332Z","revision":"abcd"}`))
		})

		t.Run("Full human message", func(t *ftt.Test) {
			bd := BotData{
				Action:      Cancel,
				TriggeredAt: time.Date(2013, 03, 23, 21, 36, 52, 332000000, time.UTC),
				Revision:    "abcd",
			}
			ret, err := append("Message for human", bd, 1000)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, ret, should.Equal(`Message for human

Bot data: {"action":"cancel","triggered_at":"2013-03-23T21:36:52.332Z","revision":"abcd"}`))
		})

		t.Run("Truncated human message", func(t *ftt.Test) {
			bd := BotData{
				Action:      Cancel,
				TriggeredAt: time.Date(2013, 03, 23, 21, 36, 52, 332000000, time.UTC),
				Revision:    "abcd",
			}
			ret, err := append("Message for human. But it's way tooooooooooooooooooooooo long", bd, 150)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, ret, should.Equal(`Message for human. But it's
...[truncated too long message]

Bot data: {"action":"cancel","triggered_at":"2013-03-23T21:36:52.332Z","revision":"abcd"}`))
		})

		t.Run("Bot Data too long to fit human message", func(t *ftt.Test) {
			bd := BotData{
				Action:      Cancel,
				TriggeredAt: time.Date(2013, 03, 23, 21, 36, 52, 332000000, time.UTC),
				Revision:    "abcd",
			}
			// Bot data itself is 89 characters long already.
			// Placeholder is 31 characters long.
			_, err := append("Message for human", bd, 100)
			assert.Loosely(t, err, should.ErrLike("bot data too long to display human message; max length: 100"))
		})
	})
}
