// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package mail

import (
	net_mail "net/mail"
	"testing"

	. "github.com/luci/luci-go/common/testing/assertions"
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
			So(m2, ShouldResembleV, m)

			// make sure it's really a copy
			m2.To[0] = "fake@faker.example.com"
			So(m2.To, ShouldNotResembleV, m.To)

			m2.Headers["SomethingElse"] = []string{"noooo"}
			So(m2.Headers, ShouldNotResembleV, m.Headers)
		})

		Convey("TestMessage is copyable", func() {
			tm := &TestMessage{*m, []string{"application/msword"}}
			tm2 := tm.Copy()
			So(tm, ShouldResembleV, tm2)

			tm2.MIMETypes[0] = "spam"
			So(tm, ShouldNotResembleV, tm2)
		})

		Convey("Message can be cast to an SDK Message", func() {
			m2 := m.ToSDKMessage()
			So(m2.Sender, ShouldResembleV, m.Sender)
			So(m2.ReplyTo, ShouldResembleV, m.ReplyTo)
			So(m2.To, ShouldResembleV, m.To)
			So(m2.Cc, ShouldResembleV, m.Cc)
			So(m2.Bcc, ShouldResembleV, m.Bcc)
			So(m2.Subject, ShouldResembleV, m.Subject)
			So(m2.Body, ShouldResembleV, m.Body)
			So(m2.HTMLBody, ShouldResembleV, m.HTMLBody)
			So(m2.Headers, ShouldResembleV, m.Headers)

			So((Attachment)(m2.Attachments[0]), ShouldResembleV, m.Attachments[0])
		})
	})
}
