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
	"fmt"
	"math/rand"
	"net/textproto"
	"testing"
	"time"

	"github.com/jordan-wright/email"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/common/data/caching/lru"
	"go.chromium.org/luci/common/data/rand/mathrand"
	"go.chromium.org/luci/mailer/api/mailer"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
	"go.chromium.org/luci/server/caching/cachingtest"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestMailer(t *testing.T) {
	t.Parallel()

	testReq := &mailer.SendMailRequest{
		RequestId: "some-request-id",
		Sender:    "sender@example.com",
		ReplyTo:   []string{"replyto-1@example.com", "replyto-2@example.com"},
		To:        []string{"to-1@example.com", "to-2@example.com"},
		Cc:        []string{"cc-1@example.com", "cc-2@example.com"},
		Bcc:       []string{"bcc-1@example.com", "bcc-2@example.com"},
		Subject:   "Subject",
		TextBody:  "Text body",
		HtmlBody:  "HTML body",
	}

	// Depends on rand.NewSource(...) seed below.
	expectedMessageID := "f1405ced8b9968baf9109259515bf7025a291b00"

	expectedEmail := email.NewEmail()
	expectedEmail.From = testReq.Sender
	expectedEmail.ReplyTo = testReq.ReplyTo
	expectedEmail.To = testReq.To
	expectedEmail.Cc = testReq.Cc
	expectedEmail.Bcc = testReq.Bcc
	expectedEmail.Subject = testReq.Subject
	expectedEmail.Text = []byte(testReq.TextBody)
	expectedEmail.HTML = []byte(testReq.HtmlBody)
	expectedEmail.Headers.Set("Message-Id", fmt.Sprintf("<%s@luci.api.cr.dev>", expectedMessageID))

	Convey("With mailer", t, func() {
		var sendErr error
		var mails []*email.Email

		srv := &mailerServer{
			callersGroup: "auth-mailer-access",
			cache: &cachingtest.BlobCache{
				LRU: lru.New(0),
			},
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

		Convey("OK", func() {
			resp, err := srv.SendMail(ctx, testReq)
			So(err, ShouldBeNil)
			So(resp, ShouldResembleProto, &mailer.SendMailResponse{
				MessageId: expectedMessageID,
			})

			So(mails, ShouldHaveLength, 1)
			So(mails[0], ShouldResemble, expectedEmail)

			Convey("Deduplication", func() {
				resp, err := srv.SendMail(ctx, testReq)
				So(err, ShouldBeNil)
				So(resp, ShouldResembleProto, &mailer.SendMailResponse{
					MessageId: expectedMessageID,
				})
				So(mails, ShouldHaveLength, 1) // no new calls
			})
		})

		Convey("Transient error", func() {
			sendErr = &textproto.Error{Code: 400}
			_, err := srv.SendMail(ctx, testReq)
			So(status.Code(err), ShouldEqual, codes.Internal)
		})

		Convey("Fatal error", func() {
			sendErr = &textproto.Error{Code: 500}
			_, err := srv.SendMail(ctx, testReq)
			So(status.Code(err), ShouldEqual, codes.InvalidArgument)
		})

		Convey("Authorization error", func() {
			ctx := auth.WithState(ctx, &authtest.FakeState{
				Identity: "user:user@example.com",
			})
			_, err := srv.SendMail(ctx, testReq)
			So(status.Code(err), ShouldEqual, codes.PermissionDenied)
		})
	})
}
