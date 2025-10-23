// Copyright 2025 The LUCI Authors.
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

package rpc

import (
	"context"

	"go.chromium.org/luci/common/errors"
	mailer_pb "go.chromium.org/luci/mailer/api/mailer"
	"go.chromium.org/luci/server/mailer"
)

const luciNotifySendEmailsGroup = "luci-notify-send-emails"

type mailerServer struct {
	mailer_pb.UnimplementedMailerServer
}

var _ mailer_pb.MailerServer = &mailerServer{}

func NewMailerServer() *mailerServer {
	return &mailerServer{}
}

func (s *mailerServer) SendMail(ctx context.Context, req *mailer_pb.SendMailRequest) (*mailer_pb.SendMailResponse, error) {
	if err := checkAllowed(ctx, luciNotifySendEmailsGroup); err != nil {
		return nil, err
	}

	// Validate the request.
	if len(req.To) == 0 && len(req.Cc) == 0 && len(req.Bcc) == 0 {
		return nil, invalidArgumentError(errors.Fmt("at least one of To, Cc, or Bcc must be non-empty"))
	}
	if req.TextBody == "" && req.HtmlBody == "" {
		return nil, invalidArgumentError(errors.Fmt("at least one of TextBody or HtmlBody must be non-empty"))
	}

	// Send the message.
	if err := mailer.Send(ctx, &mailer.Mail{
		Sender:   req.Sender,
		ReplyTo:  req.ReplyTo,
		To:       req.To,
		Cc:       req.Cc,
		Bcc:      req.Bcc,
		Subject:  req.Subject,
		TextBody: req.TextBody,
		HTMLBody: req.HtmlBody,
	}); err != nil {
		return nil, err
	}
	return &mailer_pb.SendMailResponse{}, nil
}
