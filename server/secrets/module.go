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
	"encoding/json"
	"flag"
	"os"
	"time"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/server/module"
)

// TODO(vadimsh): Add support for fetching the root secret from Cloud Secret
// Manager (https://cloud.google.com/secret-manager/docs/).

// ModuleOptions contain configuration of the secrets server module.
type ModuleOptions struct {
	RootSecretPath string // path to a JSON file with the root secret key
}

// Register registers the command line flags.
func (o *ModuleOptions) Register(f *flag.FlagSet) {
	f.StringVar(
		&o.RootSecretPath,
		"root-secret-path",
		o.RootSecretPath,
		`Path to a JSON file with the root secret key, or literal ":dev" for development not-really-a-secret`,
	)
}

// NewModule returns a server module that adds a secret store to the global
// server context.
//
// Uses a DerivedStore with the root secret populated based on the supplied
// options. When the server starts, the module reads the initial root secret (if
// provided) and launches a job to periodically reread it.
//
// An error to read the secret during the startup is fatal. But if the server
// managed to start successfully and can't re-read the secret later (e.g. the
// file disappeared), it logs the error and keeps using the cached secret.
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
	opts  *ModuleOptions
	store *DerivedStore // nil if RootSecretPath is not set
}

// Name is part of module.Module interface.
func (*serverModule) Name() string {
	return "go.chromium.org/luci/server/secrets"
}

// Initialize is part of module.Module interface.
func (m *serverModule) Initialize(ctx context.Context, host module.Host, opts module.HostOptions) (context.Context, error) {
	secret, err := m.readRootSecret(opts.Prod)
	switch {
	case err != nil:
		return nil, err
	case secret == nil:
		return ctx, nil // not configured, this is fine
	}
	m.store = NewDerivedStore(*secret)

	host.RunInBackground("luci.secrets", func(ctx context.Context) {
		for {
			if r := <-clock.After(ctx, time.Minute); r.Err != nil {
				return // the context is canceled
			}
			secret, err := m.readRootSecret(opts.Prod)
			if secret == nil {
				logging.WithError(err).Errorf(ctx, "Failed to re-read the root secret, using the cached one")
			} else {
				m.store.SetRoot(*secret)
			}
		}
	})

	return Set(ctx, m.store), nil
}

// readRootSecret reads the secret from a path specified in the options.
//
// Returns nil if the secret is not configured. Returns an error if the secret
// is configured, but could not be loaded.
func (m *serverModule) readRootSecret(prod bool) (*Secret, error) {
	switch {
	case m.opts.RootSecretPath == "":
		return nil, nil // not configured, this is fine
	case m.opts.RootSecretPath == ":dev" && !prod:
		return &Secret{Current: []byte("dev-non-secret")}, nil
	case m.opts.RootSecretPath == ":dev" && prod:
		return nil, errors.Reason("-root-secret-path \":dev\" is not allowed in production mode").Err()
	}

	f, err := os.Open(m.opts.RootSecretPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	secret := &Secret{}
	if err = json.NewDecoder(f).Decode(secret); err != nil {
		return nil, errors.Annotate(err, "not a valid JSON").Err()
	}
	if len(secret.Current) == 0 {
		return nil, errors.Reason("`current` field in the root secret is empty, this is not allowed").Err()
	}
	return secret, nil
}
