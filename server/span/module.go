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

// Package span implements a server module for communicating with Cloud Spanner.
package span

import (
	"context"
	"flag"
	"strings"
	"time"

	"cloud.google.com/go/spanner"
	"google.golang.org/api/option"
	"google.golang.org/grpc"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/grpc/grpcmon"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/module"
)

// ModuleName can be used to refer to this module when declaring dependencies.
var ModuleName = module.RegisterName("go.chromium.org/luci/server/span")

// ModuleOptions contain configuration of the Cloud Spanner server module.
type ModuleOptions struct {
	SpannerEndpoint string // the Spanner endpoint to connect to
	SpannerDatabase string // identifier of Cloud Spanner database to connect to
}

// Register registers the command line flags.
func (o *ModuleOptions) Register(f *flag.FlagSet) {
	f.StringVar(
		&o.SpannerEndpoint,
		"spanner-endpoint",
		o.SpannerEndpoint,
		"The Spanner endpoint to connect to. "+
			"The default is defined by the Cloud Spanner library, "+
			"but usually it is spanner.googleapis.com:443",
	)
	f.StringVar(
		&o.SpannerDatabase,
		"spanner-database",
		o.SpannerDatabase,
		"Identifier of the Cloud Spanner database to connect to. A valid database "+
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
	return &spannerModule{opts: opts}
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

// spannerModule implements module.Module.
type spannerModule struct {
	opts *ModuleOptions
}

// Name is part of module.Module interface.
func (*spannerModule) Name() module.Name {
	return ModuleName
}

// Dependencies is part of module.Module interface.
func (*spannerModule) Dependencies() []module.Dependency {
	return nil
}

// Initialize is part of module.Module interface.
func (m *spannerModule) Initialize(ctx context.Context, host module.Host, opts module.HostOptions) (context.Context, error) {
	switch {
	case m.opts.SpannerDatabase == "":
		return nil, errors.New("Cloud Spanner database name is required")
	case !isValidDB(m.opts.SpannerDatabase):
		return nil, errors.New("Cloud Spanner database name must have form `projects/.../instances/.../databases/...`")
	}

	// Credentials with Cloud scope.
	creds, err := auth.GetPerRPCCredentials(ctx, auth.AsSelf, auth.WithScopes(auth.CloudOAuthScopes...))
	if err != nil {
		return nil, errors.Annotate(err, "failed to get PerRPCCredentials").Err()
	}

	// Initialize the client.
	options := []option.ClientOption{
		option.WithGRPCDialOption(grpc.WithPerRPCCredentials(creds)),
		option.WithGRPCDialOption(grpc.WithUnaryInterceptor(grpcmon.NewUnaryClientInterceptor(nil))),
	}
	if m.opts.SpannerEndpoint != "" {
		options = append(options, option.WithEndpoint(m.opts.SpannerEndpoint))
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
		return nil, errors.Annotate(err, "failed to instantiate Cloud Spanner client").Err()
	}
	ctx = UseClient(ctx, client)

	// Close the client when exiting gracefully.
	host.RegisterCleanup(func(ctx context.Context) { client.Close() })

	// Run a "select 1" query to verify the database exists and we can access it
	// before we actually serve any requests.
	if err := pingDB(ctx); err != nil {
		return nil, errors.Annotate(err, "failed to ping Cloud Spanner database").Err()
	}

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

func pingDB(ctx context.Context) error {
	ctx, done := context.WithTimeout(Single(ctx), 30*time.Second)
	defer done()
	return Query(ctx, spanner.NewStatement("SELECT 1;")).Do(func(*spanner.Row) error { return nil })
}
