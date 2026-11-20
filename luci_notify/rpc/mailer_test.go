// Copyright 2026 The LUCI Authors.
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
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/logging/memlogger"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/grpc/grpcutil/testing/grpccode"
	"go.chromium.org/luci/grpc/prpc"
	mailer_pb "go.chromium.org/luci/mailer/api/mailer"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
	"go.chromium.org/luci/server/caching"
	"go.chromium.org/luci/server/mailer"
	"go.chromium.org/luci/server/router"
)

func TestSendMail(t *testing.T) {
	t.Parallel()

	ftt.Run("SendMail", t, func(t *ftt.Test) {
		ctx := context.Background()
		ctx = memory.Use(ctx)
		ctx = caching.WithEmptyProcessCache(ctx)
		ctx = memlogger.Use(ctx)

		var sent []*mailer.Mail
		ctx = mailer.Use(ctx, func(ctx context.Context, msg *mailer.Mail) (string, error) {
			sent = append(sent, msg)
			return "msg-123", nil
		})

		srv := NewMailerServer()
		baseReq := func() *mailer_pb.SendMailRequest {
			return &mailer_pb.SendMailRequest{
				To:       []string{"alice@google.com"},
				Subject:  "Build failed",
				TextBody: "Details",
			}
		}

		t.Run("rejects anonymous caller", func(t *ftt.Test) {
			ctx := fakeAuth().anonymous().setInContext(ctx)
			_, err := srv.SendMail(ctx, baseReq())
			assert.Loosely(t, err, grpccode.ShouldBe(codes.PermissionDenied))
			assert.Loosely(t, sent, should.BeEmpty)
		})

		t.Run("rejects unauthorized caller", func(t *ftt.Test) {
			ctx := fakeAuth().withWriteAccess().setInContext(ctx)
			_, err := srv.SendMail(ctx, baseReq())
			assert.Loosely(t, err, grpccode.ShouldBe(codes.PermissionDenied))
			assert.Loosely(t, sent, should.BeEmpty)
		})

		ctx = auth.WithState(ctx, &authtest.FakeState{
			Identity:       identity.Identity("user:caller@example.com"),
			IdentityGroups: []string{luciNotifySendEmailsGroup},
		})

		assertInvalidArgument := func(t *ftt.Test, err error, substr string) {
			t.Helper()
			assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
			assert.Loosely(t, err, should.ErrLike(substr))
		}

		t.Run("rejects non-empty Sender", func(t *ftt.Test) {
			for _, sender := range []string{"spoof@chromium.org", "noreply@luci-notify.appspotmail.com"} {
				req := baseReq()
				req.Sender = sender
				_, err := srv.SendMail(ctx, req)
				assertInvalidArgument(t, err, "sender must not be specified")
			}
			assert.Loosely(t, sent, should.BeEmpty)
		})

		t.Run("rejects non-empty ReplyTo", func(t *ftt.Test) {
			for _, replyTo := range []string{"attacker@evil.example", "reply@chromium.org"} {
				req := baseReq()
				req.ReplyTo = replyTo
				_, err := srv.SendMail(ctx, req)
				assertInvalidArgument(t, err, "reply_to must not be specified")
			}
			assert.Loosely(t, sent, should.BeEmpty)
		})

		t.Run("rejects empty recipients", func(t *ftt.Test) {
			req := baseReq()
			req.To = nil
			_, err := srv.SendMail(ctx, req)
			assertInvalidArgument(t, err, "at least one of To, Cc, or Bcc must be non-empty")
			assert.Loosely(t, sent, should.BeEmpty)
		})

		t.Run("enforces recipient count cap across To, Cc, and Bcc", func(t *ftt.Test) {
			makeAddrs := func(n int) []string {
				addrs := make([]string, n)
				for i := range n {
					addrs[i] = fmt.Sprintf("user%d@google.com", i)
				}
				return addrs
			}

			tooMany := baseReq()
			tooMany.To = makeAddrs(50)
			tooMany.Cc = makeAddrs(30)
			tooMany.Bcc = makeAddrs(21)
			_, err := srv.SendMail(ctx, tooMany)
			assertInvalidArgument(t, err, "total recipients (101) must not exceed 100")
			assert.Loosely(t, sent, should.BeEmpty)

			atCap := baseReq()
			atCap.To = makeAddrs(50)
			atCap.Cc = makeAddrs(30)
			atCap.Bcc = makeAddrs(20)
			_, err = srv.SendMail(ctx, atCap)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, sent, should.HaveLength(1))
		})

		t.Run("rejects empty body", func(t *ftt.Test) {
			req := baseReq()
			req.TextBody = ""
			_, err := srv.SendMail(ctx, req)
			assertInvalidArgument(t, err, "at least one of TextBody or HtmlBody must be non-empty")
			assert.Loosely(t, sent, should.BeEmpty)
		})

		t.Run("rejects CRLF newlines in header fields", func(t *ftt.Test) {
			cases := []struct {
				name   string
				mutate func(req *mailer_pb.SendMailRequest, nl string)
				substr string
			}{
				{
					name:   "Subject",
					mutate: func(r *mailer_pb.SendMailRequest, nl string) { r.Subject = "Alert" + nl + "Bcc: x@evil.example" },
					substr: "subject must not contain newlines",
				},
				{
					name:   "InReplyTo",
					mutate: func(r *mailer_pb.SendMailRequest, nl string) { r.InReplyTo = "<id-1>" + nl + "X: y" },
					substr: "in_reply_to must not contain newlines",
				},
				{
					name:   "References",
					mutate: func(r *mailer_pb.SendMailRequest, nl string) { r.References = []string{"<id-0>" + nl + "X: y"} },
					substr: "references must not contain newlines",
				},
				{
					name:   "To",
					mutate: func(r *mailer_pb.SendMailRequest, nl string) { r.To = []string{"alice@google.com" + nl} },
					substr: "To recipient must not contain newlines",
				},
				{
					name:   "Cc",
					mutate: func(r *mailer_pb.SendMailRequest, nl string) { r.Cc = []string{"alice@google.com" + nl} },
					substr: "Cc recipient must not contain newlines",
				},
				{
					name:   "Bcc",
					mutate: func(r *mailer_pb.SendMailRequest, nl string) { r.Bcc = []string{"alice@google.com" + nl} },
					substr: "Bcc recipient must not contain newlines",
				},
			}
			for _, tc := range cases {
				for _, nl := range []string{"\r", "\n", "\r\n"} {
					req := baseReq()
					tc.mutate(req, nl)
					_, err := srv.SendMail(ctx, req)
					assertInvalidArgument(t, err, tc.substr)
				}
			}
			assert.Loosely(t, sent, should.BeEmpty)
		})

		t.Run("sends mail with empty Sender and ReplyTo and no warnings for allowed addresses", func(t *ftt.Test) {
			req := &mailer_pb.SendMailRequest{
				To:         []string{"alice@google.com", "alice@GOOGLE.COM"},
				Cc:         []string{"sheriff@chromium.org"},
				Bcc:        []string{"oncall@rotations.google.com"},
				Subject:    "Alert",
				TextBody:   "Plain text",
				HtmlBody:   "<p>HTML</p>",
				InReplyTo:  "<id-1>",
				References: []string{"<id-0>"},
			}
			resp, err := srv.SendMail(ctx, req)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, resp.MessageId, should.Equal("msg-123"))
			assert.Loosely(t, sent, should.HaveLength(1))
			assert.Loosely(t, sent[0], should.Resemble(&mailer.Mail{
				Sender:     "",
				ReplyTo:    "",
				To:         []string{"alice@google.com", "alice@GOOGLE.COM"},
				Cc:         []string{"sheriff@chromium.org"},
				Bcc:        []string{"oncall@rotations.google.com"},
				Subject:    "Alert",
				TextBody:   "Plain text",
				HTMLBody:   "<p>HTML</p>",
				InReplyTo:  "<id-1>",
				References: []string{"<id-0>"},
			}))

			logs := logging.Get(ctx).(*memlogger.MemLogger)
			for _, msg := range logs.Messages() {
				assert.Loosely(t, msg.Level, should.NotEqual(logging.Warning))
			}
			assert.Loosely(t, logs.Has(logging.Info, "Mailer.SendMail sending email", map[string]any{
				"caller":  "user:caller@example.com",
				"to":      []string{"alice@google.com", "alice@GOOGLE.COM"},
				"cc":      []string{"sheriff@chromium.org"},
				"bcc":     []string{"oncall@rotations.google.com"},
				"subject": "Alert",
			}), should.BeTrue)
		})

		t.Run("warns and rejects when all recipients are disallowed", func(t *ftt.Test) {
			req := &mailer_pb.SendMailRequest{
				To:       []string{"external@example.com"},
				Cc:       []string{"Bob <bob@google.com>"},
				Bcc:      []string{"carol@google.com (comment)"},
				Subject:  "Notice",
				HtmlBody: "<p>Notice</p>",
			}
			_, err := srv.SendMail(ctx, req)
			assertInvalidArgument(t, err, "no allowed recipients in To, Cc, or Bcc")
			assert.Loosely(t, sent, should.BeEmpty)

			logs := logging.Get(ctx).(*memlogger.MemLogger)
			var warnings []string
			for _, msg := range logs.Messages() {
				if msg.Level == logging.Warning {
					warnings = append(warnings, msg.Msg)
				}
			}
			assert.Loosely(t, warnings, should.HaveLength(3))
			assert.Loosely(t, warnings[0], should.ContainSubstring("external@example.com"))
			assert.Loosely(t, warnings[0], should.ContainSubstring("user:caller@example.com"))
			assert.Loosely(t, warnings[1], should.ContainSubstring("Bob <bob@google.com>"))
			assert.Loosely(t, warnings[2], should.ContainSubstring("carol@google.com (comment)"))
		})

		t.Run("filters disallowed recipients while keeping allowed ones", func(t *ftt.Test) {
			req := &mailer_pb.SendMailRequest{
				To:       []string{"alice@google.com", "external@example.com"},
				Cc:       []string{"sheriff@chromium.org", "Bob <bob@google.com>"},
				Bcc:      []string{"oncall@rotations.google.com", "carol@other.org"},
				Subject:  "Notice",
				HtmlBody: "<p>Notice</p>",
			}
			resp, err := srv.SendMail(ctx, req)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, resp.MessageId, should.Equal("msg-123"))
			assert.Loosely(t, sent, should.HaveLength(1))
			assert.Loosely(t, sent[0].Sender, should.BeEmpty)
			assert.Loosely(t, sent[0].ReplyTo, should.BeEmpty)
			assert.Loosely(t, sent[0].To, should.Resemble([]string{"alice@google.com"}))
			assert.Loosely(t, sent[0].Cc, should.Resemble([]string{"sheriff@chromium.org"}))
			assert.Loosely(t, sent[0].Bcc, should.Resemble([]string{"oncall@rotations.google.com"}))

			logs := logging.Get(ctx).(*memlogger.MemLogger)
			var warnings []string
			for _, msg := range logs.Messages() {
				if msg.Level == logging.Warning {
					warnings = append(warnings, msg.Msg)
				}
			}
			assert.Loosely(t, warnings, should.HaveLength(3))
			assert.Loosely(t, warnings[0], should.ContainSubstring("external@example.com"))
			assert.Loosely(t, warnings[0], should.ContainSubstring("user:caller@example.com"))
			assert.Loosely(t, warnings[1], should.ContainSubstring("Bob <bob@google.com>"))
			assert.Loosely(t, warnings[2], should.ContainSubstring("carol@other.org"))
			assert.Loosely(t, logs.Has(logging.Info, "Mailer.SendMail sending email", map[string]any{
				"caller":  "user:caller@example.com",
				"to":      []string{"alice@google.com"},
				"cc":      []string{"sheriff@chromium.org"},
				"bcc":     []string{"oncall@rotations.google.com"},
				"subject": "Notice",
			}), should.BeTrue)
		})

		t.Run("maps backend send failure to Internal without leaking error text", func(t *ftt.Test) {
			ctx := mailer.Use(ctx, func(ctx context.Context, msg *mailer.Mail) (string, error) {
				return "", errors.New("gae mail failure")
			})
			_, err := srv.SendMail(ctx, baseReq())
			assert.Loosely(t, err, grpccode.ShouldBe(codes.Internal))
			assert.Loosely(t, status.Convert(err).Message(), should.NotContainSubstring("gae mail failure"))
			assert.Loosely(t, err.Error(), should.NotContainSubstring("gae mail failure"))
		})
	})
}

