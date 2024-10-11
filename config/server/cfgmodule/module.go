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

package cfgmodule

import (
	"context"
	"flag"
	"fmt"

	"google.golang.org/grpc/credentials"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/proto/config"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/signing"
	"go.chromium.org/luci/server/module"

	"go.chromium.org/luci/config/cfgclient"
	"go.chromium.org/luci/config/validation"
	"go.chromium.org/luci/config/vars"
)

// ModuleName can be used to refer to this module when declaring dependencies.
var ModuleName = module.RegisterName("go.chromium.org/luci/config/server/cfgmodule")

// ModuleOptions contain configuration of the LUCI Config server module.
type ModuleOptions struct {
	// ServiceHost is a hostname of a LUCI Config service to use.
	//
	// If given, indicates configs should be fetched from the LUCI Config service.
	// Not compatible with LocalDir.
	ServiceHost string

	// LocalDir is a file system directory to fetch configs from instead of
	// a LUCI Config service.
	//
	// See https://godoc.org/go.chromium.org/luci/config/impl/filesystem for the
	// expected layout of this directory.
	//
	// Useful when running locally in development mode. Not compatible with
	// ServiceHost.
	LocalDir string

	// Vars is a var set to use to render config set names.
	//
	// If nil, the module uses global &vars.Vars. This is usually what you want.
	//
	// During the initialization the module registers the following vars:
	//   ${appid}: value of -cloud-project server flag.
	//   ${config_service_appid}: app ID of the LUCI Config service.
	Vars *vars.VarSet

	// Rules is a rule set to use for the config validation.
	//
	// If nil, the module uses global &validation.Rules. This is usually what
	// you want.
	Rules *validation.RuleSet
}

// Register registers the command line flags.
func (o *ModuleOptions) Register(f *flag.FlagSet) {
	f.StringVar(
		&o.ServiceHost,
		"config-service-host",
		o.ServiceHost,
		`A hostname of a LUCI Config service to use (not compatible with -config-local-dir)`,
	)
	f.StringVar(
		&o.LocalDir,
		"config-local-dir",
		o.LocalDir,
		`A file system directory to fetch configs from (not compatible with -config-service-host)`,
	)
}

// NewModule returns a server module that exposes LUCI Config validation
// endpoints.
func NewModule(opts *ModuleOptions) module.Module {
	if opts == nil {
		opts = &ModuleOptions{}
	}
	return &serverModule{opts: opts}
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

// serverModule implements module.Module.
type serverModule struct {
	opts *ModuleOptions
}

// Name is part of module.Module interface.
func (*serverModule) Name() module.Name {
	return ModuleName
}

// Dependencies is part of module.Module interface.
func (*serverModule) Dependencies() []module.Dependency {
	return nil
}

// Initialize is part of module.Module interface.
func (m *serverModule) Initialize(ctx context.Context, host module.Host, opts module.HostOptions) (context.Context, error) {
	if m.opts.Vars == nil {
		m.opts.Vars = &vars.Vars
	}
	if m.opts.Rules == nil {
		m.opts.Rules = &validation.Rules
	}
	m.registerVars(opts)

	var creds credentials.PerRPCCredentials
	if m.opts.ServiceHost != "" {
		var err error
		creds, err = auth.GetPerRPCCredentials(ctx,
			auth.AsSelf,
			auth.WithIDTokenAudience("https://"+m.opts.ServiceHost),
		)
		if err != nil {
			return nil, errors.Annotate(err, "failed to get credentials to access %s", m.opts.ServiceHost).Err()
		}
	}

	// Instantiate an appropriate client based on options.
	client, err := cfgclient.New(ctx, cfgclient.Options{
		Vars:              m.opts.Vars,
		ServiceHost:       m.opts.ServiceHost,
		ConfigsDir:        m.opts.LocalDir,
		PerRPCCredentials: creds,
		UserAgent:         host.UserAgent(),
	})
	if err != nil {
		return nil, err
	}

	// Make it available in the server handlers.
	ctx = cfgclient.Use(ctx, client)

	host.RegisterCleanup(func(ctx context.Context) {
		if err := client.Close(); err != nil {
			logging.Warningf(ctx, "Failed to close the config client: %s", err)
		}
	})

	// Register the prpc `config.Consumer` service that handles configs
	// validation.
	config.RegisterConsumerServer(host, &ConsumerServer{
		Rules: m.opts.Rules,
		GetConfigServiceAccountFn: func(ctx context.Context) (string, error) {
			// Grab the expected service account ID of the LUCI Config service we use.
			info, err := m.configServiceInfo(ctx)
			if err != nil {
				return "", err
			}
			return info.ServiceAccountName, nil
		},
	})
	return ctx, nil
}

// configServiceInfo fetches LUCI Config service account name and app ID.
//
// Returns an error if ServiceHost is unset.
func (m *serverModule) configServiceInfo(ctx context.Context) (*signing.ServiceInfo, error) {
	if m.opts.ServiceHost == "" {
		return nil, errors.Reason("-config-service-host is not set").Err()
	}
	return signing.FetchServiceInfoFromLUCIService(ctx, "https://"+m.opts.ServiceHost)
}

// registerVars populates the var set with predefined vars that can be used
// in config patterns.
func (m *serverModule) registerVars(opts module.HostOptions) {
	m.opts.Vars.Register("appid", func(context.Context) (string, error) {
		if opts.CloudProject == "" {
			return "", fmt.Errorf("can't resolve ${appid}: -cloud-project is not set")
		}
		return opts.CloudProject, nil
	})
	m.opts.Vars.Register("config_service_appid", func(ctx context.Context) (string, error) {
		info, err := m.configServiceInfo(ctx)
		if err != nil {
			return "", errors.Annotate(err, "can't resolve ${config_service_appid}").Err()
		}
		return info.AppID, nil
	})
}
