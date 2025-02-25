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

package mail

import (
	net_mail "net/mail"
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestDataTypes(t *testing.T) {
	t.Parallel()

	ftt.Run("data types", t, func(t *ftt.Test) {
		m := &Message{
			"from@example.com",
			"reply_to@example.com",
			[]string{"to1@example.com", "to2@example.com"},
			[]string{"cc1@example.com", "cc2@example.com"},
			[]string{"bcc1@example.com", "bcc2@example.com"},
			"subject",
			"body",
			"<html><body>body</body></html>",
			[]Attachment{{"name.doc", []byte("data"), "https://content_id"}},
			net_mail.Header{
				"In-Reply-To": []string{"cats"},
			},
		}

		t.Run("empty copy doesn't do extra allocations", func(t *ftt.Test) {
			m := (&Message{}).Copy()
			assert.Loosely(t, m.To, should.BeNil)
			assert.Loosely(t, m.Cc, should.BeNil)
			assert.Loosely(t, m.Bcc, should.BeNil)
			assert.Loosely(t, m.Attachments, should.BeNil)
			assert.Loosely(t, m.Headers, should.BeNil)

			tm := (&TestMessage{}).Copy()
			assert.Loosely(t, tm.MIMETypes, should.BeNil)
		})

		t.Run("can copy nil", func(t *ftt.Test) {
			assert.Loosely(t, (*Message)(nil).Copy(), should.BeNil)
			assert.Loosely(t, (*Message)(nil).ToSDKMessage(), should.BeNil)
			assert.Loosely(t, (*TestMessage)(nil).Copy(), should.BeNil)
		})

		t.Run("Message is copyable", func(t *ftt.Test) {
			m2 := m.Copy()
			assert.Loosely(t, m2, should.Match(m))

			// make sure it's really a copy
			m2.To[0] = "fake@faker.example.com"
			assert.Loosely(t, m2.To, should.NotResemble(m.To))

			m2.Headers["SomethingElse"] = []string{"noooo"}
			assert.Loosely(t, m2.Headers, should.NotResemble(m.Headers))
		})

		t.Run("TestMessage is copyable", func(t *ftt.Test) {
			tm := &TestMessage{*m, []string{"application/msword"}}
			tm2 := tm.Copy()
			assert.Loosely(t, tm, should.Match(tm2))

			tm2.MIMETypes[0] = "spam"
			assert.Loosely(t, tm, should.NotResemble(tm2))
		})

		t.Run("Message can be cast to an SDK Message", func(t *ftt.Test) {
			m2 := m.ToSDKMessage()
			assert.Loosely(t, m2.Sender, should.Match(m.Sender))
			assert.Loosely(t, m2.ReplyTo, should.Match(m.ReplyTo))
			assert.Loosely(t, m2.To, should.Match(m.To))
			assert.Loosely(t, m2.Cc, should.Match(m.Cc))
			assert.Loosely(t, m2.Bcc, should.Match(m.Bcc))
			assert.Loosely(t, m2.Subject, should.Match(m.Subject))
			assert.Loosely(t, m2.Body, should.Match(m.Body))
			assert.Loosely(t, m2.HTMLBody, should.Match(m.HTMLBody))
			assert.Loosely(t, m2.Headers, should.Match(m.Headers))

			assert.Loosely(t, (Attachment)(m2.Attachments[0]), should.Match(m.Attachments[0]))
		})
	})
}
