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

package mailer

import (
	"context"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/mailer/api/mailer"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestSend(t *testing.T) {
	t.Parallel()

	Convey("With mock client", t, func() {
		client := &mockClient{}
		ctx := Use(context.Background(), func(ctx context.Context, msg *Mail) error {
			return sendMail(ctx, client, msg)
		})

		Convey("OK", func() {
			err := Send(ctx, &Mail{
				Sender:   "sender@example.com",
				ReplyTo:  []string{"replyto-1@example.com", "replyto-2@example.com"},
				To:       []string{"to-1@example.com", "to-2@example.com"},
				Cc:       []string{"cc-1@example.com", "cc-2@example.com"},
				Bcc:      []string{"bcc-1@example.com", "bcc-2@example.com"},
				Subject:  "Subject",
				TextBody: "Text body",
				HTMLBody: "HTML body",
			})
			So(err, ShouldBeNil)

			So(client.calls, ShouldHaveLength, 1)
			So(client.calls[0].RequestId, ShouldHaveLength, 36)
			So(client.calls[0], ShouldResembleProto, &mailer.SendMailRequest{
				RequestId: client.calls[0].RequestId,
				Sender:    "sender@example.com",
				ReplyTo:   []string{"replyto-1@example.com", "replyto-2@example.com"},
				To:        []string{"to-1@example.com", "to-2@example.com"},
				Cc:        []string{"cc-1@example.com", "cc-2@example.com"},
				Bcc:       []string{"bcc-1@example.com", "bcc-2@example.com"},
				Subject:   "Subject",
				TextBody:  "Text body",
				HtmlBody:  "HTML body",
			})
		})

		Convey("Err", func() {
			client.err = status.Errorf(codes.PermissionDenied, "boom")
			So(Send(ctx, &Mail{}), ShouldEqual, client.err)
		})
	})
}

type mockClient struct {
	err   error
	calls []*mailer.SendMailRequest
}

func (cl *mockClient) SendMail(ctx context.Context, in *mailer.SendMailRequest, opts ...grpc.CallOption) (*mailer.SendMailResponse, error) {
	if cl.err != nil {
		return nil, cl.err
	}
	cl.calls = append(cl.calls, in)
	return &mailer.SendMailResponse{MessageId: "msg"}, nil
}
