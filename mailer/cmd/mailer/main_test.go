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

package main

import (
	"context"
	"math/rand"
	"net/textproto"
	"testing"
	"time"

	"github.com/jordan-wright/email"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/common/data/rand/mathrand"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
	"go.chromium.org/luci/server/caching/cachingtest"

	"go.chromium.org/luci/mailer/api/mailer"
)

func TestMailer(t *testing.T) {
	t.Parallel()

	testReq := &mailer.SendMailRequest{
		RequestId: "some-request-id",
		Sender:    "sender@example.com",
		ReplyTo:   "replyto-1@example.com",
		To:        []string{"to-1@example.com", "to-2@example.com"},
		Cc:        []string{"cc-1@example.com", "cc-2@example.com"},
		Bcc:       []string{"bcc-1@example.com", "bcc-2@example.com"},
		Subject:   "Subject",
		TextBody:  "Text body",
		HtmlBody:  "HTML body",
		InReplyTo: "<some-other-msg>",
	}

	// Depends on rand.NewSource(...) seed below.
	expectedMessageID := "<f1405ced8b9968baf9109259515bf7025a291b00@luci.api.cr.dev>"

	expectedEmail := email.NewEmail()
	expectedEmail.From = testReq.Sender
	expectedEmail.ReplyTo = []string{testReq.ReplyTo}
	expectedEmail.To = testReq.To
	expectedEmail.Cc = testReq.Cc
	expectedEmail.Bcc = testReq.Bcc
	expectedEmail.Subject = testReq.Subject
	expectedEmail.Text = []byte(testReq.TextBody)
	expectedEmail.HTML = []byte(testReq.HtmlBody)
	expectedEmail.Headers.Set("Message-Id", expectedMessageID)
	expectedEmail.Headers.Set("In-Reply-To", "<some-other-msg>")

	ftt.Run("With mailer", t, func(t *ftt.Test) {
		var sendErr error
		var mails []*email.Email

		srv := &mailerServer{
			callersGroup: "auth-mailer-access",
			cache:        cachingtest.NewBlobCache(),
			send: func(msg *email.Email, timeout time.Duration) error {
				if sendErr != nil {
					return sendErr
				}
				mails = append(mails, msg)
				return nil
			},
		}

		ctx := mathrand.Set(context.Background(), rand.New(rand.NewSource(123)))
		ctx = auth.WithState(ctx, &authtest.FakeState{
			Identity:       "user:user@example.com",
			IdentityGroups: []string{"auth-mailer-access"},
		})

		t.Run("OK", func(t *ftt.Test) {
			resp, err := srv.SendMail(ctx, testReq)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, resp, should.Match(&mailer.SendMailResponse{
				MessageId: expectedMessageID,
			}))

			assert.Loosely(t, mails, should.HaveLength(1))
			assert.Loosely(t, mails[0], should.Match(expectedEmail))

			t.Run("Deduplication", func(t *ftt.Test) {
				resp, err := srv.SendMail(ctx, testReq)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, resp, should.Match(&mailer.SendMailResponse{
					MessageId: expectedMessageID,
				}))
				assert.Loosely(t, mails, should.HaveLength(1)) // no new calls
			})
		})

		t.Run("Transient error", func(t *ftt.Test) {
			sendErr = &textproto.Error{Code: 400}
			_, err := srv.SendMail(ctx, testReq)
			assert.Loosely(t, status.Code(err), should.Equal(codes.Internal))
		})

		t.Run("Fatal error", func(t *ftt.Test) {
			sendErr = &textproto.Error{Code: 500}
			_, err := srv.SendMail(ctx, testReq)
			assert.Loosely(t, status.Code(err), should.Equal(codes.InvalidArgument))
		})

		t.Run("Authorization error", func(t *ftt.Test) {
			ctx := auth.WithState(ctx, &authtest.FakeState{
				Identity: "user:user@example.com",
			})
			_, err := srv.SendMail(ctx, testReq)
			assert.Loosely(t, status.Code(err), should.Equal(codes.PermissionDenied))
		})
	})
}
