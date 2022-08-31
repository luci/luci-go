// Copyright 2022 The LUCI Authors.
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

// Package legacydb defines a LUCI Server module that connects to a legacy
// (old) Spanner database to support migration from another system. It is
// based on the Span server module at go.chromium.org/luci/server/span.
package legacydb

import (
	"context"
	"flag"
	"strings"

	"cloud.google.com/go/spanner"
	"google.golang.org/grpc"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/grpc/grpcmon"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/module"
	"google.golang.org/api/option"
)

// ModuleName can be used to refer to this module when declaring dependencies.
var ModuleName = module.RegisterName("go.chromium.org/luci/analysis/internal/migration")

// ModuleOptions contain configuration of the Cloud Spanner server module.
type ModuleOptions struct {
	SpannerDatabase string // identifier of Cloud Spanner database to connect to
}

// Register registers the command line flags.
func (o *ModuleOptions) Register(f *flag.FlagSet) {
	f.StringVar(
		&o.SpannerDatabase,
		"legacy-spanner-database",
		o.SpannerDatabase,
		"Identifier of the Legacy Cloud Spanner database to connect to. A valid database "+
			"name has the form projects/PROJECT_ID/instances/INSTANCE_ID/databases/DATABASE_ID. Required.",
	)
}

// NewModule returns a server module that sets up a Spanner client connected
// to some single Cloud Spanner database.
//
// Client's functionality is exposed via Single(ctx), ReadOnlyTransaction(ctx),
// ReadWriteTransaction(ctx), etc.
//
// The underlying *spanner.Client is intentionally not exposed to make sure
// all callers use the functions mentioned above since they generally add
// additional functionality on top of the raw Spanner client that other LUCI
// packages assume to be present. Using the Spanner client directly may violate
// such assumptions leading to undefined behavior when multiple packages are
// used together.
func NewModule(opts *ModuleOptions) module.Module {
	if opts == nil {
		opts = &ModuleOptions{}
	}
	return &legacyDBModule{opts: opts}
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

// legacyDBModule implements module.Module.
type legacyDBModule struct {
	opts *ModuleOptions
}

// Name is part of module.Module interface.
func (*legacyDBModule) Name() module.Name {
	return ModuleName
}

// Dependencies is part of module.Module interface.
func (*legacyDBModule) Dependencies() []module.Dependency {
	return nil
}

// Initialize is part of module.Module interface.
func (m *legacyDBModule) Initialize(ctx context.Context, host module.Host, opts module.HostOptions) (context.Context, error) {
	switch {
	case m.opts.SpannerDatabase == "":
		return nil, nil
	case !isValidDB(m.opts.SpannerDatabase):
		return nil, errors.New("Legacy Cloud Spanner database name must have form `projects/.../instances/.../databases/...`")
	}

	// Credentials with Cloud scope.
	creds, err := auth.GetPerRPCCredentials(ctx, auth.AsSelf, auth.WithScopes(auth.CloudOAuthScopes...))
	if err != nil {
		return nil, errors.Annotate(err, "failed to get PerRPCCredentials").Err()
	}

	// Initialize the client.
	options := []option.ClientOption{
		option.WithGRPCDialOption(grpc.WithPerRPCCredentials(creds)),
		option.WithGRPCDialOption(grpcmon.WithClientRPCStatsMonitor()),
	}
	client, err := spanner.NewClientWithConfig(ctx,
		m.opts.SpannerDatabase,
		spanner.ClientConfig{
			SessionPoolConfig: spanner.SessionPoolConfig{
				TrackSessionHandles: !opts.Prod,
			},
		},
		options...,
	)
	if err != nil {
		return nil, errors.Annotate(err, "failed to instantiate Legacy Cloud Spanner client").Err()
	}
	ctx = UseLegacyClient(ctx, client)

	// Close the client when exiting gracefully.
	host.RegisterCleanup(func(ctx context.Context) { client.Close() })

	return ctx, nil
}

func isValidDB(name string) bool {
	chunks := strings.Split(name, "/")
	if len(chunks) != 6 {
		return false
	}
	for _, ch := range chunks {
		if ch == "" {
			return false
		}
	}
	return chunks[0] == "projects" && chunks[2] == "instances" && chunks[4] == "databases"
}

var (
	clientContextKey = "go.chromium.org/luci/analysis/internal/migration:client"
)

// UseLegacyClient installs a Legacy Spanner client into the context.
//
// Primarily used by the module initialization code. May be useful in tests as
// well.
func UseLegacyClient(ctx context.Context, client *spanner.Client) context.Context {
	return context.WithValue(ctx, &clientContextKey, client)
}

// LegacyClient returns a Spanner client used to connect to the old database
// (if any). This may be nil if no old database was configured.
func LegacyClient(ctx context.Context) *spanner.Client {
	cl, _ := ctx.Value(&clientContextKey).(*spanner.Client)
	if cl == nil {
		return nil
	}
	return cl
}
