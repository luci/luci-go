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
	"flag"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/grpc/grpcutil"
	"go.chromium.org/luci/grpc/prpc"
	"go.chromium.org/luci/mailer/api/mailer"

	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/internal/gae"
	gaebasepb "go.chromium.org/luci/server/internal/gae/base"
	gaemailpb "go.chromium.org/luci/server/internal/gae/mail"
	"go.chromium.org/luci/server/module"
)

// ModuleName can be used to refer to this module when declaring dependencies.
var ModuleName = module.RegisterName("go.chromium.org/luci/server/mailer")

// ModuleOptions contain configuration of the mailer server module.
//
// It will be used to initialize the mailer in the context.
type ModuleOptions struct {
	// MailerService defines what mailing backend to use.
	//
	// Supported values are:
	//   * "https://<host>"" to use a luci.mailer.v1.Mailer pRPC service.
	//   * "gae" to use GAE bundled Mail service (works only on GAE, see below).
	//
	// Also "http://<host>" can be used locally to connect to a local pRPC mailer
	// service without TLS. This is useful for local integration tests.
	//
	// Using "gae" backend requires running on GAE and having
	// "app_engine_apis: true" in the module YAML.
	// See https://cloud.google.com/appengine/docs/standard/go/services/access.
	//
	// On GAE defaults to "gae", elsewhere defaults to no backend at all which
	// results in emails being logged in local logs only and not actually sent
	// anywhere.
	MailerService string

	// DefaultSender is a value to use in "From" email header field by default.
	//
	// Used only if `Sender` field of Mail struct is not populated.
	//
	// On GAE defaults to "<appid> noreply@<appid>.appspotmail.com".
	//
	// When using a pRPC backend, defaults to an empty string, which indicates
	// that the pRPC backend should make the decision itself.
	DefaultSender string
}

// Register registers the command line flags.
func (o *ModuleOptions) Register(f *flag.FlagSet) {
	f.StringVar(&o.MailerService, "mailer-service", o.MailerService, `What mailing backend to use.`)
	f.StringVar(&o.DefaultSender, "mailer-default-sender", o.DefaultSender, `A value to use in "From" email header field by default.`)
}

// NewModule returns a server module that initializes the mailer in the context.
func NewModule(opts *ModuleOptions) module.Module {
	if opts == nil {
		opts = &ModuleOptions{}
	}
	return &mailerModule{opts: opts}
}

// NewModuleFromFlags is a variant of NewModule that initializes options through
// command line flags.
//
// Calling this function registers flags in flag.CommandLine. They are usually
// parsed in server.Main(...).
func NewModuleFromFlags() module.Module {
	opts := &ModuleOptions{}
	opts.Register(flag.CommandLine)
	return NewModule(opts)
}

// mailerModule implements module.Module.
type mailerModule struct {
	opts *ModuleOptions
}

// Name is part of module.Module interface.
func (*mailerModule) Name() module.Name {
	return ModuleName
}

// Dependencies is part of module.Module interface.
func (*mailerModule) Dependencies() []module.Dependency {
	return nil
}

// Initialize is part of module.Module interface.
func (m *mailerModule) Initialize(ctx context.Context, host module.Host, opts module.HostOptions) (context.Context, error) {
	service := m.opts.MailerService

	if service == "" {
		if opts.Serverless == module.GAE {
			service = "gae"
		} else {
			logging.Warningf(ctx, "Mailer service is not configured, emails will be dropped")
			return Use(ctx, func(ctx context.Context, msg *Mail) error {
				logging.Errorf(ctx, "No mailer configured: dropping message to %q with subject %q", msg.To, msg.Subject)
				return nil
			}), nil
		}
	}

	var mailer Mailer
	var err error

	switch {
	case strings.HasPrefix(service, "https://"):
		mailer, err = m.initRPCMailer(ctx, strings.TrimPrefix(service, "https://"), false)
	case strings.HasPrefix(service, "http://"):
		mailer, err = m.initRPCMailer(ctx, strings.TrimPrefix(service, "http://"), true)
	case service == "gae":
		if opts.Serverless != module.GAE {
			return nil, errors.New(`"-mailer-service gae" can only be used on GAE`)
		}
		if m.opts.DefaultSender == "" {
			m.opts.DefaultSender = fmt.Sprintf("%s <noreply@%s.appspotmail.com>", opts.CloudProject, opts.CloudProject)
		}
		mailer, err = m.initGAEMailer(ctx)
	default:
		return nil, errors.Fmt("unrecognized -mailer-service %q", service)
	}

	if err != nil {
		return nil, err
	}
	return Use(ctx, mailer), nil
}

func (m *mailerModule) sender(msg *Mail) string {
	if msg.Sender != "" {
		return msg.Sender
	}
	return m.opts.DefaultSender
}

func (m *mailerModule) initRPCMailer(ctx context.Context, host string, insecure bool) (Mailer, error) {
	tr, err := auth.GetRPCTransport(ctx, auth.AsSelf, auth.WithIDToken())
	if err != nil {
		return nil, errors.Fmt("failed to get a RPC transport: %w", err)
	}

	mailerClient := mailer.NewMailerClient(&prpc.Client{
		C:    &http.Client{Transport: tr},
		Host: host,
		Options: &prpc.Options{
			Insecure:      insecure,
			PerRPCTimeout: 10 * time.Second,
			Retry: func() retry.Iterator {
				return &retry.ExponentialBackoff{
					Limited: retry.Limited{
						Delay:    50 * time.Millisecond,
						Retries:  -1,
						MaxTotal: 20 * time.Second,
					},
				}
			},
		},
	})

	return func(ctx context.Context, msg *Mail) error {
		requestID, err := uuid.NewRandom()
		if err != nil {
			return transient.Tag.Apply(errors.Fmt("failed to generate request ID: %w", err))
		}
		resp, err := mailerClient.SendMail(ctx, &mailer.SendMailRequest{
			RequestId: requestID.String(),
			Sender:    m.sender(msg),
			ReplyTo:   msg.ReplyTo,
			To:        msg.To,
			Cc:        msg.Cc,
			Bcc:       msg.Bcc,
			Subject:   msg.Subject,
			TextBody:  msg.TextBody,
			HtmlBody:  msg.HTMLBody,
		})
		if err != nil {
			return grpcutil.WrapIfTransient(err)
		}
		logging.Infof(ctx, "Email enqueued as %q", resp.MessageId)
		return nil
	}, nil
}

func (m *mailerModule) initGAEMailer(ctx context.Context) (Mailer, error) {
	return func(ctx context.Context, msg *Mail) error {
		req := &gaemailpb.MailMessage{
			Sender:  proto.String(m.sender(msg)),
			To:      msg.To,
			Cc:      msg.Cc,
			Bcc:     msg.Bcc,
			Subject: &msg.Subject,
		}
		if msg.ReplyTo != "" {
			req.ReplyTo = &msg.ReplyTo
		}
		if msg.TextBody != "" {
			req.TextBody = &msg.TextBody
		}
		if msg.HTMLBody != "" {
			req.HtmlBody = &msg.HTMLBody
		}

		res := &gaebasepb.VoidProto{}
		if err := gae.Call(ctx, "mail", "Send", req, res); err != nil {
			// TODO(vadimsh): In theory we can extract internal GAE Mail error codes
			// here and decide if an error is transient or not. For now assume they
			// all are.
			return transient.Tag.Apply(err)
		}
		logging.Infof(ctx, "Email enqueued")
		return nil
	}, nil
}
