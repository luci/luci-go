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
	"strings"

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
	// InternalRoutingPrefix is a relative URI prefix that will be used for all
	// exposed HTTP routes.
	//
	// This is a prefix in the server's router. Default is "/internal/tasks/".
	InternalRoutingPrefix string

	// ExternalRoutingPrefix is an absolute URL prefix that matches routes
	// exposed via InternalRoutingPrefix.
	//
	// See Dispatcher for more info.
	//
	// Ignored on GAE. Required on non-GAE.
	ExternalRoutingPrefix string

	// PushAs is a service account email to be used for generating OIDC tokens.
	//
	// The service account must be within the same project. The server account
	// must have "iam.serviceAccounts.actAs" permission for `PushAs` account.
	//
	// By default set to the server's own account. Ignored on GAE.
	PushAs string
}

// Register registers the command line flags.
func (o *ModuleOptions) Register(f *flag.FlagSet) {
	f.StringVar(&o.InternalRoutingPrefix, "tq-internal-routing-prefix", "/internal/tasks/",
		`Relative URI prefix that will be used for all exposed HTTP routes`)

	f.StringVar(&o.ExternalRoutingPrefix, "tq-external-routing-prefix", "",
		`Absolute URL prefix that matches routes exposed via -tq-internal-routing-prefix. `+
			`This should usually be "https://<domain>/<internal-routing-prefix>", but may be `+
			`different if your load balancing layer does URL rewrites. Ignored on GAE, required elsewhere.`)

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
	if opts.CloudProject == "" {
		return nil, errors.Reason("-cloud-project is required").Err()
	}
	if opts.CloudRegion == "" {
		return nil, errors.Reason("-cloud-region is required").Err()
	}

	Default.GAE = opts.GAE
	Default.CloudProject = opts.CloudProject
	Default.CloudRegion = opts.CloudRegion

	Default.InternalRoutingPrefix = m.opts.InternalRoutingPrefix
	if !strings.HasSuffix(Default.InternalRoutingPrefix, "/") {
		Default.InternalRoutingPrefix += "/"
	}

	if !opts.GAE {
		if m.opts.PushAs != "" {
			Default.PushAs = m.opts.PushAs
		} else {
			info, err := auth.GetSigner(ctx).ServiceInfo(ctx)
			if err != nil {
				return nil, errors.Annotate(err, "failed to get own service account email").Err()
			}
			Default.PushAs = info.ServiceAccountName
		}

		if m.opts.ExternalRoutingPrefix == "" {
			return nil, errors.Reason("-tq-external-routing-prefix is required, it should " +
				"be an URL prefix over which TQ routes are exposed, usually `https://<domain>/internal/tasks`").Err()
		}
		Default.ExternalRoutingPrefix = m.opts.ExternalRoutingPrefix
		if !strings.HasSuffix(Default.ExternalRoutingPrefix, "/") {
			Default.ExternalRoutingPrefix += "/"
		}
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
