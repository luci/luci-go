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
	"net/http"
	"strings"

	"google.golang.org/grpc/credentials"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/proto/config"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/signing"
	"go.chromium.org/luci/server/module"
	"go.chromium.org/luci/server/router"

	"go.chromium.org/luci/config/cfgclient"
	"go.chromium.org/luci/config/validation"
	"go.chromium.org/luci/config/vars"
)

// ModuleName can be used to refer to this module when declaring dependencies.
var ModuleName = module.RegisterName("go.chromium.org/luci/config/server/cfgmodule")

// A scope expected in access tokens from the LUCI Config service.
const configValidationAuthScope = "https://www.googleapis.com/auth/userinfo.email"

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

	// Enable authentication and authorization for the validation endpoint only
	// when running in production (i.e. not on a developer workstation).
	var middleware router.MiddlewareChain
	if opts.Prod {
		middleware = router.NewMiddlewareChain(
			(&auth.Authenticator{
				Methods: []auth.Method{
					&auth.GoogleOAuth2Method{
						Scopes: []string{configValidationAuthScope},
					},
				},
			}).GetMiddleware(),
			m.authorizeConfigService,
		)
	}

	// Install the validation endpoint that will be called by the legacy
	// LUCI Config.
	// TODO(yiwzhang): Remove after all apps using LUCI Server Framework is
	// redeployed and the legacy LUCI Config service has been turned down.
	InstallHandlers(host.Routes(), middleware, m.opts.Rules)

	// Register the prpc `config.Consumer` service that handles configs
	// validation.
	config.RegisterConsumerServer(host, &ConsumerServer{
		Rules: m.opts.Rules,
		GetConfigServiceAccountFn: func(ctx context.Context) (string, error) {
			// TODO(yiwzhang): Remove this after the service host pointing to the new
			// LUCI Config service. For now, hardcode the expected service account
			// name when the service is still using legacy LUCI Config service as
			// config service host. However, it's possible for the service to
			// receive the validation traffic from the new LUCI Config service.
			// So the consumer server needs to allow corresponding new LUCI Config
			// service account.
			if strings.HasSuffix(m.opts.ServiceHost, "appspot.com") {
				if strings.Contains(m.opts.ServiceHost, "dev") {
					return "config-service@luci-config-dev.iam.gserviceaccount.com", nil
				}
				return "config-service@luci-config.iam.gserviceaccount.com", nil
			}
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

// authorizeConfigService is a middleware that passes the request only if it
// came from LUCI Config service.
func (m *serverModule) authorizeConfigService(c *router.Context, next router.Handler) {
	// Grab the expected service account ID of the LUCI Config service we use.
	info, err := m.configServiceInfo(c.Request.Context())
	if err != nil {
		errors.Log(c.Request.Context(), err)
		if transient.Tag.In(err) {
			http.Error(c.Writer, "Transient error during authorization", http.StatusInternalServerError)
		} else {
			http.Error(c.Writer, "Permission denied (not configured)", http.StatusForbidden)
		}
		return
	}

	// Check the call is actually from the LUCI Config.
	caller := auth.CurrentIdentity(c.Request.Context())
	if caller.Kind() == identity.User && caller.Value() == info.ServiceAccountName {
		next(c)
	} else {
		// TODO(yiwzhang): Temporarily allow both old and new LUCI Config service
		// account to make the validation request. Revert this change after the old
		// LUCI Config service is fully deprecated and all traffic have been
		// migrated to the new LUCI Config service.
		allowedGroup := "service-accounts-luci-config"
		if strings.Contains(m.opts.ServiceHost, "dev") {
			allowedGroup += "-dev"
		}
		switch yes, err := auth.IsMember(c.Request.Context(), allowedGroup); {
		case err != nil:
			errors.Log(c.Request.Context(), err)
			http.Error(c.Writer, "error during authorization", http.StatusInternalServerError)
		case yes:
			next(c)
		default:
			http.Error(c.Writer, "Permission denied", http.StatusForbidden)
		}
	}
}
