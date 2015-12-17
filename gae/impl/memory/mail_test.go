// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package memory

import (
	net_mail "net/mail"
	"testing"

	mailS "github.com/luci/gae/service/mail"
	userS "github.com/luci/gae/service/user"
	. "github.com/luci/luci-go/common/testing/assertions"
	. "github.com/smartystreets/goconvey/convey"
	"golang.org/x/net/context"
)

func TestMail(t *testing.T) {
	t.Parallel()

	Convey("mail", t, func() {
		c := Use(context.Background())
		user := userS.Get(c)
		mail := mailS.Get(c)

		Convey("good cases", func() {
			Convey("start with an empty set of messages", func() {
				So(mail.Testable().SentMessages(), ShouldBeEmpty)
			})

			Convey("can send a message from the admin", func() {
				So(mail.Send(&mailS.Message{
					Sender:  "admin@example.com",
					To:      []string{"Valued Customer <customer@example.com>"},
					Subject: "You are valued.",
					Body:    "We value you.",
				}), ShouldBeNil)

				Convey("and it shows up in sent messages", func() {
					So(mail.Testable().SentMessages(), ShouldResembleV, []*mailS.TestMessage{
						{Message: mailS.Message{
							Sender:  "admin@example.com",
							To:      []string{"Valued Customer <customer@example.com>"},
							Subject: "You are valued.",
							Body:    "We value you.",
						}},
					})

					Convey("which can be reset", func() {
						mail.Testable().Reset()
						So(mail.Testable().SentMessages(), ShouldBeEmpty)
					})
				})
			})

			Convey("can send a message on behalf of a user", func() {
				user.Testable().Login("dood@example.com", "", false)
				So(mail.Send(&mailS.Message{
					Sender:  "Friendly Person <dood@example.com>",
					To:      []string{"Other Friendly Person <dudette@example.com>"},
					Subject: "Hi",
					Body:    "An app is sending a message for me. It's the future.",
				}), ShouldBeNil)
			})

			Convey("can send a message to the admins", func() {
				So(mail.SendToAdmins(&mailS.Message{
					Sender:  "admin@example.com",
					Subject: "Reminder",
					Body:    "I forgot",
				}), ShouldBeNil)

				So(mail.Testable().SentMessages(), ShouldResembleV, []*mailS.TestMessage{
					{Message: mailS.Message{
						Sender:  "admin@example.com",
						To:      []string{"admin@example.com"},
						Subject: "Reminder",
						Body:    "I forgot",
					}},
				})
			})

			Convey("can set admin emails", func() {
				mail.Testable().SetAdminEmails(
					"Friendly <hello@example.com>",
					"Epic <nerdsnipe@example.com>",
				)

				So(mail.SendToAdmins(&mailS.Message{
					Sender:  "hello@example.com",
					Subject: "Reminder",
					Body:    "I forgot",
				}), ShouldBeNil)

				So(mail.Testable().SentMessages(), ShouldResembleV, []*mailS.TestMessage{
					{Message: mailS.Message{
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
				So(mail.SendToAdmins(&mailS.Message{
					Sender:  "admin@example.com",
					Subject: "Reminder",
					Body:    "I forgot",
					Attachments: []mailS.Attachment{
						{Name: "reminder.txt", Data: []byte("bananas")},
						{Name: "coolthing", Data: []byte("bananas")},
					},
				}), ShouldBeNil)

				So(mail.Testable().SentMessages(), ShouldResembleV, []*mailS.TestMessage{
					{
						Message: mailS.Message{
							Sender:  "admin@example.com",
							To:      []string{"admin@example.com"},
							Subject: "Reminder",
							Body:    "I forgot",
							Attachments: []mailS.Attachment{
								{Name: "reminder.txt", Data: []byte("bananas")},
								{Name: "coolthing", Data: []byte("bananas")},
							},
						},
						MIMETypes: []string{"text/plain", "application/octet-stream"}},
				})
			})

			Convey("can have headers", func() {
				So(mail.SendToAdmins(&mailS.Message{
					Sender:  "admin@example.com",
					Subject: "Reminder",
					Body:    "I forgot",
					Headers: net_mail.Header{
						"in-reply-to": []string{"epicness"},
						"List-Id":     []string{"spam"},
					},
				}), ShouldBeNil)

				So(mail.Testable().SentMessages(), ShouldResembleV, []*mailS.TestMessage{
					{Message: mailS.Message{
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
				So(func() { mail.Testable().SetAdminEmails("i am a banana") },
					ShouldPanicLike, `invalid email ("i am a banana"): mail: missing phrase`)
			})

			Convey("sending from a non-user, non-admin is an error", func() {
				mail.Testable().SetAdminEmails("Friendly <hello@example.com>")

				So(mail.Send(&mailS.Message{
					Sender:  "someone_else@example.com",
					Subject: "Reminder",
					Body:    "I forgot",
				}), ShouldErrLike, "invalid Sender: someone_else@example.com")
			})

			Convey("sending from a bogus address is a problem", func() {
				So(mail.Send(&mailS.Message{
					Sender: "lalal",
				}), ShouldErrLike, "unparsable Sender address: lalal: mail: missing phrase")
			})

			Convey("sending with no recipients is a problem", func() {
				So(mail.Send(&mailS.Message{
					Sender: "admin@example.com",
				}), ShouldErrLike, "one of To, Cc or Bcc must be non-empty")
			})

			Convey("bad addresses are a problem", func() {
				So(mail.Send(&mailS.Message{
					Sender: "admin@example.com",
					To:     []string{"wut"},
				}), ShouldErrLike, `invalid email ("wut"): mail: missing phrase`)

				So(mail.Send(&mailS.Message{
					Sender: "admin@example.com",
					Cc:     []string{"wut"},
				}), ShouldErrLike, `invalid email ("wut"): mail: missing phrase`)

				So(mail.Send(&mailS.Message{
					Sender: "admin@example.com",
					Bcc:    []string{"wut"},
				}), ShouldErrLike, `invalid email ("wut"): mail: missing phrase`)
			})

			Convey("no body is a problem", func() {
				So(mail.Send(&mailS.Message{
					Sender: "admin@example.com",
					To:     []string{"wut@example.com"},
				}), ShouldErrLike, `one of Body or HTMLBody must be non-empty`)
			})

			Convey("bad attachments are a problem", func() {
				So(mail.Send(&mailS.Message{
					Sender: "admin@example.com",
					To:     []string{"wut@example.com"},
					Body:   "nice thing",
					Attachments: []mailS.Attachment{
						{Name: "nice.exe", Data: []byte("boom")},
					},
				}), ShouldErrLike, `illegal attachment extension for "nice.exe"`)
			})

			Convey("bad headers are a problem", func() {
				So(mail.SendToAdmins(&mailS.Message{
					Sender:  "admin@example.com",
					Subject: "Reminder",
					Body:    "I forgot",
					Headers: net_mail.Header{"x-spam-cool": []string{"value"}},
				}), ShouldErrLike, `disallowed header: x-spam-cool`)

			})

		})

	})
}
