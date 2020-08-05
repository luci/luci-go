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
	luciflag "go.chromium.org/luci/common/flag"

	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/module"
	"go.chromium.org/luci/server/tq/tqtesting"
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

	// AuthorizedPushers is a list of service account emails to accept pushes from
	// in addition to PushAs.
	//
	// See Dispatcher for more info.
	//
	// Optional.
	AuthorizedPushers []string

	// ServingPrefix is an URL path prefix to serve registered task handlers from.
	//
	// POSTs to an URL under this prefix (regardless which one) will be treated
	// as Cloud Tasks pushes.
	//
	// Default is "/internal/tasks". If set to literal "-", no routes will be
	// registered at all.
	ServingPrefix string
}

// Register registers the command line flags.
func (o *ModuleOptions) Register(f *flag.FlagSet) {
	f.StringVar(&o.DefaultTargetHost, "tq-default-target-host", "",
		`Hostname to dispatch Cloud Tasks to by default.`)

	f.StringVar(&o.PushAs, "tq-push-as", "",
		`Service account email to be used for generating OIDC tokens. `+
			`Default is server's own account.`)

	f.Var(luciflag.StringSlice(&o.AuthorizedPushers), "tq-authorized-pusher",
		`Service account email to accept pushes from (in addition to -tq-push-as). May be repeated.`)

	f.StringVar(&o.ServingPrefix, "tq-serving-prefix", "/internal/tasks",
		`URL prefix to serve registered task handlers from. Set to '-' to disable serving.`)
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
	Default.AuthorizedPushers = m.opts.AuthorizedPushers

	if m.opts.PushAs != "" {
		Default.PushAs = m.opts.PushAs
	} else {
		info, err := auth.GetSigner(ctx).ServiceInfo(ctx)
		if err != nil {
			return nil, errors.Annotate(err, "failed to get own service account email").Err()
		}
		Default.PushAs = info.ServiceAccountName
	}

	if opts.Prod {
		// When running for real use real Cloud Tasks service.
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
	} else {
		// When running locally use a simple in-memory scheduler.
		router := host.Routes()
		scheduler := &tqtesting.Scheduler{}
		host.RunInBackground("luci.tq", func(ctx context.Context) {
			scheduler.Run(ctx, &tqtesting.LoopbackHTTPExecutor{
				Handler: router,
			})
		})
		Default.NoAuth = true
		Default.Submitter = scheduler
		if Default.CloudProject == "" {
			Default.CloudProject = "tq-project"
		}
		if Default.CloudRegion == "" {
			Default.CloudRegion = "tq-region"
		}
		if Default.DefaultTargetHost == "" {
			Default.DefaultTargetHost = "127.0.0.1" // not actually used
		}
	}

	if m.opts.ServingPrefix != "-" {
		Default.InstallRoutes(host.Routes(), m.opts.ServingPrefix)
	}

	return ctx, nil
}
