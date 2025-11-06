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

// Binary mailer implements the mailer server.
package main

import (
	"context"
	"flag"
	"fmt"
	"net/smtp"
	"net/textproto"
	"strings"
	"time"

	"github.com/jordan-wright/email"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/auth/scopes"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/server"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/openid"
	"go.chromium.org/luci/server/caching"
	"go.chromium.org/luci/server/limiter"
	shared_mailer "go.chromium.org/luci/server/mailer"
	"go.chromium.org/luci/server/module"
	"go.chromium.org/luci/server/redisconn"

	"go.chromium.org/luci/mailer/api/mailer"
)

// Note: to run this code locally you'll need an SMTP server. A simple solution
// is to run a test SMTP server (like https://github.com/mailhog/MailHog) via
// Docker:
//
// $ docker run -p 127.0.0.1:1025:1025 -p 127.0.0.1:8025:8025 mailhog/mailhog
// $ ./mailer -smtp-port 1025
// <open http://127.0.0.1:8025 in the browser to see MailHog's UI>

func main() {
	smtpPort := flag.Int("smtp-port", 1025,
		`A port of a localhost SMTP server to use.`)
	smtpPoolSize := flag.Int("smtp-pool-size", 100,
		`The maximum number of parallel connections that can be made to the SMTP server.`)
	callersGroup := flag.String("mailer-callers-group", "auth-mailer-access",
		`A LUCI group with callers authorized to use this service.`)

	modules := []module.Module{
		limiter.NewModuleFromFlags(),
		redisconn.NewModuleFromFlags(),
	}

	server.Main(nil, modules, func(srv *server.Server) error {
		smtpAddr := fmt.Sprintf("127.0.0.1:%d", *smtpPort)

		// Wait a bit until the SMTP server is up. This is important when it is
		// launched by Kubernetes in parallel to launching `mailer` server itself.
		if err := waitSMTP(srv.Context, smtpAddr); err != nil {
			return errors.Fmt("failed to connect to the SMTP server: %w", err)
		}

		// Use a connection pool to avoid opening/closing connections all the time.
		pool, err := email.NewPool(smtpAddr, *smtpPoolSize, nil)
		if err != nil {
			return errors.Fmt("failed to create SMTP connection pool: %w", err)
		}
		srv.RegisterCleanup(func(context.Context) { pool.Close() })

		srv.SetRPCAuthMethods([]auth.Method{
			// The primary authentication method.
			&openid.GoogleIDTokenAuthMethod{
				AudienceCheck: openid.AudienceMatchesHost,
				SkipNonJWT:    true, // pass OAuth2 access tokens through
			},
			// Backward compatibility for RPC Explorer and old clients.
			&auth.GoogleOAuth2Method{
				Scopes: []string{scopes.Email},
			},
		})

		mailer.RegisterMailerServer(srv, &mailerServer{
			callersGroup: *callersGroup,
			pool:         pool,
			cache:        caching.GlobalCache(srv.Context, "mailer"),
		})

		return nil
	})
}

// waitSMTP tries to ping an SMTP server in a loop.
func waitSMTP(ctx context.Context, addr string) error {
	attempt := 0
	for {
		attempt++
		switch err := pingSMTP(addr); {
		case err == nil:
			logging.Infof(ctx, "The SMTP server %q is ready", addr)
			return nil
		case attempt > 10:
			return err
		default:
			logging.Warningf(ctx, "Waiting for the SMTP server: %s", err)
			if res := clock.Sleep(ctx, 2*time.Second); res.Err != nil {
				return res.Err // the server is shutting down already
			}
		}
	}
}

// pingSMTP returns nil if it manages to connect to the SMTP server.
func pingSMTP(addr string) error {
	client, err := smtp.Dial(addr)
	if err != nil {
		return err
	}
	defer client.Close()
	return client.Noop()
}

type mailerServer struct {
	mailer.UnimplementedMailerServer

	callersGroup string            // a LUCI group with authorized callers
	pool         *email.Pool       // an SMTP connection pool
	cache        caching.BlobCache // to store requestID => messageID mapping

	// Used in tests to mock pool.Send.
	send func(msg *email.Email, timeout time.Duration) error
}

