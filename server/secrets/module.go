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
	"encoding/base64"
	"flag"
	"math"
	"strings"
	"time"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/data/rand/mathrand"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/module"
)

// ModuleName can be used to refer to this module when declaring dependencies.
var ModuleName = module.RegisterName("go.chromium.org/luci/server/secrets")

// ModuleOptions contain configuration of the secrets server module.
type ModuleOptions struct {
	// RootSecret points to the root secret key used to derive all other secrets.
	//
	// It can be either a local file system path, a reference to a Google Secret
	// Manager secret (in a form "sm://<project>/<secret>"), or a literal secret
	// value (in a form "devsecret://<base64-encoded secret>"). The latter is
	// supposed to be used **only** during development (e.g. locally or in
	// development deployments).
	//
	// When it is a local file system path, it should point to a JSON file with
	// the following structure (see Secret struct):
	//
	//   {
	//     "current": <base64-encoded blob>,
	//     "previous": [<base64-encoded blob>, <base64-encoded blob>, ...]
	//   }
	//
	// When using Google Secret Manager, the secret version "latest" is used to
	// get the current value of the secret, and a single immediately preceding
	// previous version (if it is still enabled) is used to get the previous
	// version of the secret. This allows graceful rotation of secrets.
	RootSecret string

	// Source produces the root secret to use by the module.
	//
	// If given, overrides RootSecret. This is useful when the secrets module
	// is initialized programmatically rather than through flags.
	//
	// The module will periodically use Source's ReadSecret to refresh the root
	// secret it stores in memory. This allows the secret to be rotated without
	// restarting all servers that use it.
	Source Source
}

// Register registers the command line flags.
func (o *ModuleOptions) Register(f *flag.FlagSet) {
	f.StringVar(
		&o.RootSecret,
		"root-secret",
		o.RootSecret,
		`Either a local path to JSON file, `+
			`or "sm://<project>/<secret>" to use Google Secret Manager, `+
			`or "devsecret://<base64-encoded value>" for static development secret`,
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
	// Prepare a Source based on RootSecret.
	source := m.opts.Source
	if source == nil {
		if m.opts.RootSecret == "" {
			return ctx, nil // secrets are not configured, this is fine
		}
		var err error
		source, err = initSource(ctx, m.opts.RootSecret, opts.Prod)
		if err != nil {
			return nil, errors.Annotate(err, "can't initialize the secret source").Err()
		}
	}

	// This is mostly useful for SecretManagerSource to gracefully shutdown
	// the gRPC connection.
	host.RegisterCleanup(func(context.Context) { source.Close() })

	// Read the initial value of the secret, can't start without it.
	secret, err := source.ReadSecret(ctx)
	if err != nil {
		return nil, errors.Annotate(err, "failed to read the initial value of the root secret").Err()
	}
	store := NewDerivedStore(*secret)

	// Periodically re-read the secret from the source to support rotation.
	host.RunInBackground("luci.secrets", func(ctx context.Context) {
		attempt := 0
		for {
			sleep := secretReadInterval(ctx, attempt)
			if attempt > 0 {
				logging.Errorf(ctx, "Will attempt to read the secret again in %s", sleep)
			}
			if r := <-clock.After(ctx, sleep); r.Err != nil {
				return // the context is canceled
			}
			secret, err := source.ReadSecret(ctx)
			if err != nil {
				attempt += 1
				logging.Errorf(ctx, "Failed to re-read the root secret (attempt %d): %s", attempt, err)
			} else {
				attempt = 0
				store.SetRoot(*secret)
			}
		}
	})

	return Use(ctx, store), nil
}

// initSource initializes a correct Source based on rootSecret.
func initSource(ctx context.Context, rootSecret string, prod bool) (Source, error) {
	switch {
	case strings.HasPrefix(rootSecret, "devsecret://"):
		value, err := base64.RawStdEncoding.DecodeString(strings.TrimPrefix(rootSecret, "devsecret://"))
		if err != nil {
			return nil, errors.Annotate(err, "bad devsecret://, not base64 encoding").Err()
		}
		return &StaticSource{Secret: &Secret{Current: value}}, nil

	case strings.HasPrefix(rootSecret, "sm://"):
		ts, err := auth.GetTokenSource(ctx, auth.AsSelf, auth.WithScopes(auth.CloudOAuthScopes...))
		if err != nil {
			return nil, errors.Annotate(err, "failed to get the token source").Err()
		}
		return NewSecretManagerSource(ctx, rootSecret, ts)

	case strings.Contains(rootSecret, "://"):
		return nil, errors.Reason("not supported secret reference %q", rootSecret).Err()

	default:
		return &FileSource{Path: rootSecret}, nil
	}
}

// secretReadInterval tells how long to sleep between ReadSecret calls.
//
// When there are no read errors, sleep 10-15 min (randomized).
// When there are errors, do exponential backoff with jitter.
func secretReadInterval(ctx context.Context, attempt int) time.Duration {
	if attempt == 0 {
		return 10*time.Minute + time.Duration(mathrand.Int63n(ctx, int64(5*time.Minute)))
	}
	factor := math.Pow(2.0, float64(attempt)) // 2, 4, 8, ...
	factor += 10 * mathrand.Float64(ctx)
	dur := time.Duration(float64(time.Second) * factor)
	if dur > 30*time.Minute {
		dur = 30 * time.Minute
	}
	return dur
}
