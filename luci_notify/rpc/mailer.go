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
	"strings"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/grpc/appstatus"
	mailer_pb "go.chromium.org/luci/mailer/api/mailer"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/mailer"

	"go.chromium.org/luci/luci_notify/notify"
)

const (
	luciNotifySendEmailsGroup = "luci-notify-send-emails"

	// maxSendMailRecipients caps the total number of recipients (across To, Cc,
	// and Bcc) accepted in a single SendMail RPC, matching the App Engine Mail
	// API per-message recipient limit.
	maxSendMailRecipients = 100
)

type mailerServer struct {
	mailer_pb.UnimplementedMailerServer
}

var _ mailer_pb.MailerServer = &mailerServer{}

func NewMailerServer() *mailerServer {
	return &mailerServer{}
}

func (s *mailerServer) SendMail(ctx context.Context, req *mailer_pb.SendMailRequest) (*mailer_pb.SendMailResponse, error) {
	resp, err := s.sendMail(ctx, req)
	return resp, appstatus.GRPCifyAndLog(ctx, err)
}

func (s *mailerServer) sendMail(ctx context.Context, req *mailer_pb.SendMailRequest) (*mailer_pb.SendMailResponse, error) {
	if err := checkAllowed(ctx, luciNotifySendEmailsGroup); err != nil {
		return nil, err
	}

	// Validate the request.
	if req.Sender != "" {
		return nil, invalidArgumentError(errors.New("sender must not be specified"))
	}
	if req.ReplyTo != "" {
		return nil, invalidArgumentError(errors.New("reply_to must not be specified"))
	}
	totalRecipients := len(req.To) + len(req.Cc) + len(req.Bcc)
	if totalRecipients == 0 {
		return nil, invalidArgumentError(errors.New("at least one of To, Cc, or Bcc must be non-empty"))
	}
	if totalRecipients > maxSendMailRecipients {
		return nil, invalidArgumentError(errors.Fmt("total recipients (%d) must not exceed %d", totalRecipients, maxSendMailRecipients))
	}
	if req.TextBody == "" && req.HtmlBody == "" {
		return nil, invalidArgumentError(errors.New("at least one of TextBody or HtmlBody must be non-empty"))
	}
	if err := validateNoHeaderNewlines(req); err != nil {
		return nil, invalidArgumentError(err)
	}

	caller := auth.CurrentIdentity(ctx)
	allowedTo := filterAllowedRecipients(ctx, caller, "To", req.To)
	allowedCc := filterAllowedRecipients(ctx, caller, "Cc", req.Cc)
	allowedBcc := filterAllowedRecipients(ctx, caller, "Bcc", req.Bcc)
	if len(allowedTo) == 0 && len(allowedCc) == 0 && len(allowedBcc) == 0 {
		return nil, invalidArgumentError(errors.New("no allowed recipients in To, Cc, or Bcc"))
	}

	logging.Fields{
		"caller":  string(caller),
		"to":      allowedTo,
		"cc":      allowedCc,
		"bcc":     allowedBcc,
		"subject": req.Subject,
	}.Infof(ctx, "Mailer.SendMail sending email")

	// Always leave Sender and ReplyTo empty so callers cannot spoof email headers.
	messageID, err := mailer.Send(ctx, &mailer.Mail{
		Sender:     "",
		ReplyTo:    "",
		To:         allowedTo,
		Cc:         allowedCc,
		Bcc:        allowedBcc,
		Subject:    req.Subject,
		TextBody:   req.TextBody,
		HTMLBody:   req.HtmlBody,
		InReplyTo:  req.InReplyTo,
		References: req.References,
	})
	if err != nil {
		return nil, err
	}
	return &mailer_pb.SendMailResponse{
		MessageId: messageID,
	}, nil
}

func filterAllowedRecipients(ctx context.Context, caller identity.Identity, field string, recipients []string) []string {
	var allowed []string
	for _, r := range recipients {
		if notify.IsAllowedRecipientAddress(r) {
			allowed = append(allowed, r)
		} else {
			logging.Warningf(ctx, "Mailer.SendMail caller %q specified %s recipient with non-allowed address %q; dropping", caller, field, r)
		}
	}
	return allowed
}

func validateNoHeaderNewlines(req *mailer_pb.SendMailRequest) error {
	for _, f := range []struct {
		name  string
		value string
	}{
		{name: "subject", value: req.Subject},
		{name: "in_reply_to", value: req.InReplyTo},
	} {
		if strings.ContainsAny(f.value, "\r\n") {
			return errors.Fmt("%s must not contain newlines", f.name)
		}
	}
	for _, ref := range req.References {
		if strings.ContainsAny(ref, "\r\n") {
			return errors.New("references must not contain newlines")
		}
	}
	for _, group := range []struct {
		field      string
		recipients []string
	}{
		{field: "To", recipients: req.To},
		{field: "Cc", recipients: req.Cc},
		{field: "Bcc", recipients: req.Bcc},
	} {
		for _, r := range group.recipients {
			if strings.ContainsAny(r, "\r\n") {
				return errors.Fmt("%s recipient must not contain newlines", group.field)
			}
		}
	}
	return nil
}
