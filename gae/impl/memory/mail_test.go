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
	"context"
	net_mail "net/mail"
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	"go.chromium.org/luci/gae/service/mail"
	"go.chromium.org/luci/gae/service/user"
)

func TestMail(t *testing.T) {
	t.Parallel()

	ftt.Run("mail", t, func(t *ftt.Test) {
		c := Use(context.Background())

		t.Run("good cases", func(t *ftt.Test) {
			t.Run("start with an empty set of messages", func(t *ftt.Test) {
				assert.Loosely(t, mail.GetTestable(c).SentMessages(), should.BeEmpty)
			})

			t.Run("can send a message from the admin", func(t *ftt.Test) {
				assert.Loosely(t, mail.Send(c, &mail.Message{
					Sender:  "admin@example.com",
					To:      []string{"Valued Customer <customer@example.com>"},
					Subject: "You are valued.",
					Body:    "We value you.",
				}), should.BeNil)

				t.Run("and it shows up in sent messages", func(t *ftt.Test) {
					assert.Loosely(t, mail.GetTestable(c).SentMessages(), should.Resemble([]*mail.TestMessage{
						{Message: mail.Message{
							Sender:  "admin@example.com",
							To:      []string{"Valued Customer <customer@example.com>"},
							Subject: "You are valued.",
							Body:    "We value you.",
						}},
					}))

					t.Run("which can be reset", func(t *ftt.Test) {
						mail.GetTestable(c).Reset()
						assert.Loosely(t, mail.GetTestable(c).SentMessages(), should.BeEmpty)
					})
				})
			})

			t.Run("can send a message on behalf of a user", func(t *ftt.Test) {
				user.GetTestable(c).Login("dood@example.com", "", false)
				assert.Loosely(t, mail.Send(c, &mail.Message{
					Sender:  "Friendly Person <dood@example.com>",
					To:      []string{"Other Friendly Person <dudette@example.com>"},
					Subject: "Hi",
					Body:    "An app is sending a message for me. It's the future.",
				}), should.BeNil)
			})

			t.Run("can send a message to the admins", func(t *ftt.Test) {
				assert.Loosely(t, mail.SendToAdmins(c, &mail.Message{
					Sender:  "admin@example.com",
					Subject: "Reminder",
					Body:    "I forgot",
				}), should.BeNil)

				assert.Loosely(t, mail.GetTestable(c).SentMessages(), should.Resemble([]*mail.TestMessage{
					{Message: mail.Message{
						Sender:  "admin@example.com",
						To:      []string{"admin@example.com"},
						Subject: "Reminder",
						Body:    "I forgot",
					}},
				}))
			})

			t.Run("can set admin emails", func(t *ftt.Test) {
				mail.GetTestable(c).SetAdminEmails(
					"Friendly <hello@example.com>",
					"Epic <nerdsnipe@example.com>",
				)

				assert.Loosely(t, mail.SendToAdmins(c, &mail.Message{
					Sender:  "hello@example.com",
					Subject: "Reminder",
					Body:    "I forgot",
				}), should.BeNil)

				assert.Loosely(t, mail.GetTestable(c).SentMessages(), should.Resemble([]*mail.TestMessage{
					{Message: mail.Message{
						Sender: "hello@example.com",
						To: []string{
							"Friendly <hello@example.com>",
							"Epic <nerdsnipe@example.com>",
						},
						Subject: "Reminder",
						Body:    "I forgot",
					}},
				}))
			})

			t.Run("attachments get mimetypes assigned to them", func(t *ftt.Test) {
				assert.Loosely(t, mail.SendToAdmins(c, &mail.Message{
					Sender:  "admin@example.com",
					Subject: "Reminder",
					Body:    "I forgot",
					Attachments: []mail.Attachment{
						{Name: "reminder.txt", Data: []byte("bananas")},
						{Name: "coolthing", Data: []byte("bananas")},
					},
				}), should.BeNil)

				assert.Loosely(t, mail.GetTestable(c).SentMessages(), should.Resemble([]*mail.TestMessage{
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
				}))
			})

			t.Run("can have headers", func(t *ftt.Test) {
				assert.Loosely(t, mail.SendToAdmins(c, &mail.Message{
					Sender:  "admin@example.com",
					Subject: "Reminder",
					Body:    "I forgot",
					Headers: net_mail.Header{
						"in-reply-to": []string{"epicness"},
						"List-Id":     []string{"spam"},
					},
				}), should.BeNil)

				assert.Loosely(t, mail.GetTestable(c).SentMessages(), should.Resemble([]*mail.TestMessage{
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
				}))

			})
		})

		t.Run("errors", func(t *ftt.Test) {
			t.Run("setting a non-email is a panic", func(t *ftt.Test) {
				assert.Loosely(t, func() { mail.GetTestable(c).SetAdminEmails("i am a banana") },
					should.PanicLike(`invalid email ("i am a banana")`))
			})

			t.Run("sending from a non-user, non-admin is an error", func(t *ftt.Test) {
				mail.GetTestable(c).SetAdminEmails("Friendly <hello@example.com>")

				assert.Loosely(t, mail.Send(c, &mail.Message{
					Sender:  "someone_else@example.com",
					Subject: "Reminder",
					Body:    "I forgot",
				}), should.ErrLike("invalid Sender: someone_else@example.com"))
			})

			t.Run("sending from a bogus address is a problem", func(t *ftt.Test) {
				assert.Loosely(t, mail.Send(c, &mail.Message{
					Sender: "lalal",
				}), should.ErrLike("unparsable Sender address: lalal"))
			})

			t.Run("sending with no recipients is a problem", func(t *ftt.Test) {
				assert.Loosely(t, mail.Send(c, &mail.Message{
					Sender: "admin@example.com",
				}), should.ErrLike("one of To, Cc or Bcc must be non-empty"))
			})

			t.Run("bad addresses are a problem", func(t *ftt.Test) {
				assert.Loosely(t, mail.Send(c, &mail.Message{
					Sender: "admin@example.com",
					To:     []string{"wut"},
				}), should.ErrLike(`invalid email ("wut")`))

				assert.Loosely(t, mail.Send(c, &mail.Message{
					Sender: "admin@example.com",
					Cc:     []string{"wut"},
				}), should.ErrLike(`invalid email ("wut")`))

				assert.Loosely(t, mail.Send(c, &mail.Message{
					Sender: "admin@example.com",
					Bcc:    []string{"wut"},
				}), should.ErrLike(`invalid email ("wut")`))
			})

			t.Run("no body is a problem", func(t *ftt.Test) {
				assert.Loosely(t, mail.Send(c, &mail.Message{
					Sender: "admin@example.com",
					To:     []string{"wut@example.com"},
				}), should.ErrLike(`one of Body or HTMLBody must be non-empty`))
			})

			t.Run("bad attachments are a problem", func(t *ftt.Test) {
				assert.Loosely(t, mail.Send(c, &mail.Message{
					Sender: "admin@example.com",
					To:     []string{"wut@example.com"},
					Body:   "nice thing",
					Attachments: []mail.Attachment{
						{Name: "nice.exe", Data: []byte("boom")},
					},
				}), should.ErrLike(`illegal attachment extension for "nice.exe"`))
			})

			t.Run("bad headers are a problem", func(t *ftt.Test) {
				assert.Loosely(t, mail.SendToAdmins(c, &mail.Message{
					Sender:  "admin@example.com",
					Subject: "Reminder",
					Body:    "I forgot",
					Headers: net_mail.Header{"x-spam-cool": []string{"value"}},
				}), should.ErrLike(`disallowed header: x-spam-cool`))

			})

		})

	})
}
