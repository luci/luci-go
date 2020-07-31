// Copyright 2020 The LUCI Authors.
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

package tq

import (
	"context"
	"flag"

	cloudtasks "cloud.google.com/go/cloudtasks/apiv2"
	"google.golang.org/api/option"
	"google.golang.org/grpc"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/module"
)

// ModuleOptions contain configuration of the TQ server module.
//
// It will be used to initialize Default dispatcher.
type ModuleOptions struct {
	// DefaultTargetHost is a hostname to dispatch Cloud Tasks to by default.
	//
	// Individual task classes may override it with their own specific host.
	//
	// On GAE defaults to the GAE application itself. Elsewhere has no default:
	// if the dispatcher can't figure out where to send the task, the task
	// submission fails.
	DefaultTargetHost string

	// PushAs is a service account email to be used for generating OIDC tokens.
	//
	// The service account must be within the same project. The server account
	// must have "iam.serviceAccounts.actAs" permission for `PushAs` account.
	//
	// By default set to the server's own account.
	PushAs string
}

// Register registers the command line flags.
func (o *ModuleOptions) Register(f *flag.FlagSet) {
	f.StringVar(&o.DefaultTargetHost, "tq-default-target-host", "",
		`Hostname to dispatch Cloud Tasks to by default.`)

	f.StringVar(&o.PushAs, "tq-push-as", "",
		`Service account email to be used for generating OIDC tokens. `+
			`Default is server's own account.`)
}

// NewModule returns a server module that sets up a TQ dispatcher.
func NewModule(opts *ModuleOptions) module.Module {
	if opts == nil {
		opts = &ModuleOptions{}
	}
	return &tqModule{opts: opts}
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

// tqModule implements module.Module.
type tqModule struct {
	opts *ModuleOptions
}

// Name is part of module.Module interface.
func (*tqModule) Name() string {
	return "go.chromium.org/luci/server/tq"
}

// Initialize is part of module.Module interface.
func (m *tqModule) Initialize(ctx context.Context, host module.Host, opts module.HostOptions) (context.Context, error) {
	Default.GAE = opts.GAE
	Default.CloudProject = opts.CloudProject
	Default.CloudRegion = opts.CloudRegion
	Default.DefaultTargetHost = m.opts.DefaultTargetHost

	if m.opts.PushAs != "" {
		Default.PushAs = m.opts.PushAs
	} else {
		info, err := auth.GetSigner(ctx).ServiceInfo(ctx)
		if err != nil {
			return nil, errors.Annotate(err, "failed to get own service account email").Err()
		}
		Default.PushAs = info.ServiceAccountName
	}

	creds, err := auth.GetPerRPCCredentials(ctx, auth.AsSelf, auth.WithScopes(auth.CloudOAuthScopes...))
	if err != nil {
		return nil, errors.Annotate(err, "failed to get PerRPCCredentials").Err()
	}
	client, err := cloudtasks.NewClient(ctx, option.WithGRPCDialOption(grpc.WithPerRPCCredentials(creds)))
	if err != nil {
		return nil, errors.Annotate(err, "failed to initialize Cloud Tasks client").Err()
	}
	host.RegisterCleanup(func(ctx context.Context) { client.Close() })

	Default.Submitter = &CloudTaskSubmitter{Client: client}
	Default.InstallRoutes(host.Routes())
	return ctx, nil
}