func TestMailerPRPC(t *testing.T) {
	t.Parallel()

	ftt.Run("Mailer pRPC HTTP status codes", t, func(t *ftt.Test) {
		var sent []*mailer.Mail
		callPRPC := func(authBuilder *fakeAuthBuilder, jsonBody string) *httptest.ResponseRecorder {
			prpcSrv := &prpc.Server{}
			mailer_pb.RegisterMailerServer(prpcSrv, NewMailerServer())

			r := router.New()
			prpcSrv.InstallHandlers(r, router.NewMiddlewareChain(func(c *router.Context, next router.Handler) {
				ctx := c.Request.Context()
				ctx = memory.Use(ctx)
				ctx = caching.WithEmptyProcessCache(ctx)
				ctx = memlogger.Use(ctx)
				ctx = authBuilder.setInContext(ctx)
				ctx = mailer.Use(ctx, func(ctx context.Context, msg *mailer.Mail) (string, error) {
					sent = append(sent, msg)
					return "msg-prpc", nil
				})
				c.Request = c.Request.WithContext(ctx)
				next(c)
			}))

			req := httptest.NewRequest(http.MethodPost, "/prpc/luci.mailer.v1.Mailer/SendMail", bytes.NewBufferString(jsonBody))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Accept", "application/json")
			rw := httptest.NewRecorder()
			r.ServeHTTP(rw, req)
			return rw
		}

		authorized := &fakeAuthBuilder{
			state: &authtest.FakeState{
				Identity:       identity.Identity("user:caller@example.com"),
				IdentityGroups: []string{luciNotifySendEmailsGroup},
			},
		}

		t.Run("unauthorized caller returns HTTP 403 PermissionDenied", func(t *ftt.Test) {
			rw := callPRPC(fakeAuth().withWriteAccess(), `{"to":["alice@google.com"],"textBody":"hi"}`)
			assert.Loosely(t, rw.Code, should.Equal(http.StatusForbidden))
			assert.Loosely(t, rw.Header().Get(prpc.HeaderGRPCCode), should.Equal(strconv.Itoa(int(codes.PermissionDenied))))
			assert.Loosely(t, sent, should.BeEmpty)
		})

		t.Run("invalid inputs return HTTP 400 InvalidArgument instead of 500", func(t *ftt.Test) {
			invalidBodies := []string{
				`{"sender":"spoof@google.com","to":["alice@google.com"],"textBody":"hi"}`,
				`{"replyTo":"attacker@evil.example","to":["alice@google.com"],"textBody":"hi"}`,
				`{"to":["alice@google.com"],"subject":"bad\r\nheader","textBody":"hi"}`,
				`{"to":["alice@google.com\n"],"subject":"ok","textBody":"hi"}`,
				`{"to":["victim@anydomain.example"],"subject":"Notice","textBody":"hi"}`,
				`{"to":[],"textBody":"hi"}`,
			}
			for _, body := range invalidBodies {
				rw := callPRPC(authorized, body)
				assert.Loosely(t, rw.Code, should.Equal(http.StatusBadRequest), truth.Explain("body %s returned %s", body, rw.Body.String()))
				assert.Loosely(t, rw.Header().Get(prpc.HeaderGRPCCode), should.Equal(strconv.Itoa(int(codes.InvalidArgument))), truth.Explain("body %s", body))
			}
			assert.Loosely(t, sent, should.BeEmpty)
		})

		t.Run("valid request returns HTTP 200 OK", func(t *ftt.Test) {
			rw := callPRPC(authorized, `{"to":["alice@google.com"],"subject":"Notice","textBody":"Details"}`)
			assert.Loosely(t, rw.Code, should.Equal(http.StatusOK))
			assert.Loosely(t, rw.Header().Get(prpc.HeaderGRPCCode), should.Equal(strconv.Itoa(int(codes.OK))))
			assert.Loosely(t, sent, should.HaveLength(1))
			assert.Loosely(t, sent[0].Sender, should.BeEmpty)
			assert.Loosely(t, sent[0].ReplyTo, should.BeEmpty)
		})
	})
}
