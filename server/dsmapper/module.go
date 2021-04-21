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

package dsmapper

import (
	"context"
	"flag"

	"go.chromium.org/luci/server/gaeemulation"
	"go.chromium.org/luci/server/module"
	"go.chromium.org/luci/server/tq"
)

// ModuleName can be used to refer to this module when declaring dependencies.
var ModuleName = module.RegisterName("go.chromium.org/luci/server/dsmapper")

// ModuleOptions contain configuration of the dsmapper server module.
type ModuleOptions struct {
	// MapperQueue is a name of the Cloud Tasks queue to use for mapping jobs.
	//
	// This queue will perform all "heavy" tasks. It should be configured
	// appropriately to allow desired number of shards to run in parallel.
	//
	// For example, if the largest submitted job is expected to have 128 shards,
	// max_concurrent_requests setting of the mapper queue should be at least 128,
	// otherwise some shards will be stalled waiting for others to finish
	// (defeating the purpose of having large number of shards).
	//
	// If empty, "default" is used.
	MapperQueue string

	// ControlQueue is a name of the Cloud Tasks queue to use for control signals.
	//
	// This queue is used very lightly when starting and stopping jobs (roughly
	// 2*Shards tasks overall per job). A default queue.yaml settings for such
	// queue should be sufficient (unless you run a lot of different jobs at
	// once).
	//
	// If empty, "default" is used.
	ControlQueue string
}

// Register registers the command line flags.
func (o *ModuleOptions) Register(f *flag.FlagSet) {
	if o.MapperQueue == "" {
		o.MapperQueue = "default"
	}
	if o.ControlQueue == "" {
		o.ControlQueue = "default"
	}
	f.StringVar(
		&o.MapperQueue,
		"dsmapper-mapper-queue",
		o.MapperQueue,
		`Cloud Tasks queue to use for mapping jobs.`,
	)
	f.StringVar(
		&o.ControlQueue,
		"dsmapper-control-queue",
		o.ControlQueue,
		`Cloud Tasks queue to use for control signals.`,
	)
}

// NewModule returns a server module that initializes Default controller.
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
	return []module.Dependency{
		module.RequiredDependency(gaeemulation.ModuleName),
		module.RequiredDependency(tq.ModuleName),
	}
}

// Initialize is part of module.Module interface.
func (m *serverModule) Initialize(ctx context.Context, host module.Host, opts module.HostOptions) (context.Context, error) {
	Default.ControlQueue = m.opts.ControlQueue
	Default.MapperQueue = m.opts.MapperQueue
	Default.Install(&tq.Default)
	return nil, nil
}
