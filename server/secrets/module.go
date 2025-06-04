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

package secrets

import (
	"context"
	"flag"

	secretmanager "cloud.google.com/go/secretmanager/apiv1"
	"google.golang.org/api/option"
	"google.golang.org/grpc"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/tsmon"
	"go.chromium.org/luci/grpc/grpcmon"

	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/module"
)

// ModuleName can be used to refer to this module when declaring dependencies.
var ModuleName = module.RegisterName("go.chromium.org/luci/server/secrets")

// ModuleOptions contain configuration of the secrets server module.
type ModuleOptions struct {
	// RootSecret points to the root secret used to derive random secrets.
	//
	// In production it should be a reference to a Google Secret Manager secret
	// (in a form "sm://<project>/<secret>" or just "sm://<secret>" to fetch it
	// from the current project).
	//
	// In non-production environments it can be a literal secret value in a form
	// "devsecret://<base64-encoded secret>" or "devsecret-text://<secret>". If
	// omitted in a non-production environment, some phony hardcoded value is
	// used.
	//
	// When using Google Secret Manager, the secret version "latest" is used to
	// get the current value of the root secret, and a single immediately
	// preceding previous version (if it is still enabled) is used to get the
	// previous version of the root secret. This allows graceful rotation of
	// random secrets.
	RootSecret string

	// PrimaryTinkAEADKey is the secret name with the JSON-serialized clear text
	// Tink AEAD keyset to use for AEAD operations by default via PrimaryTinkAEAD.
	//
	// It is optional. If unset, PrimaryTinkAEAD will return nil. Code that
	// depends on a presence of an AEAD implementation must check that the return
	// value of PrimaryTinkAEAD is not nil during startup.
	PrimaryTinkAEADKey string
}

// Register registers the command line flags.
func (o *ModuleOptions) Register(f *flag.FlagSet) {
	f.StringVar(
		&o.RootSecret,
		"root-secret",
		o.RootSecret,
		`Either "sm://<project>/<secret>" or "sm://<secret>" to use Google Secret Manager, `+
			`or "devsecret://<base64-encoded value>" or "devsecret-text://<value>" `+
			`for a static development secret.`,
	)
	f.StringVar(
		&o.PrimaryTinkAEADKey,
		"primary-tink-aead-key",
		o.PrimaryTinkAEADKey,
		`A "sm://..." reference to a clear text JSON Tink AEAD key set to use for `+
			`AEAD operations by default. Optional, but some server modules may require `+
			`it and will refuse to start if it is not set. `+
			`For development, you need a valid AEAD keyset and pass it via `+
			`devsecret://... or devsecret-text://... or specify `+
			`devsecret-gen://tink/aead to automatically generate a new random key, `+
			`which you can then re-use via devsecret:// in the future.`,
	)
}

// NewModule returns a server module that adds a secret store backed by Google
// Secret Manager to the global server context.
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
	if !opts.Prod && m.opts.RootSecret == "" {
		m.opts.RootSecret = "devsecret-text://phony-root-secret-do-not-depend-on"
	}

	ts, err := auth.GetTokenSource(ctx, auth.AsSelf, auth.WithScopes(auth.CloudOAuthScopes...))
	if err != nil {
		return nil, errors.Fmt("failed to initialize the token source: %w", err)
	}
	client, err := secretmanager.NewClient(
		ctx,
		option.WithTokenSource(ts),
		option.WithGRPCDialOption(grpc.WithStatsHandler(&grpcmon.ClientRPCStatsMonitor{})),
	)

	if err != nil {
		return nil, errors.Fmt("failed to initialize the Secret Manager client: %w", err)
	}
	host.RegisterCleanup(func(context.Context) { client.Close() })

	store := &SecretManagerStore{
		CloudProject:        opts.CloudProject,
		AccessSecretVersion: client.AccessSecretVersion,
	}
	ctx = Use(ctx, store)

	if m.opts.RootSecret != "" {
		if err := store.LoadRootSecret(ctx, m.opts.RootSecret); err != nil {
			return nil, errors.Fmt("failed to initialize the secret store: %w", err)
		}
	}

	if m.opts.PrimaryTinkAEADKey != "" {
		aead, err := LoadTinkAEAD(ctx, m.opts.PrimaryTinkAEADKey)
		if err != nil {
			return nil, errors.Fmt("failed to initialize the primary tink AEAD key: %w", err)
		}
		ctx = setPrimaryTinkAEAD(ctx, aead)
	}

	host.RunInBackground("luci.secrets", store.MaintenanceLoop)

	// Report initial values of metrics and refresh them on every tsmon flush.
	store.ReportMetrics(ctx)
	tsmon.RegisterCallbackIn(ctx, store.ReportMetrics)

	return ctx, nil
}
