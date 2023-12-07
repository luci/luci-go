// Copyright 2021 The LUCI Authors.
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

package bqlog

import (
	"context"
	"flag"
	"time"

	storage "cloud.google.com/go/bigquery/storage/apiv1beta2"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/api/option"
	"google.golang.org/grpc"
	"google.golang.org/grpc/keepalive"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/grpc/grpcmon"

	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/module"
)

// ModuleName can be used to refer to this module when declaring dependencies.
var ModuleName = module.RegisterName("go.chromium.org/luci/server/bqlog")

// ModuleOptions contain configuration of the bqlog server module.
//
// It will be used to initialize Default bundler.
type ModuleOptions struct {
	// Bundler is a bundler to use.
	//
	// Default is the global Default instance.
	Bundler *Bundler

	// CloudProject is a project with the dataset to send logs to.
	//
	// Default is the project the server is running in.
	CloudProject string

	// Dataset is a BQ dataset to send logs to.
	//
	// Can be omitted in the development mode. In that case BQ writes will be
	// disabled and the module will be running in the dry run mode. Required in
	// the production.
	Dataset string
}

// Register registers the command line flags.
func (o *ModuleOptions) Register(f *flag.FlagSet) {
	f.StringVar(&o.CloudProject, "bqlog-cloud-project", o.CloudProject,
		`Cloud Project with the BQ dataset with tables to writes logs into, default is the same as -cloud-project.`)
	f.StringVar(&o.Dataset, "bqlog-dataset", o.Dataset,
		`BigQuery dataset name within the project with tables to writes logs into. Required.`)
}

// NewModule returns a server module that sets up a cron dispatcher.
func NewModule(opts *ModuleOptions) module.Module {
	if opts == nil {
		opts = &ModuleOptions{}
	}
	return &bqlogModule{opts: opts}
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

// bqlogModule implements module.Module.
type bqlogModule struct {
	opts *ModuleOptions
}

// Name is part of module.Module interface.
func (*bqlogModule) Name() module.Name {
	return ModuleName
}

// Dependencies is part of module.Module interface.
func (*bqlogModule) Dependencies() []module.Dependency {
	return nil
}

// Initialize is part of module.Module interface.
func (m *bqlogModule) Initialize(ctx context.Context, host module.Host, opts module.HostOptions) (context.Context, error) {
	bundler := m.opts.Bundler
	if bundler == nil {
		bundler = &Default
	}

	if m.opts.CloudProject != "" {
		bundler.CloudProject = m.opts.CloudProject
	} else {
		bundler.CloudProject = opts.CloudProject
	}
	if opts.Prod && m.opts.Dataset == "" {
		return nil, errors.Reason("flag -bqlog-dataset is required").Err()
	}
	bundler.Dataset = m.opts.Dataset

	var writer BigQueryWriter
	if m.opts.Dataset != "" {
		// Create the production writer that writes to the BQ for real.
		creds, err := auth.GetPerRPCCredentials(ctx, auth.AsSelf, auth.WithScopes(auth.CloudOAuthScopes...))
		if err != nil {
			return nil, errors.Annotate(err, "failed to initialize credentials").Err()
		}
		writer, err = storage.NewBigQueryWriteClient(ctx,
			option.WithGRPCDialOption(grpc.WithStatsHandler(&grpcmon.ClientRPCStatsMonitor{})),
			option.WithGRPCDialOption(grpc.WithUnaryInterceptor(otelgrpc.UnaryClientInterceptor())),
			option.WithGRPCDialOption(grpc.WithStreamInterceptor(otelgrpc.StreamClientInterceptor())),
			option.WithGRPCDialOption(grpc.WithPerRPCCredentials(creds)),
			option.WithGRPCDialOption(grpc.WithKeepaliveParams(keepalive.ClientParameters{
				Time: time.Minute,
			})),
		)
		if err != nil {
			return nil, errors.Annotate(err, "failed to create BQ write client").Err()
		}
	} else {
		// Use some phony names in the dev mode, we won't be writing to BQ.
		bundler.CloudProject = "bqlog-project"
		bundler.Dataset = "bqlog-dataset"
		// Use a writer that just silently discards everything.
		writer = &FakeBigQueryWriter{}
	}

	// Launch the bundler background activities after the server is fully
	// initialized (i.e. when it launches its background activities). But use
	// the root context, since we want the bundler to keep running right until its
	// Shutdown logic is called below. If we use `ctx` from RunInBackground,
	// the bundler will completely shutdown before it has a chance to drain itself
	// properly.
	rootCtx := ctx
	host.RunInBackground("luci.bqlog", func(ctx context.Context) {
		bundler.Start(rootCtx, writer)
	})

	// Try to flush everything gracefully when closing down.
	host.RegisterCleanup(func(ctx context.Context) {
		bundler.Shutdown(ctx)
		writer.Close()
	})

	return ctx, nil
}
