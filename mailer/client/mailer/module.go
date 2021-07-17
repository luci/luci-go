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
	"net/http"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/grpc/prpc"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/module"

	"go.chromium.org/luci/mailer/api/mailer"
)

// ModuleName can be used to refer to this module when declaring dependencies.
var ModuleName = module.RegisterName("go.chromium.org/luci/mailer/client/mailer")

// ModuleOptions contain configuration of the bqlog server module.
//
// It will be used to initialize Default bundler.
type ModuleOptions struct {
	// MailerService is a domain name of a luci.mailer.v1.Mailer pRPC service.
	//
	// If unset, no emails will be sent (just logged in the process log).
	MailerService string

	// MailerInsecure, if set, instructs to use http:// instead of https://.
	//
	// Useful for local integration tests.
	MailerInsecure bool
}

// Register registers the command line flags.
func (o *ModuleOptions) Register(f *flag.FlagSet) {
	f.StringVar(&o.MailerService, "mailer-service", o.MailerService,
		`A domain name a luci.mailer.v1.Mailer pRPC service to use.`)
	f.BoolVar(&o.MailerInsecure, "mailer-insecure", o.MailerInsecure,
		`Use HTTP instead of HTTPS when connecting to the mailer service.`)
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
	if m.opts.MailerService == "" {
		logging.Warningf(ctx, "Mailer service is not configured, emails will be dropped")
		return Use(ctx, func(ctx context.Context, msg *Mail) error {
			logging.Errorf(ctx, "No mailer configured: dropping message to %q with subject %q", msg.To, msg.Subject)
			return nil
		}), nil
	}

	tr, err := auth.GetRPCTransport(ctx, auth.AsSelf, auth.WithIDToken())
	if err != nil {
		return nil, errors.Annotate(err, "failed to get a RPC transport").Err()
	}

	prpcClient := &prpc.Client{
		C:       &http.Client{Transport: tr},
		Host:    m.opts.MailerService,
		Options: prpc.DefaultOptions(),
	}
	if m.opts.MailerInsecure {
		prpcClient.Options.Insecure = true
	}
	mailerClient := mailer.NewMailerClient(prpcClient)

	return Use(ctx, func(ctx context.Context, msg *Mail) error {
		return sendMail(ctx, mailerClient, msg)
	}), nil
}

// sendMail sends the message through the mailer RPC client.
func sendMail(ctx context.Context, cl mailer.MailerClient, msg *Mail) error {
	panic("not implemented yet")
}
