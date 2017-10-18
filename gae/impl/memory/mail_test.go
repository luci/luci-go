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

package memory

import (
	net_mail "net/mail"
	"testing"

	"go.chromium.org/gae/service/mail"
	"go.chromium.org/gae/service/user"

	"golang.org/x/net/context"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestMail(t *testing.T) {
	t.Parallel()

	Convey("mail", t, func() {
		c := Use(context.Background())

		Convey("good cases", func() {
			Convey("start with an empty set of messages", func() {
				So(mail.GetTestable(c).SentMessages(), ShouldBeEmpty)
			})

			Convey("can send a message from the admin", func() {
				So(mail.Send(c, &mail.Message{
					Sender:  "admin@example.com",
					To:      []string{"Valued Customer <customer@example.com>"},
					Subject: "You are valued.",
					Body:    "We value you.",
				}), ShouldBeNil)

				Convey("and it shows up in sent messages", func() {
					So(mail.GetTestable(c).SentMessages(), ShouldResemble, []*mail.TestMessage{
						{Message: mail.Message{
							Sender:  "admin@example.com",
							To:      []string{"Valued Customer <customer@example.com>"},
							Subject: "You are valued.",
							Body:    "We value you.",
						}},
					})

					Convey("which can be reset", func() {
						mail.GetTestable(c).Reset()
						So(mail.GetTestable(c).SentMessages(), ShouldBeEmpty)
					})
				})
			})

			Convey("can send a message on behalf of a user", func() {
				user.GetTestable(c).Login("dood@example.com", "", false)
				So(mail.Send(c, &mail.Message{
					Sender:  "Friendly Person <dood@example.com>",
					To:      []string{"Other Friendly Person <dudette@example.com>"},
					Subject: "Hi",
					Body:    "An app is sending a message for me. It's the future.",
				}), ShouldBeNil)
			})

			Convey("can send a message to the admins", func() {
				So(mail.SendToAdmins(c, &mail.Message{
					Sender:  "admin@example.com",
					Subject: "Reminder",
					Body:    "I forgot",
				}), ShouldBeNil)

				So(mail.GetTestable(c).SentMessages(), ShouldResemble, []*mail.TestMessage{
					{Message: mail.Message{
						Sender:  "admin@example.com",
						To:      []string{"admin@example.com"},
						Subject: "Reminder",
						Body:    "I forgot",
					}},
				})
			})

			Convey("can set admin emails", func() {
				mail.GetTestable(c).SetAdminEmails(
					"Friendly <hello@example.com>",
					"Epic <nerdsnipe@example.com>",
				)

				So(mail.SendToAdmins(c, &mail.Message{
					Sender:  "hello@example.com",
					Subject: "Reminder",
					Body:    "I forgot",
				}), ShouldBeNil)

				So(mail.GetTestable(c).SentMessages(), ShouldResemble, []*mail.TestMessage{
					{Message: mail.Message{
						Sender: "hello@example.com",
						To: []string{
							"Friendly <hello@example.com>",
							"Epic <nerdsnipe@example.com>",
						},
						Subject: "Reminder",
						Body:    "I forgot",
					}},
				})
			})

			Convey("attachments get mimetypes assigned to them", func() {
				So(mail.SendToAdmins(c, &mail.Message{
					Sender:  "admin@example.com",
					Subject: "Reminder",
					Body:    "I forgot",
					Attachments: []mail.Attachment{
						{Name: "reminder.txt", Data: []byte("bananas")},
						{Name: "coolthing", Data: []byte("bananas")},
					},
				}), ShouldBeNil)

				So(mail.GetTestable(c).SentMessages(), ShouldResemble, []*mail.TestMessage{
					{
						Message: mail.Message{
							Sender:  "admin@example.com",
							To:      []string{"admin@example.com"},
							Subject: "Reminder",
							Body:    "I forgot",
							Attachments: []mail.Attachment{
								{Name: "reminder.txt", Data: []byte("bananas")},
								{Name: "coolthing", Data: []byte("bananas")},
							},
						},
						MIMETypes: []string{"text/plain", "application/octet-stream"}},
				})
			})

			Convey("can have headers", func() {
				So(mail.SendToAdmins(c, &mail.Message{
					Sender:  "admin@example.com",
					Subject: "Reminder",
					Body:    "I forgot",
					Headers: net_mail.Header{
						"in-reply-to": []string{"epicness"},
						"List-Id":     []string{"spam"},
					},
				}), ShouldBeNil)

				So(mail.GetTestable(c).SentMessages(), ShouldResemble, []*mail.TestMessage{
					{Message: mail.Message{
						Sender:  "admin@example.com",
						To:      []string{"admin@example.com"},
						Subject: "Reminder",
						Body:    "I forgot",
						Headers: net_mail.Header{
							"In-Reply-To": []string{"epicness"},
							"List-Id":     []string{"spam"},
						},
					}},
				})

			})
		})

		Convey("errors", func() {
			Convey("setting a non-email is a panic", func() {
				So(func() { mail.GetTestable(c).SetAdminEmails("i am a banana") },
					ShouldPanicLike, `invalid email ("i am a banana")`)
			})

			Convey("sending from a non-user, non-admin is an error", func() {
				mail.GetTestable(c).SetAdminEmails("Friendly <hello@example.com>")

				So(mail.Send(c, &mail.Message{
					Sender:  "someone_else@example.com",
					Subject: "Reminder",
					Body:    "I forgot",
				}), ShouldErrLike, "invalid Sender: someone_else@example.com")
			})

			Convey("sending from a bogus address is a problem", func() {
				So(mail.Send(c, &mail.Message{
					Sender: "lalal",
				}), ShouldErrLike, "unparsable Sender address: lalal")
			})

			Convey("sending with no recipients is a problem", func() {
				So(mail.Send(c, &mail.Message{
					Sender: "admin@example.com",
				}), ShouldErrLike, "one of To, Cc or Bcc must be non-empty")
			})

			Convey("bad addresses are a problem", func() {
				So(mail.Send(c, &mail.Message{
					Sender: "admin@example.com",
					To:     []string{"wut"},
				}), ShouldErrLike, `invalid email ("wut")`)

				So(mail.Send(c, &mail.Message{
					Sender: "admin@example.com",
					Cc:     []string{"wut"},
				}), ShouldErrLike, `invalid email ("wut")`)

				So(mail.Send(c, &mail.Message{
					Sender: "admin@example.com",
					Bcc:    []string{"wut"},
				}), ShouldErrLike, `invalid email ("wut")`)
			})

			Convey("no body is a problem", func() {
				So(mail.Send(c, &mail.Message{
					Sender: "admin@example.com",
					To:     []string{"wut@example.com"},
				}), ShouldErrLike, `one of Body or HTMLBody must be non-empty`)
			})

			Convey("bad attachments are a problem", func() {
				So(mail.Send(c, &mail.Message{
					Sender: "admin@example.com",
					To:     []string{"wut@example.com"},
					Body:   "nice thing",
					Attachments: []mail.Attachment{
						{Name: "nice.exe", Data: []byte("boom")},
					},
				}), ShouldErrLike, `illegal attachment extension for "nice.exe"`)
			})

			Convey("bad headers are a problem", func() {
				So(mail.SendToAdmins(c, &mail.Message{
					Sender:  "admin@example.com",
					Subject: "Reminder",
					Body:    "I forgot",
					Headers: net_mail.Header{"x-spam-cool": []string{"value"}},
				}), ShouldErrLike, `disallowed header: x-spam-cool`)

			})

		})

	})
}
