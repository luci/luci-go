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

	. "github.com/smartystreets/goconvey/convey"
)

func TestDataTypes(t *testing.T) {
	t.Parallel()

	Convey("data types", t, func() {
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

		Convey("empty copy doesn't do extra allocations", func() {
			m := (&Message{}).Copy()
			So(m.To, ShouldBeNil)
			So(m.Cc, ShouldBeNil)
			So(m.Bcc, ShouldBeNil)
			So(m.Attachments, ShouldBeNil)
			So(m.Headers, ShouldBeNil)

			tm := (&TestMessage{}).Copy()
			So(tm.MIMETypes, ShouldBeNil)
		})

		Convey("can copy nil", func() {
			So((*Message)(nil).Copy(), ShouldBeNil)
			So((*Message)(nil).ToSDKMessage(), ShouldBeNil)
			So((*TestMessage)(nil).Copy(), ShouldBeNil)
		})

		Convey("Message is copyable", func() {
			m2 := m.Copy()
			So(m2, ShouldResemble, m)

			// make sure it's really a copy
			m2.To[0] = "fake@faker.example.com"
			So(m2.To, ShouldNotResemble, m.To)

			m2.Headers["SomethingElse"] = []string{"noooo"}
			So(m2.Headers, ShouldNotResemble, m.Headers)
		})

		Convey("TestMessage is copyable", func() {
			tm := &TestMessage{*m, []string{"application/msword"}}
			tm2 := tm.Copy()
			So(tm, ShouldResemble, tm2)

			tm2.MIMETypes[0] = "spam"
			So(tm, ShouldNotResemble, tm2)
		})

		Convey("Message can be cast to an SDK Message", func() {
			m2 := m.ToSDKMessage()
			So(m2.Sender, ShouldResemble, m.Sender)
			So(m2.ReplyTo, ShouldResemble, m.ReplyTo)
			So(m2.To, ShouldResemble, m.To)
			So(m2.Cc, ShouldResemble, m.Cc)
			So(m2.Bcc, ShouldResemble, m.Bcc)
			So(m2.Subject, ShouldResemble, m.Subject)
			So(m2.Body, ShouldResemble, m.Body)
			So(m2.HTMLBody, ShouldResemble, m.HTMLBody)
			So(m2.Headers, ShouldResemble, m.Headers)

			So((Attachment)(m2.Attachments[0]), ShouldResemble, m.Attachments[0])
		})
	})
}