// SendMail implements the corresponding RPC method.
func (s *mailerServer) SendMail(ctx context.Context, req *mailer.SendMailRequest) (*mailer.SendMailResponse, error) {
	caller := auth.CurrentIdentity(ctx)

	switch yes, err := auth.IsMember(ctx, s.callersGroup); {
	case err != nil:
		logging.Errorf(ctx, "IsMember call failed when checking %q: %s", caller, err)
		return nil, status.Errorf(codes.Internal, "failed to authorize the request")
	case !yes:
		logging.Errorf(ctx, "Unauthorized caller: %q", caller)
		return nil, status.Errorf(codes.PermissionDenied, "caller %q is unauthorized", caller)
	}

	logging.Infof(ctx, "Caller: %q", caller)
	logging.Infof(ctx, "Request ID: %q", req.RequestId)

	requestDedupKey := ""
	if req.RequestId != "" {
		requestDedupKey = requestDedupCacheKey(caller, req.RequestId)
	}

	// If the request has RequestID, check if we already handled this request.
	if requestDedupKey != "" {
		switch msgID, err := s.deduplicateRequest(ctx, requestDedupKey); {
		case err != nil:
			logging.Errorf(ctx, "Failed to check for duplicate requests: %s", err)
			return nil, status.Errorf(codes.Internal, "failed to check for duplicate requests: %s", err)
		case msgID != "":
			logging.Infof(ctx, "Deduplicated the request, message ID is %q", msgID)
			return &mailer.SendMailResponse{MessageId: msgID}, nil
		}
	}

	msgID, err := s.enqueueMail(ctx, req)
	if err != nil {
		logging.Errorf(ctx, "Failed to enqueue the mail: %s", err)
		return nil, err
	}
	logging.Infof(ctx, "Message ID: %q", msgID)

	// Best-effort at remembering the RequestID => MessageID mapping for
	// deduplication. If this fails, returning an error here will be more harmful
	// than just ignoring it: the client will retry and a thus send a duplicate
	// email.
	if requestDedupKey != "" {
		if err := s.rememberRequestID(ctx, requestDedupKey, msgID); err != nil {
			logging.Errorf(ctx,
				"Failed to remember RequestID => MessageID association: %q => %q: %s",
				requestDedupKey, msgID, err,
			)
		}
	}

	return &mailer.SendMailResponse{MessageId: msgID}, nil
}

// requestDedupCacheKey returns a cache key of a entry used to dedup requests.
func requestDedupCacheKey(caller identity.Identity, requestID string) string {
	return fmt.Sprintf("reqID:v1:%s:%s", caller, requestID)
}

// deduplicateRequest checks if this request has already been performed.
//
// Returns the resulting message ID if it was or "" otherwise.
func (s *mailerServer) deduplicateRequest(ctx context.Context, cacheKey string) (string, error) {
	if s.cache == nil {
		logging.Warningf(ctx, "The cache is not configured, skipping deduplication check for %q", cacheKey)
		return "", nil
	}
	switch val, err := s.cache.Get(ctx, cacheKey); {
	case err == caching.ErrCacheMiss:
		return "", nil
	case err != nil:
		return "", err
	default:
		return string(val), nil
	}
}

// rememberRequestID stores RequestID => MessageID association.
func (s *mailerServer) rememberRequestID(ctx context.Context, cacheKey, messageID string) error {
	if s.cache == nil {
		return nil
	}
	return s.cache.Set(ctx, cacheKey, []byte(messageID), time.Hour)
}

// enqueueMail sends the mail to the SMTP server.
//
// Assumes all authorization checks have been made already. Returns the
// resulting message ID. Returns gRPC errors.
func (s *mailerServer) enqueueMail(ctx context.Context, req *mailer.SendMailRequest) (string, error) {
	timeout := 10 * time.Second
	if deadline, ok := ctx.Deadline(); ok {
		timeout = deadline.Sub(clock.Now(ctx))
		if timeout <= 0 {
			return "", status.Error(codes.DeadlineExceeded, "deadline exceeded")
		}
	}

	msg := email.NewEmail()
	msg.From = req.Sender
	if req.ReplyTo != "" {
		msg.ReplyTo = []string{req.ReplyTo}
	}
	msg.To = req.To
	msg.Cc = req.Cc
	msg.Bcc = req.Bcc
	msg.Subject = req.Subject
	msg.Text = []byte(req.TextBody)
	msg.HTML = []byte(req.HtmlBody)
	if req.InReplyTo != "" {
		msg.Headers.Set("In-Reply-To", req.InReplyTo)
	}
	if len(req.References) > 0 {
		msg.Headers.Set("References", strings.Join(req.References, "\n"))
	}

	// Generate a message ID and convert it into an RFC 2822 compliant form.
	//
	// Note that the SMTP server assigns its own ID to the message as well when
	// acknowledging it with "250 Ok" response, but unfortunately `net/smtp`
	// package (and consequently all Go packages based on it), doesn't expose this
	// ID in its API (it totally ignores the text status attached to "250 Ok"
	// response).
	msgID := shared_mailer.GenerateMessageID(ctx)
	msg.Headers.Set("Message-Id", msgID)

	send := s.send
	if send == nil {
		send = s.pool.Send
	}

	switch err := send(msg, timeout); {
	case err == nil:
		return msgID, nil
	case err == email.ErrTimeout:
		return "", status.Error(codes.DeadlineExceeded, "timeout when waiting for an SMTP connection")
	case err == email.ErrClosed:
		return "", status.Error(codes.Internal, "the mailer server is shutting down")
	case isFatalSMTP(err):
		return "", status.Errorf(codes.InvalidArgument, "failed to send the email: %s", err)
	default:
		return "", status.Errorf(codes.Internal, "transient SMTP error: %s", err)
	}
}

// isFatalSMTP recognizes fatal SMTP errors.
//
// We assume only SMTP replies with "Permanent Negative Completion reply"
// status codes are fatal and everything else (like network errors, transient
// SMTP replies, etc.) are transient.
//
// See https://datatracker.ietf.org/doc/html/rfc5321#section-4.2.1 which
// defines 5xx as "Permanent Negative Completion reply".
func isFatalSMTP(err error) bool {
	if tpe, ok := err.(*textproto.Error); ok {
		return tpe.Code >= 500
	}
	return false
}
